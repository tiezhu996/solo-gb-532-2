package constants

// 运行质量回放共享常量。回放算法版本随结果持久化，便于追溯快照由哪一版算法生成。
const (
	ReplayAlgorithmV1 = "replay-v1"

	ReplayFlagDropout  = "dropout"
	ReplayFlagOverlap  = "overlap"
	ReplayFlagAccuracy = "accuracy"

	ReplayDefaultDropoutFactor      = 2.5
	ReplayDefaultAccuracyThresholdM = 10.0
	ReplayDefaultRevisitWindow      = 3
	ReplayMaxSamples                = 5000

	ReplayNavAccuracyGood     = 1.5
	ReplayNavAccuracyDegraded = 6.0
	ReplayNavAccuracyInvalid  = 15.0
)

// NavBaseAccuracy 把导航质量枚举映射为回放算法的基准定位误差（米）。
func NavBaseAccuracy(quality string) float64 {
	switch quality {
	case NavGood:
		return ReplayNavAccuracyGood
	case NavDegraded:
		return ReplayNavAccuracyDegraded
	default:
		return ReplayNavAccuracyInvalid
	}
}

// SupportedReplayAlgorithm 报告请求的算法版本是否受支持。
func SupportedReplayAlgorithm(version string) bool {
	return version == ReplayAlgorithmV1
}
