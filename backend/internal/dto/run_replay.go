package dto

// CreateReplayRequest 为一次已处理运行请求生成质量回放快照。
// 所有算法参数都参与输入哈希；缺省时由服务端填入默认值。
type CreateReplayRequest struct {
	RunID              uint    `json:"run_id" binding:"required,gt=0"`
	AlgorithmVersion   string  `json:"algorithm_version" binding:"omitempty,min=3,max=40"`
	ExpectedSamples    int     `json:"expected_samples" binding:"omitempty,gte=0"`
	DropoutFactor      float64 `json:"dropout_factor" binding:"omitempty,gte=1.5,lte=10"`
	AccuracyThresholdM float64 `json:"accuracy_threshold_m" binding:"omitempty,gt=0,lte=200"`
	OverlapResolutionM float64 `json:"overlap_resolution_m" binding:"omitempty,gt=0,lte=100"`
}

type RunReplayQuery struct {
	RunID    uint
	Page     int
	PageSize int
}
