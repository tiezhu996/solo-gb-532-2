package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"sonar-survey-coverage-planner/backend/internal/constants"
	"sonar-survey-coverage-planner/backend/internal/dto"
	"sonar-survey-coverage-planner/backend/internal/model"
	"sonar-survey-coverage-planner/backend/internal/repository"
	"sonar-survey-coverage-planner/backend/pkg/api"
)

const replayTestBoundary = `{"type":"Feature","properties":{"name":"回放测区"},"geometry":{"type":"Polygon","coordinates":[[[0,0],[1000,0],[1000,600],[0,600],[0,0]]]}}`
const replayTestLines = `{"type":"Feature","properties":{"name":"回放测线"},"geometry":{"type":"MultiLineString","coordinates":[[[40,100],[960,100]]]}}`
const replayTestTrack = `{"type":"Feature","properties":{"source":"test"},"geometry":{"type":"MultiLineString","coordinates":[[[30,100],[970,100]],[[30,300],[970,300]],[[30,500],[970,500]]]}}`

func replayServiceFixture(t *testing.T) (*RunReplayService, *gorm.DB, model.SonarRun) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent), TranslateError: true})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.SurveyArea{}, &model.TransectPlan{}, &model.SonarRun{}, &model.CoverageGap{}, &model.RunReplay{}, &model.AuditEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	area := model.SurveyArea{AreaCode: "REPLAY-A01", Name: "回放测区", BoundaryGeoJSON: []byte(replayTestBoundary), TargetResolutionM: 20, CoordinateSystem: "EPSG:32650", DefaultSwathM: 180, OwnerTeam: "测试组", Status: constants.AreaActive, Version: 1}
	if err := db.Create(&area).Error; err != nil {
		t.Fatalf("create area: %v", err)
	}
	plan := model.TransectPlan{SurveyAreaID: area.ID, Name: "回放测线", LineGeoJSON: []byte(replayTestLines), PlannedHeading: 90, PlannedSwathM: 180, LineSpacingM: 200, PlanState: constants.PlanLocked, Version: 1, CreatedBy: 1}
	if err := db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	run := seedReplayRun(t, db, plan.ID, "RUN-REPLAY-OK", replayTestTrack, string(constants.RunProcessed), "checksum-replay-ok")
	service := NewRunReplayService(repository.NewRunReplayRepository(db), repository.NewSonarRunRepository(db), NewAuditService(repository.NewSupportRepository(db)))
	return service, db, run
}

func seedReplayRun(t *testing.T, db *gorm.DB, planID uint, code, track, state, checksum string) model.SonarRun {
	t.Helper()
	started := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	run := model.SonarRun{TransectPlanID: planID, RunCode: code, TrackGeoJSON: []byte(track), ActualSwathM: 170, StartedAt: started, EndedAt: started.Add(70 * time.Minute), NavigationQuality: constants.NavGood, RunState: state, SourceChecksum: checksum, ImportedBy: 1, Version: 1}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("create run %s: %v", code, err)
	}
	return run
}

func replayActor() Actor {
	return Actor{RequestID: "req-replay-test", UserID: 1, Username: "processor", Role: constants.RoleDataProcessor}
}

func countRows(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var total int64
	if err := db.Table(table).Count(&total).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return total
}

func assertAppError(t *testing.T, err error, status int, code string) {
	t.Helper()
	var appErr *api.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("error %v is not an AppError", err)
	}
	if appErr.Status != status || appErr.Code != code {
		t.Fatalf("error = %d %s, want %d %s", appErr.Status, appErr.Code, status, code)
	}
}

func TestRunReplayCreateSuccess(t *testing.T) {
	service, db, run := replayServiceFixture(t)
	view, err := service.Create(dto.CreateReplayRequest{RunID: run.ID}, replayActor())
	if err != nil {
		t.Fatalf("create replay: %v", err)
	}
	if view.Replay.ID == 0 || view.Replay.AlgorithmVersion != constants.ReplayAlgorithmV1 {
		t.Fatalf("replay = %+v", view.Replay)
	}
	if view.Replay.SampleCount != 6 || view.Summary.SampleCount != 6 {
		t.Fatalf("sample count = %d/%d, want 6", view.Replay.SampleCount, view.Summary.SampleCount)
	}
	if view.Summary.DroppedPoints != 0 || view.Summary.OverlapCells != 0 || view.Summary.AccuracyAnomalies != 0 {
		t.Fatalf("clean track must have zero quality findings: %+v", view.Summary)
	}
	var frames struct {
		Features []json.RawMessage `json:"features"`
	}
	if err := json.Unmarshal(view.Frames, &frames); err != nil {
		t.Fatalf("frames must be a frozen FeatureCollection: %v", err)
	}
	if len(frames.Features) != 6 {
		t.Fatalf("frozen frames = %d, want 6", len(frames.Features))
	}
	if len(view.Replay.InputHash) != 64 {
		t.Fatalf("input hash = %q, want 64 hex chars", view.Replay.InputHash)
	}
	if countRows(t, db, "run_replays") != 1 {
		t.Fatal("exactly one replay snapshot must persist")
	}
	if countRows(t, db, "audit_events") != 1 {
		t.Fatal("successful replay must write one audit event")
	}
	loaded, err := service.Get(view.Replay.ID)
	if err != nil {
		t.Fatalf("get replay: %v", err)
	}
	if loaded.Summary.SampleCount != 6 || len(loaded.Frames) == 0 {
		t.Fatalf("frozen snapshot must load back: %+v", loaded.Summary)
	}
}

func TestRunReplayCreateErrorFlows(t *testing.T) {
	service, db, run := replayServiceFixture(t)
	if _, err := service.Create(dto.CreateReplayRequest{RunID: run.ID}, replayActor()); err != nil {
		t.Fatalf("seed replay: %v", err)
	}
	var plan model.TransectPlan
	if err := db.First(&plan, run.TransectPlanID).Error; err != nil {
		t.Fatalf("load plan: %v", err)
	}
	imported := seedReplayRun(t, db, plan.ID, "RUN-REPLAY-NEW", replayTestTrack, string(constants.RunImported), "checksum-replay-new")
	declaredTrack := `{"type":"Feature","properties":{"sample_count":10},"geometry":{"type":"MultiLineString","coordinates":[[[30,100],[970,100]],[[30,300],[970,300]],[[30,500],[970,500]]]}}`
	declared := seedReplayRun(t, db, plan.ID, "RUN-REPLAY-DECLARED", declaredTrack, string(constants.RunProcessed), "checksum-replay-declared")
	outsideTrack := `{"type":"Feature","properties":{},"geometry":{"type":"MultiLineString","coordinates":[[[30,100],[970,100]],[[30,300],[1200,300]]]}}`
	outside := seedReplayRun(t, db, plan.ID, "RUN-REPLAY-OUTSIDE", outsideTrack, string(constants.RunProcessed), "checksum-replay-outside")

	cases := []struct {
		name    string
		request dto.CreateReplayRequest
		status  int
		code    string
	}{
		{"非法运行不存在", dto.CreateReplayRequest{RunID: 99999}, 404, "RESOURCE_NOT_FOUND"},
		{"非法运行未处理", dto.CreateReplayRequest{RunID: imported.ID}, 409, "RUN_NOT_PROCESSED"},
		{"重复回放", dto.CreateReplayRequest{RunID: run.ID}, 409, "REPLAY_DUPLICATE"},
		{"请求声明采样缺失", dto.CreateReplayRequest{RunID: run.ID, ExpectedSamples: 99}, 422, "REPLAY_SAMPLE_MISSING"},
		{"航迹声明采样缺失", dto.CreateReplayRequest{RunID: declared.ID}, 422, "REPLAY_SAMPLE_MISSING"},
		{"坐标越界", dto.CreateReplayRequest{RunID: outside.ID}, 422, "REPLAY_COORDINATE_OUT_OF_BOUNDS"},
		{"算法版本不支持", dto.CreateReplayRequest{RunID: run.ID, AlgorithmVersion: "replay-v99"}, 422, "REPLAY_ALGORITHM_UNSUPPORTED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.Create(tc.request, replayActor()); err == nil {
				t.Fatalf("expected %s error", tc.code)
			} else {
				assertAppError(t, err, tc.status, tc.code)
			}
		})
	}

	t.Run("错误流不改原有数据", func(t *testing.T) {
		if total := countRows(t, db, "run_replays"); total != 1 {
			t.Fatalf("replays = %d, want only the seeded success", total)
		}
		if total := countRows(t, db, "audit_events"); total != 1 {
			t.Fatalf("audits = %d, failed replays must not audit", total)
		}
		for _, id := range []uint{run.ID, imported.ID, declared.ID, outside.ID} {
			var stored model.SonarRun
			if err := db.First(&stored, id).Error; err != nil {
				t.Fatalf("load run %d: %v", id, err)
			}
			if stored.Version != 1 {
				t.Fatalf("run %d version = %d, replay errors must not touch runs", id, stored.Version)
			}
		}
		var stored model.SonarRun
		if err := db.First(&stored, imported.ID).Error; err != nil {
			t.Fatalf("load imported run: %v", err)
		}
		if stored.RunState != string(constants.RunImported) {
			t.Fatalf("run state = %s, want unchanged imported", stored.RunState)
		}
	})
}

func TestRunReplayDuplicateDoesNotReplaceSnapshot(t *testing.T) {
	service, db, run := replayServiceFixture(t)
	first, err := service.Create(dto.CreateReplayRequest{RunID: run.ID}, replayActor())
	if err != nil {
		t.Fatalf("create replay: %v", err)
	}
	if _, err := service.Create(dto.CreateReplayRequest{RunID: run.ID}, replayActor()); err == nil {
		t.Fatal("duplicate replay must be rejected")
	} else {
		assertAppError(t, err, 409, "REPLAY_DUPLICATE")
	}
	var stored model.RunReplay
	if err := db.First(&stored, first.Replay.ID).Error; err != nil {
		t.Fatalf("load replay: %v", err)
	}
	if stored.InputHash != first.Replay.InputHash || stored.SampleCount != 6 {
		t.Fatal("original snapshot must stay intact after duplicate rejection")
	}
	if countRows(t, db, "run_replays") != 1 {
		t.Fatal("duplicate replay must not create a second snapshot")
	}
}

func TestRunReplayListFiltersByRun(t *testing.T) {
	service, db, run := replayServiceFixture(t)
	if _, err := service.Create(dto.CreateReplayRequest{RunID: run.ID}, replayActor()); err != nil {
		t.Fatalf("create replay: %v", err)
	}
	items, total, err := service.List(dto.RunReplayQuery{RunID: run.ID, Page: 1, PageSize: 20})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("list = %d/%d err=%v, want 1", len(items), total, err)
	}
	items, total, err = service.List(dto.RunReplayQuery{RunID: run.ID + 100, Page: 1, PageSize: 20})
	if err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("filtered list = %d/%d err=%v, want empty", len(items), total, err)
	}
	_ = db
}
