package model

import (
	"time"

	"gorm.io/datatypes"
)

// RunReplay 是一次运行质量回放的固定快照：输入哈希、算法版本、冻结的回放帧与汇总。
// 快照创建后不可变，重复回放同一输入会被唯一索引拒绝。
type RunReplay struct {
	ID                uint           `json:"id" gorm:"primaryKey"`
	RunID             uint           `json:"run_id" gorm:"not null;index"`
	AlgorithmVersion  string         `json:"algorithm_version" gorm:"size:40;not null"`
	InputHash         string         `json:"input_hash" gorm:"size:64;not null;uniqueIndex"`
	SampleCount       int            `json:"sample_count" gorm:"not null"`
	DroppedPoints     int            `json:"dropped_points" gorm:"not null"`
	DropoutEvents     int            `json:"dropout_events" gorm:"not null"`
	OverlapCells      int            `json:"overlap_cells" gorm:"not null"`
	OverlapRatio      float64        `json:"overlap_ratio" gorm:"not null"`
	AccuracyAnomalies int            `json:"accuracy_anomalies" gorm:"not null"`
	FramesGeoJSON     datatypes.JSON `json:"frames_geojson" gorm:"column:frames_geojson;type:jsonb;not null"`
	SummaryJSON       datatypes.JSON `json:"summary_json" gorm:"column:summary_json;type:jsonb;not null"`
	CreatedBy         uint           `json:"created_by" gorm:"not null"`
	CreatedAt         time.Time      `json:"created_at"`
	Run               *SonarRun      `json:"run,omitempty" gorm:"foreignKey:RunID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT"`
}

func (RunReplay) TableName() string { return "run_replays" }
