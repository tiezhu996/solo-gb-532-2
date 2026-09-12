package geometry

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"

	"sonar-survey-coverage-planner/backend/internal/constants"
)

func replayTestParams() ReplayParams {
	return ReplayParams{DropoutFactor: 2.5, AccuracyThresholdM: 10, OverlapResolutionM: 20, RevisitWindow: 3}
}

func replayTestTimes() (time.Time, time.Time) {
	started := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	return started, started.Add(60 * time.Minute)
}

func TestBuildReplayStraightTrack(t *testing.T) {
	lines := []orb.LineString{{{30, 100}, {970, 100}}, {{30, 300}, {970, 300}}, {{30, 500}, {970, 500}}}
	started, ended := replayTestTimes()
	frames, summary, err := BuildReplay(lines, 170, constants.ReplayNavAccuracyGood, started, ended, replayTestParams())
	if err != nil {
		t.Fatalf("build replay: %v", err)
	}
	if len(frames) != 6 || summary.SampleCount != 6 {
		t.Fatalf("frames = %d, samples = %d, want 6", len(frames), summary.SampleCount)
	}
	if summary.DroppedPoints != 0 || summary.DropoutEvents != 0 {
		t.Fatalf("straight parallel track must not report dropouts: %+v", summary.DropoutGaps)
	}
	if summary.OverlapCells != 0 || summary.OverlapRatio != 0 {
		t.Fatalf("200 m spaced lines with 170 m swath must not overlap: cells=%d ratio=%.3f", summary.OverlapCells, summary.OverlapRatio)
	}
	if summary.AccuracyAnomalies != 0 {
		t.Fatalf("straight track must not report accuracy anomalies: %d", summary.AccuracyAnomalies)
	}
	for index, frame := range frames {
		if frame.Seq != index {
			t.Fatalf("frame seq = %d, want %d", frame.Seq, index)
		}
		if len(frame.Flags) != 0 {
			t.Fatalf("frame %d unexpected flags %v", index, frame.Flags)
		}
	}
	if math.Abs(frames[5].CumulativeM-frames[4].CumulativeM-940) > 0.001 {
		t.Fatalf("last line cumulative step = %.2f, want 940", frames[5].CumulativeM-frames[4].CumulativeM)
	}
	if summary.CoveredCells == 0 {
		t.Fatal("covered cells must be positive")
	}
	if math.Abs(summary.DurationMinutes-60) > 0.001 {
		t.Fatalf("duration = %.2f, want 60", summary.DurationMinutes)
	}
}

func TestBuildReplayDetectsDropout(t *testing.T) {
	lines := []orb.LineString{{{0, 0}, {10, 0}, {20, 0}, {30, 0}, {130, 0}, {140, 0}}}
	started, ended := replayTestTimes()
	frames, summary, err := BuildReplay(lines, 20, constants.ReplayNavAccuracyGood, started, ended, replayTestParams())
	if err != nil {
		t.Fatalf("build replay: %v", err)
	}
	if summary.DropoutEvents != 1 {
		t.Fatalf("dropout events = %d, want 1", summary.DropoutEvents)
	}
	gap := summary.DropoutGaps[0]
	if gap.AfterSeq != 3 || math.Abs(gap.GapM-100) > 0.001 || gap.EstimatedMissing != 9 {
		t.Fatalf("gap = %+v, want after_seq 3, 100 m, 9 missing", gap)
	}
	if summary.DroppedPoints != 9 {
		t.Fatalf("dropped points = %d, want 9", summary.DroppedPoints)
	}
	if !containsFlag(frames[4].Flags, constants.ReplayFlagDropout) {
		t.Fatalf("resume sample 4 must carry dropout flag: %v", frames[4].Flags)
	}
	if containsFlag(frames[3].Flags, constants.ReplayFlagDropout) {
		t.Fatal("sample before the gap must not be flagged")
	}
}

func TestBuildReplayDropoutAcrossLineBreak(t *testing.T) {
	lines := []orb.LineString{{{0, 0}, {10, 0}}, {{110, 0}, {120, 0}}}
	started, ended := replayTestTimes()
	_, summary, err := BuildReplay(lines, 20, constants.ReplayNavAccuracyGood, started, ended, replayTestParams())
	if err != nil {
		t.Fatalf("build replay: %v", err)
	}
	if summary.DropoutEvents != 1 || summary.DroppedPoints != 9 {
		t.Fatalf("line break gap must count as dropout: events=%d dropped=%d", summary.DropoutEvents, summary.DroppedPoints)
	}
}

func TestBuildReplayDropoutBoundaryNotFlagged(t *testing.T) {
	// 间距恰好在 DropoutFactor × 中位数阈值上：严格大于才判定丢点。
	lines := []orb.LineString{{{0, 0}, {10, 0}, {35, 0}, {45, 0}}}
	started, ended := replayTestTimes()
	_, summary, err := BuildReplay(lines, 20, constants.ReplayNavAccuracyGood, started, ended, replayTestParams())
	if err != nil {
		t.Fatalf("build replay: %v", err)
	}
	if summary.DropoutEvents != 0 {
		t.Fatalf("gap exactly at threshold must not count: %+v", summary.DropoutGaps)
	}
}

func TestBuildReplayDetectsOverlap(t *testing.T) {
	outbound := orb.LineString{}
	return_ := orb.LineString{}
	for index := 0; index <= 4; index++ {
		outbound = append(outbound, orb.Point{float64(index) * 50, 0})
		return_ = append(return_, orb.Point{float64(4-index) * 50, 5})
	}
	lines := []orb.LineString{append(outbound, return_...)}
	started, ended := replayTestTimes()
	frames, summary, err := BuildReplay(lines, 40, constants.ReplayNavAccuracyGood, started, ended, replayTestParams())
	if err != nil {
		t.Fatalf("build replay: %v", err)
	}
	if summary.OverlapCells == 0 || summary.OverlapRatio <= 0 {
		t.Fatalf("re-surveyed corridor must report overlap: %+v", summary)
	}
	if !containsFlag(frames[9].Flags, constants.ReplayFlagOverlap) {
		t.Fatalf("final re-survey sample must carry overlap flag: %v", frames[9].Flags)
	}
	if containsFlag(frames[0].Flags, constants.ReplayFlagOverlap) {
		t.Fatal("first pass sample must not be flagged as overlap")
	}
}

func TestBuildReplayDetectsAccuracyAnomaly(t *testing.T) {
	lines := []orb.LineString{{{0, 0}, {10, 20}, {20, 0}, {30, 20}, {40, 0}}}
	started, ended := replayTestTimes()
	frames, summary, err := BuildReplay(lines, 20, constants.ReplayNavAccuracyGood, started, ended, replayTestParams())
	if err != nil {
		t.Fatalf("build replay: %v", err)
	}
	if summary.AccuracyAnomalies != 3 {
		t.Fatalf("anomalies = %d, want 3 interior zigzag samples", summary.AccuracyAnomalies)
	}
	if math.Abs(summary.MaxAccuracyM-21.5) > 0.001 {
		t.Fatalf("max accuracy = %.2f, want 21.5", summary.MaxAccuracyM)
	}
	if !containsFlag(frames[2].Flags, constants.ReplayFlagAccuracy) {
		t.Fatalf("zigzag sample must carry accuracy flag: %v", frames[2].Flags)
	}
	if containsFlag(frames[0].Flags, constants.ReplayFlagAccuracy) {
		t.Fatal("line endpoint keeps base accuracy and must not be flagged")
	}
}

func TestBuildReplayAccuracyBoundaryNotFlagged(t *testing.T) {
	// 精度恰好在阈值上：严格大于才判定异常。
	lines := []orb.LineString{{{0, 0}, {50, 0}, {100, 0}}}
	started, ended := replayTestTimes()
	params := replayTestParams()
	params.AccuracyThresholdM = constants.ReplayNavAccuracyGood
	_, summary, err := BuildReplay(lines, 20, constants.ReplayNavAccuracyGood, started, ended, params)
	if err != nil {
		t.Fatalf("build replay: %v", err)
	}
	if summary.AccuracyAnomalies != 0 {
		t.Fatalf("accuracy exactly at threshold must not count: %d", summary.AccuracyAnomalies)
	}
}

func TestBuildReplayRejectsInvalidInput(t *testing.T) {
	started, ended := replayTestTimes()
	if _, _, err := BuildReplay([]orb.LineString{{{0, 0}}}, 20, 1.5, started, ended, replayTestParams()); err == nil {
		t.Fatal("single sample must be rejected")
	}
	tooMany := orb.LineString{}
	for index := 0; index <= constants.ReplayMaxSamples; index++ {
		tooMany = append(tooMany, orb.Point{float64(index), 0})
	}
	if _, _, err := BuildReplay([]orb.LineString{tooMany}, 20, 1.5, started, ended, replayTestParams()); !errors.Is(err, ErrTooManySamples) {
		t.Fatalf("expected ErrTooManySamples, got %v", err)
	}
	params := replayTestParams()
	params.OverlapResolutionM = 1
	if _, _, err := BuildReplay([]orb.LineString{{{0, 0}, {100, 0}}}, 2000, 1.5, started, ended, params); !errors.Is(err, ErrGeometryTooLarge) {
		t.Fatalf("expected ErrGeometryTooLarge, got %v", err)
	}
}

func TestFirstOutsideBoundary(t *testing.T) {
	boundary := orb.Polygon{orb.Ring{{0, 0}, {100, 0}, {100, 100}, {0, 100}, {0, 0}}}
	if _, _, found := FirstOutsideBoundary([]orb.LineString{{{10, 10}, {50, 50}}}, boundary); found {
		t.Fatal("track fully inside must not be flagged")
	}
	seq, point, found := FirstOutsideBoundary([]orb.LineString{{{10, 10}, {150, 50}}}, boundary)
	if !found || seq != 1 || point[0] != 150 {
		t.Fatalf("outside seq = %d point = %v found = %v, want seq 1 at x 150", seq, point, found)
	}
}

func TestDeclaredSampleCount(t *testing.T) {
	declared, found, err := DeclaredSampleCount([]byte(`{"type":"Feature","properties":{"sample_count":4},"geometry":{"type":"LineString","coordinates":[[0,0],[1,1]]}}`))
	if err != nil || !found || declared != 4 {
		t.Fatalf("declared = %d found = %v err = %v, want 4/true/nil", declared, found, err)
	}
	_, found, err = DeclaredSampleCount([]byte(`{"type":"Feature","properties":{"source":"demo"},"geometry":{"type":"LineString","coordinates":[[0,0],[1,1]]}}`))
	if err != nil || found {
		t.Fatalf("missing property must parse as absent: found=%v err=%v", found, err)
	}
	if _, _, err = DeclaredSampleCount([]byte(`{"type":"Feature","properties":{"sample_count":"many"},"geometry":{"type":"LineString","coordinates":[[0,0],[1,1]]}}`)); err == nil {
		t.Fatal("non-numeric sample_count must be rejected")
	}
	if _, _, err = DeclaredSampleCount([]byte(`{"type":"Feature","properties":{"sample_count":2.5},"geometry":{"type":"LineString","coordinates":[[0,0],[1,1]]}}`)); err == nil {
		t.Fatal("fractional sample_count must be rejected")
	}
}

func TestReplayInputHashStable(t *testing.T) {
	params := replayTestParams()
	first := ReplayInputHash("checksum-a", constants.ReplayAlgorithmV1, params, 0)
	second := ReplayInputHash("checksum-a", constants.ReplayAlgorithmV1, params, 0)
	if first != second || len(first) != 64 {
		t.Fatalf("hash must be stable 64 hex chars: %q vs %q", first, second)
	}
	if ReplayInputHash("checksum-a", constants.ReplayAlgorithmV1, params, 12) == first {
		t.Fatal("expected samples must participate in hash")
	}
	changed := params
	changed.AccuracyThresholdM = 12
	if ReplayInputHash("checksum-a", constants.ReplayAlgorithmV1, changed, 0) == first {
		t.Fatal("algorithm params must participate in hash")
	}
	if ReplayInputHash("checksum-b", constants.ReplayAlgorithmV1, params, 0) == first {
		t.Fatal("run checksum must participate in hash")
	}
}

func TestReplayFramesFeatureCollectionRoundTrip(t *testing.T) {
	lines := []orb.LineString{{{0, 0}, {10, 0}, {20, 0}}}
	started, ended := replayTestTimes()
	frames, _, err := BuildReplay(lines, 20, constants.ReplayNavAccuracyGood, started, ended, replayTestParams())
	if err != nil {
		t.Fatalf("build replay: %v", err)
	}
	data, err := ReplayFramesFeatureCollection(frames)
	if err != nil {
		t.Fatalf("marshal frames: %v", err)
	}
	collection, err := geojson.UnmarshalFeatureCollection(data)
	if err != nil {
		t.Fatalf("frozen frames must stay valid GeoJSON: %v", err)
	}
	if len(collection.Features) != 3 {
		t.Fatalf("features = %d, want 3", len(collection.Features))
	}
	for index, feature := range collection.Features {
		seq, ok := feature.Properties["seq"].(float64)
		if !ok || int(seq) != index {
			t.Fatalf("feature %d seq property = %v", index, feature.Properties["seq"])
		}
		if _, ok := feature.Geometry.(orb.Point); !ok {
			t.Fatalf("feature %d geometry must be a point", index)
		}
	}
}

func containsFlag(flags []string, name string) bool {
	for _, flag := range flags {
		if flag == name {
			return true
		}
	}
	return false
}
