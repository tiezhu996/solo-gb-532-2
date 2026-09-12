package geometry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"

	"sonar-survey-coverage-planner/backend/internal/constants"
)

// ErrTooManySamples 表示回放采样数超过离线处理上限。
var ErrTooManySamples = errors.New("replay sample count exceeds limit")

// ReplayParams 是回放算法的可调参数，全部参与输入哈希，保证快照可复现。
type ReplayParams struct {
	DropoutFactor      float64 `json:"dropout_factor"`
	AccuracyThresholdM float64 `json:"accuracy_threshold_m"`
	OverlapResolutionM float64 `json:"overlap_resolution_m"`
	RevisitWindow      int     `json:"revisit_window"`
}

// ReplayFrame 是单个采样序号的回放帧，质量标记来自 constants 中的回放标记枚举。
type ReplayFrame struct {
	Seq         int      `json:"seq"`
	X           float64  `json:"x"`
	Y           float64  `json:"y"`
	CumulativeM float64  `json:"cumulative_m"`
	AccuracyM   float64  `json:"accuracy_m"`
	Flags       []string `json:"flags"`
}

// DropoutGap 描述一次丢点事件：发生在 AfterSeq 之后、跨越 GapM 米、估算缺失 EstimatedMissing 个采样。
type DropoutGap struct {
	AfterSeq         int     `json:"after_seq"`
	GapM             float64 `json:"gap_m"`
	EstimatedMissing int     `json:"estimated_missing"`
}

// ReplaySummary 汇总丢点、重复覆盖和精度异常，随快照冻结保存。
type ReplaySummary struct {
	SampleCount        int          `json:"sample_count"`
	TrackLengthM       float64      `json:"track_length_m"`
	DurationMinutes    float64      `json:"duration_minutes"`
	DroppedPoints      int          `json:"dropped_points"`
	DropoutEvents      int          `json:"dropout_events"`
	DropoutGaps        []DropoutGap `json:"dropout_gaps"`
	CoveredCells       int          `json:"covered_cells"`
	OverlapCells       int          `json:"overlap_cells"`
	OverlapRatio       float64      `json:"overlap_ratio"`
	AccuracyAnomalies  int          `json:"accuracy_anomalies"`
	AccuracyThresholdM float64      `json:"accuracy_threshold_m"`
	MaxAccuracyM       float64      `json:"max_accuracy_m"`
	ResolutionM        float64      `json:"resolution_m"`
}

type replaySample struct {
	point     orb.Point
	lineStart bool
}

// CountSamples 统计航迹全部采样点数量。
func CountSamples(lines []orb.LineString) int {
	total := 0
	for _, line := range lines {
		total += len(line)
	}
	return total
}

// DeclaredSampleCount 读取航迹 Feature properties 中采集系统声明的 sample_count。
// 第二个返回值表示属性是否存在；存在但不是非负整数时返回错误。
func DeclaredSampleCount(data []byte) (int, bool, error) {
	feature, err := geojson.UnmarshalFeature(data)
	if err != nil {
		return 0, false, fmt.Errorf("%w: parse feature: %v", ErrInvalidGeometry, err)
	}
	raw, ok := feature.Properties["sample_count"]
	if !ok {
		return 0, false, nil
	}
	value, ok := raw.(float64)
	if !ok || value < 0 || math.Floor(value) != value {
		return 0, true, fmt.Errorf("%w: sample_count property must be a non-negative integer", ErrInvalidGeometry)
	}
	return int(value), true, nil
}

// FirstOutsideBoundary 按采样序号返回第一个落在测区边界外的点；全部在界内时 found 为 false。
func FirstOutsideBoundary(lines []orb.LineString, boundary orb.Polygon) (seq int, point orb.Point, found bool) {
	index := 0
	for _, line := range lines {
		for _, position := range line {
			if !polygonContains(boundary, position) {
				return index, position, true
			}
			index++
		}
	}
	return 0, orb.Point{}, false
}

// ReplayInputHash 对运行来源校验和、算法版本和全部回放参数做稳定哈希，用于重复回放检测。
func ReplayInputHash(runChecksum, algorithm string, params ReplayParams, expectedSamples int) string {
	payload, _ := json.Marshal(struct {
		RunChecksum string  `json:"run_checksum"`
		Algorithm   string  `json:"algorithm"`
		Dropout     float64 `json:"dropout_factor"`
		Accuracy    float64 `json:"accuracy_threshold_m"`
		Resolution  float64 `json:"overlap_resolution_m"`
		Window      int     `json:"revisit_window"`
		Expected    int     `json:"expected_samples"`
	}{runChecksum, algorithm, params.DropoutFactor, params.AccuracyThresholdM, params.OverlapResolutionM, params.RevisitWindow, expectedSamples})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// BuildReplay 按采样序号构建回放帧并汇总丢点、重复覆盖与精度异常。
// 丢点：相邻采样间距（含跨线断点）超过 DropoutFactor 倍间距中位数。
// 重复覆盖：以 OverlapResolutionM 网格栅格化扫幅，同一网格被间隔超过 RevisitWindow 个采样的两批采样覆盖。
// 精度异常：基准定位误差（导航质量）叠加航迹局部抖动，超过 AccuracyThresholdM。
func BuildReplay(lines []orb.LineString, swathM, baseAccuracyM float64, startedAt, endedAt time.Time, params ReplayParams) ([]ReplayFrame, ReplaySummary, error) {
	if swathM <= 0 || !finite(swathM) {
		return nil, ReplaySummary{}, fmt.Errorf("%w: swath must be positive", ErrInvalidGeometry)
	}
	if params.OverlapResolutionM <= 0 || !finite(params.OverlapResolutionM) {
		return nil, ReplaySummary{}, fmt.Errorf("%w: overlap resolution must be positive", ErrInvalidGeometry)
	}
	if params.RevisitWindow < 1 {
		return nil, ReplaySummary{}, fmt.Errorf("%w: revisit window must be at least one", ErrInvalidGeometry)
	}
	samples := flattenSamples(lines)
	if len(samples) < 2 {
		return nil, ReplaySummary{}, fmt.Errorf("%w: at least two samples are required", ErrInvalidGeometry)
	}
	if len(samples) > constants.ReplayMaxSamples {
		return nil, ReplaySummary{}, fmt.Errorf("%w: %d samples", ErrTooManySamples, len(samples))
	}
	median := medianSpacing(samples)
	frames := make([]ReplayFrame, len(samples))
	flagSets := make([]map[string]bool, len(samples))
	for index := range flagSets {
		flagSets[index] = map[string]bool{}
	}
	dropoutGaps := []DropoutGap{}
	dropped := 0
	cumulative := 0.0
	for index, sample := range samples {
		if index > 0 {
			gap := distance(samples[index-1].point, sample.point)
			cumulative += gap
			if gap > params.DropoutFactor*median && gap > 0 {
				missing := 0
				if median > 0 {
					missing = int(math.Round(gap/median)) - 1
				}
				if missing < 0 {
					missing = 0
				}
				dropoutGaps = append(dropoutGaps, DropoutGap{AfterSeq: index - 1, GapM: gap, EstimatedMissing: missing})
				dropped += missing
				flagSets[index][constants.ReplayFlagDropout] = true
			}
		}
		frames[index] = ReplayFrame{Seq: index, X: sample.point[0], Y: sample.point[1], CumulativeM: cumulative}
	}
	anomalies, maxAccuracy := markAccuracy(samples, frames, flagSets, baseAccuracyM, params.AccuracyThresholdM)
	covered, overlap, err := markOverlap(samples, swathM, params.OverlapResolutionM, params.RevisitWindow, flagSets)
	if err != nil {
		return nil, ReplaySummary{}, err
	}
	for index := range frames {
		flags := make([]string, 0, len(flagSets[index]))
		for _, name := range []string{constants.ReplayFlagDropout, constants.ReplayFlagAccuracy, constants.ReplayFlagOverlap} {
			if flagSets[index][name] {
				flags = append(flags, name)
			}
		}
		frames[index].Flags = flags
	}
	overlapRatio := 0.0
	if covered > 0 {
		overlapRatio = float64(overlap) / float64(covered)
	}
	summary := ReplaySummary{
		SampleCount: len(samples), TrackLengthM: TrackLength(lines), DurationMinutes: endedAt.Sub(startedAt).Minutes(),
		DroppedPoints: dropped, DropoutEvents: len(dropoutGaps), DropoutGaps: dropoutGaps,
		CoveredCells: covered, OverlapCells: overlap, OverlapRatio: overlapRatio,
		AccuracyAnomalies: anomalies, AccuracyThresholdM: params.AccuracyThresholdM, MaxAccuracyM: maxAccuracy,
		ResolutionM: params.OverlapResolutionM,
	}
	return frames, summary, nil
}

// ReplayFramesFeatureCollection 把回放帧冻结为 GeoJSON FeatureCollection 快照。
func ReplayFramesFeatureCollection(frames []ReplayFrame) ([]byte, error) {
	collection := geojson.NewFeatureCollection()
	for _, frame := range frames {
		feature := geojson.NewFeature(orb.Point{frame.X, frame.Y})
		feature.Properties = map[string]interface{}{
			"seq":          frame.Seq,
			"cumulative_m": frame.CumulativeM,
			"accuracy_m":   frame.AccuracyM,
			"flags":        frame.Flags,
		}
		collection.Append(feature)
	}
	return collection.MarshalJSON()
}

func flattenSamples(lines []orb.LineString) []replaySample {
	samples := make([]replaySample, 0)
	for _, line := range lines {
		for index, point := range line {
			samples = append(samples, replaySample{point: point, lineStart: index == 0})
		}
	}
	return samples
}

func medianSpacing(samples []replaySample) float64 {
	spacings := make([]float64, 0, len(samples))
	for index := 1; index < len(samples); index++ {
		if samples[index].lineStart {
			continue
		}
		spacings = append(spacings, distance(samples[index-1].point, samples[index].point))
	}
	if len(spacings) == 0 {
		return 0
	}
	sort.Float64s(spacings)
	middle := len(spacings) / 2
	if len(spacings)%2 == 1 {
		return spacings[middle]
	}
	return (spacings[middle-1] + spacings[middle]) / 2
}

func markAccuracy(samples []replaySample, frames []ReplayFrame, flagSets []map[string]bool, baseAccuracyM, thresholdM float64) (int, float64) {
	anomalies := 0
	maxAccuracy := 0.0
	for index, sample := range samples {
		deviation := 0.0
		if index > 0 && index < len(samples)-1 && !sample.lineStart && !samples[index+1].lineStart {
			deviation = pointSegmentDistance(sample.point, samples[index-1].point, samples[index+1].point)
		}
		accuracy := baseAccuracyM + deviation
		frames[index].AccuracyM = accuracy
		if accuracy > maxAccuracy {
			maxAccuracy = accuracy
		}
		if accuracy > thresholdM {
			anomalies++
			flagSets[index][constants.ReplayFlagAccuracy] = true
		}
	}
	return anomalies, maxAccuracy
}

func markOverlap(samples []replaySample, swathM, resolution float64, window int, flagSets []map[string]bool) (int, int, error) {
	radius := swathM / 2
	cellsPerSample := math.Pi * (radius / resolution) * (radius / resolution)
	if cellsPerSample > 5000 || cellsPerSample*float64(len(samples)) > 2_000_000 {
		return 0, 0, fmt.Errorf("%w: swath and resolution create too many replay cells", ErrGeometryTooLarge)
	}
	type cellKey struct{ x, y int }
	cellVisits := map[cellKey][]int{}
	stamp := func(sampleIndex int) {
		point := samples[sampleIndex].point
		minX := int(math.Floor((point[0] - radius) / resolution))
		maxX := int(math.Floor((point[0] + radius) / resolution))
		minY := int(math.Floor((point[1] - radius) / resolution))
		maxY := int(math.Floor((point[1] + radius) / resolution))
		for cx := minX; cx <= maxX; cx++ {
			for cy := minY; cy <= maxY; cy++ {
				center := orb.Point{(float64(cx) + 0.5) * resolution, (float64(cy) + 0.5) * resolution}
				if distance(center, point) <= radius {
					key := cellKey{cx, cy}
					cellVisits[key] = append(cellVisits[key], sampleIndex)
				}
			}
		}
	}
	for index := range samples {
		stamp(index)
	}
	overlap := 0
	for _, visits := range cellVisits {
		if len(visits) == 0 {
			continue
		}
		batches := 1
		for index := 1; index < len(visits); index++ {
			if visits[index]-visits[index-1] > window {
				batches++
			}
		}
		if batches >= 2 {
			overlap++
		}
	}
	for index, sample := range samples {
		key := cellKey{int(math.Floor(sample.point[0] / resolution)), int(math.Floor(sample.point[1] / resolution))}
		for _, earlier := range cellVisits[key] {
			if index-earlier > window {
				flagSets[index][constants.ReplayFlagOverlap] = true
				break
			}
		}
	}
	return len(cellVisits), overlap, nil
}
