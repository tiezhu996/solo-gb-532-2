package repository

import (
	"fmt"

	"gorm.io/gorm"
	"sonar-survey-coverage-planner/backend/internal/dto"
	"sonar-survey-coverage-planner/backend/internal/model"
)

type RunReplayRepository struct{ db *gorm.DB }

func NewRunReplayRepository(db *gorm.DB) *RunReplayRepository {
	return &RunReplayRepository{db: db}
}

// replayListColumns 列表页只读标量列，避免把大体积冻结帧带出数据库。
var replayListColumns = []string{"id", "run_id", "algorithm_version", "input_hash", "sample_count", "dropped_points", "dropout_events", "overlap_cells", "overlap_ratio", "accuracy_anomalies", "created_by", "created_at"}

func (r *RunReplayRepository) List(query dto.RunReplayQuery) ([]model.RunReplay, int64, error) {
	db := r.db.Model(&model.RunReplay{})
	if query.RunID > 0 {
		db = db.Where("run_id = ?", query.RunID)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count run replays: %w", err)
	}
	var items []model.RunReplay
	if err := db.Select(replayListColumns).Preload("Run").Order("created_at DESC, id DESC").Offset((query.Page - 1) * query.PageSize).Limit(query.PageSize).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("list run replays: %w", err)
	}
	return items, total, nil
}

func (r *RunReplayRepository) Get(id uint) (model.RunReplay, error) {
	var item model.RunReplay
	if err := r.db.Preload("Run.TransectPlan.SurveyArea").First(&item, id).Error; err != nil {
		return item, fmt.Errorf("get run replay: %w", err)
	}
	return item, nil
}

func (r *RunReplayRepository) ByInputHash(hash string) (model.RunReplay, error) {
	var item model.RunReplay
	if err := r.db.Where("input_hash = ?", hash).First(&item).Error; err != nil {
		return item, fmt.Errorf("find replay input hash: %w", err)
	}
	return item, nil
}

func (r *RunReplayRepository) Create(item *model.RunReplay) error {
	if err := r.db.Create(item).Error; err != nil {
		return fmt.Errorf("create run replay: %w", err)
	}
	return nil
}
