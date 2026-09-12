package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"sonar-survey-coverage-planner/backend/internal/constants"
	"sonar-survey-coverage-planner/backend/internal/dto"
	"sonar-survey-coverage-planner/backend/internal/geometry"
	"sonar-survey-coverage-planner/backend/internal/model"
	"sonar-survey-coverage-planner/backend/internal/repository"
	"sonar-survey-coverage-planner/backend/pkg/api"
)

// ReplayView 是回放快照的读取视图：记录本体、冻结帧 GeoJSON 和冻结汇总。
type ReplayView struct {
	Replay  model.RunReplay        `json:"replay"`
	Frames  json.RawMessage        `json:"frames"`
	Summary geometry.ReplaySummary `json:"summary"`
}

type RunReplayService struct {
	repository *repository.RunReplayRepository
	runs       *repository.SonarRunRepository
	audit      *AuditService
}

func NewRunReplayService(repository *repository.RunReplayRepository, runs *repository.SonarRunRepository, audit *AuditService) *RunReplayService {
	return &RunReplayService{repository: repository, runs: runs, audit: audit}
}

func (s *RunReplayService) List(query dto.RunReplayQuery) ([]model.RunReplay, int64, error) {
	return s.repository.List(query)
}

func (s *RunReplayService) Get(id uint) (ReplayView, error) {
	item, err := s.repository.Get(id)
	if err != nil {
		return ReplayView{}, mapDatabaseError(err, "运行回放")
	}
	return replayView(item)
}

// Create 为已处理运行生成固定回放快照。所有校验失败都在任何写库之前返回，
// 因此非法运行、采样缺失、坐标越界和重复回放都不会改动原有数据。
func (s *RunReplayService) Create(request dto.CreateReplayRequest, actor Actor) (ReplayView, error) {
	run, err := s.runs.Get(request.RunID)
	if err != nil {
		return ReplayView{}, mapDatabaseError(err, "声呐运行")
	}
	if run.RunState != string(constants.RunProcessed) {
		return ReplayView{}, api.Conflict("RUN_NOT_PROCESSED", "仅已处理运行可生成质量回放", nil)
	}
	if run.TransectPlan == nil || run.TransectPlan.SurveyArea == nil {
		return ReplayView{}, api.Unprocessable("RUN_AREA_MISSING", "运行缺少关联测区，无法校验回放边界", nil)
	}
	area := run.TransectPlan.SurveyArea
	if err := geometry.ValidateProjectedCRS(area.CoordinateSystem); err != nil {
		return ReplayView{}, api.Unprocessable("COORDINATE_SYSTEM_INVALID", "回放只支持米制投影坐标", err)
	}
	algorithm := request.AlgorithmVersion
	if algorithm == "" {
		algorithm = constants.ReplayAlgorithmV1
	}
	if !constants.SupportedReplayAlgorithm(algorithm) {
		return ReplayView{}, api.Unprocessable("REPLAY_ALGORITHM_UNSUPPORTED", "不支持的回放算法版本", nil)
	}
	lines, err := geometry.ParseLines(run.TrackGeoJSON)
	if err != nil {
		return ReplayView{}, api.Unprocessable("GEOJSON_INVALID", "运行航迹无法解析", err)
	}
	actual := geometry.CountSamples(lines)
	if err := checkDeclaredSamples(run.TrackGeoJSON, actual); err != nil {
		return ReplayView{}, err
	}
	if request.ExpectedSamples > 0 && request.ExpectedSamples != actual {
		return ReplayView{}, unprocessableDetails("REPLAY_SAMPLE_MISSING", "请求声明的采样数与航迹实际采样不一致", map[string]int{"declared": request.ExpectedSamples, "actual": actual})
	}
	boundary, err := geometry.ParsePolygon(area.BoundaryGeoJSON)
	if err != nil {
		return ReplayView{}, api.Unprocessable("GEOJSON_INVALID", "测区边界无法计算", err)
	}
	if seq, point, found := geometry.FirstOutsideBoundary(lines, boundary); found {
		return ReplayView{}, unprocessableDetails("REPLAY_COORDINATE_OUT_OF_BOUNDS", "航迹采样超出测区边界", map[string]any{"seq": seq, "x": point[0], "y": point[1]})
	}
	params := replayParams(request, area.TargetResolutionM)
	hash := geometry.ReplayInputHash(run.SourceChecksum, algorithm, params, request.ExpectedSamples)
	if existing, lookupErr := s.repository.ByInputHash(hash); lookupErr == nil {
		return ReplayView{}, conflictDetails("REPLAY_DUPLICATE", "相同输入的回放快照已存在", map[string]any{"existing_replay_id": existing.ID, "input_hash": hash})
	} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		return ReplayView{}, lookupErr
	}
	frames, summary, err := geometry.BuildReplay(lines, run.ActualSwathM, constants.NavBaseAccuracy(run.NavigationQuality), run.StartedAt, run.EndedAt, params)
	if err != nil {
		return ReplayView{}, replayCalculationError(err)
	}
	framesJSON, err := geometry.ReplayFramesFeatureCollection(frames)
	if err != nil {
		return ReplayView{}, fmt.Errorf("marshal replay frames: %w", err)
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return ReplayView{}, fmt.Errorf("marshal replay summary: %w", err)
	}
	item := model.RunReplay{
		RunID: run.ID, AlgorithmVersion: algorithm, InputHash: hash,
		SampleCount: summary.SampleCount, DroppedPoints: summary.DroppedPoints, DropoutEvents: summary.DropoutEvents,
		OverlapCells: summary.OverlapCells, OverlapRatio: summary.OverlapRatio, AccuracyAnomalies: summary.AccuracyAnomalies,
		FramesGeoJSON: datatypes.JSON(framesJSON), SummaryJSON: datatypes.JSON(summaryJSON),
		CreatedBy: actor.UserID, CreatedAt: time.Now().UTC(),
	}
	if err := s.repository.Create(&item); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return ReplayView{}, conflictDetails("REPLAY_DUPLICATE", "相同输入的回放快照已存在", map[string]any{"input_hash": hash})
		}
		return ReplayView{}, mapDatabaseError(err, "运行回放")
	}
	if err := s.audit.Record(actor, "replay.create", "run_replay", item.ID, nil, item, map[string]any{"input_hash": hash, "algorithm_version": algorithm, "run_id": run.ID, "run_code": run.RunCode, "summary": summary}); err != nil {
		return ReplayView{}, err
	}
	created, err := s.repository.Get(item.ID)
	if err != nil {
		return ReplayView{}, mapDatabaseError(err, "运行回放")
	}
	return replayView(created)
}

func checkDeclaredSamples(trackGeoJSON []byte, actual int) error {
	declared, found, err := geometry.DeclaredSampleCount(trackGeoJSON)
	if err != nil {
		return api.Unprocessable("REPLAY_SAMPLE_MISSING", "航迹声明的采样数无效", err)
	}
	if found && declared != actual {
		return unprocessableDetails("REPLAY_SAMPLE_MISSING", "航迹声明的采样数与实际采样不一致", map[string]int{"declared": declared, "actual": actual})
	}
	return nil
}

func unprocessableDetails(code, message string, details any) *api.AppError {
	err := api.Unprocessable(code, message, nil)
	err.Details = details
	return err
}

func conflictDetails(code, message string, details any) *api.AppError {
	err := api.Conflict(code, message, nil)
	err.Details = details
	return err
}

func replayParams(request dto.CreateReplayRequest, defaultResolution float64) geometry.ReplayParams {
	params := geometry.ReplayParams{
		DropoutFactor:      constants.ReplayDefaultDropoutFactor,
		AccuracyThresholdM: constants.ReplayDefaultAccuracyThresholdM,
		OverlapResolutionM: defaultResolution,
		RevisitWindow:      constants.ReplayDefaultRevisitWindow,
	}
	if request.DropoutFactor > 0 {
		params.DropoutFactor = request.DropoutFactor
	}
	if request.AccuracyThresholdM > 0 {
		params.AccuracyThresholdM = request.AccuracyThresholdM
	}
	if request.OverlapResolutionM > 0 {
		params.OverlapResolutionM = request.OverlapResolutionM
	}
	return params
}

func replayCalculationError(err error) error {
	switch {
	case errors.Is(err, geometry.ErrTooManySamples):
		return api.Unprocessable("REPLAY_SAMPLE_OVERFLOW", "采样数超过离线回放上限", err)
	case errors.Is(err, geometry.ErrGeometryTooLarge):
		return api.Unprocessable("REPLAY_GRID_TOO_LARGE", "扫幅与分辨率生成的回放网格过大", err)
	default:
		return api.Unprocessable("REPLAY_CALCULATION_INVALID", "回放计算参数或几何无效", err)
	}
}

func replayView(item model.RunReplay) (ReplayView, error) {
	var summary geometry.ReplaySummary
	if err := json.Unmarshal(item.SummaryJSON, &summary); err != nil {
		return ReplayView{}, fmt.Errorf("parse frozen replay summary: %w", err)
	}
	return ReplayView{Replay: item, Frames: json.RawMessage(item.FramesGeoJSON), Summary: summary}, nil
}
