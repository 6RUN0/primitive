package primitive

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// buildNoisyTarget returns a deterministic picture with gradients and a block
// pattern, so that the search has to pick between genuinely different shapes.
func buildNoisyTarget(width, height int) *image.NRGBA {
	target := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			target.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 255 / width),
				G: uint8(y * 255 / height),
				B: uint8(((x / 8) ^ (y / 8)) * 37),
				A: 255,
			})
		}
	}
	return target
}

// runModel returns the SVG produced by steps shapes of one model.
func runModel(target image.Image, background Color, workers int, seed int64, shapeType ShapeType, steps int) string {
	model := NewModel(target, background, 64, workers, seed)
	for i := 0; i < steps; i++ {
		model.Step(shapeType, 128, 0, 0.5)
	}
	return model.SVG()
}

// TestSeedRepeatsOutput pins the -seed contract for one and for several
// workers: a clock-based Worker.Rnd gives a new picture on every run.
func TestSeedRepeatsOutput(t *testing.T) {
	target := buildNoisyTarget(32, 32)
	background := MakeColor(AverageImageColor(target))
	for _, workers := range []int{1, 4} {
		first := runModel(target, background, workers, 42, ShapeTypeTriangle, 3)
		second := runModel(target, background, workers, 42, ShapeTypeTriangle, 3)
		if first != second {
			t.Errorf("j=%d: two runs with one seed differ", workers)
		}
		if other := runModel(target, background, workers, 43, ShapeTypeTriangle, 3); other == first {
			t.Errorf("j=%d: seeds 42 and 43 gave one output", workers)
		}
	}
}

// TestWorkersSearchDifferentShapes pins per-worker streams: with one seed for all
// workers the extra workers repeat the search of the first one.
func TestWorkersSearchDifferentShapes(t *testing.T) {
	model := NewModel(buildNoisyTarget(32, 32), Color{0, 0, 0, 255}, 64, 4, 7)
	seen := map[int]bool{}
	for _, worker := range model.Workers {
		seen[worker.Rnd.Int()] = true
	}
	if len(seen) != len(model.Workers) {
		t.Errorf("got %d distinct streams, want %d", len(seen), len(model.Workers))
	}
}

// TestAdjacentSeedsGiveDifferentOutput pins deriveWorkerSeed for the case most
// sensitive to shared streams: with seed+index two adjacent seeds share 7 of 8
// workers, and one step gives one picture on 17 of 20 such pairs.
func TestAdjacentSeedsGiveDifferentOutput(t *testing.T) {
	target := buildNoisyTarget(32, 32)
	background := MakeColor(AverageImageColor(target))
	for seed := int64(1); seed <= 20; seed++ {
		first := runModel(target, background, 8, seed, ShapeTypeTriangle, 1)
		second := runModel(target, background, 8, seed+1, ShapeTypeTriangle, 1)
		if first == second {
			t.Errorf("seeds %d and %d gave one output", seed, seed+1)
		}
	}
}

// TestDistantSeedsGiveDifferentOutput pins values that math/rand reduces modulo
// 2^31-1: with seed+index each of these worker sets holds a stream equal to 0
// modulo that prime, and the four seeds give one picture.
func TestDistantSeedsGiveDifferentOutput(t *testing.T) {
	target := buildNoisyTarget(32, 32)
	background := MakeColor(AverageImageColor(target))
	seen := map[string]int64{}
	for _, seed := range []int64{-3, -1, 2147483647, math.MaxInt64} {
		output := runModel(target, background, 4, seed, ShapeTypeTriangle, 2)
		if other, isSeen := seen[output]; isSeen {
			t.Errorf("seeds %d and %d gave one output", other, seed)
		}
		seen[output] = seed
	}
}

// TestDeriveWorkerSeedSplitsStreams pins distinct worker seeds around the values
// where a plain sum wraps or meets its neighbors.
func TestDeriveWorkerSeedSplitsStreams(t *testing.T) {
	seen := map[int64]bool{}
	seeds := []int64{math.MinInt64, -2, -1, 1, 2, 2147483647, 2147483648, math.MaxInt64}
	for _, seed := range seeds {
		for index := 0; index < 16; index++ {
			derived := deriveWorkerSeed(seed, index)
			if seen[derived] {
				t.Errorf("seed %d, worker %d repeats a derived seed", seed, index)
			}
			seen[derived] = true
		}
	}
}

// TestEqualEnergyPickIgnoresWorkerTiming pins the tie-break in runWorkers. On a
// target equal to the background every shape scores the same, so a pick by
// arrival order on the result channel would depend on the goroutine scheduler.
func TestEqualEnergyPickIgnoresWorkerTiming(t *testing.T) {
	background := Color{100, 100, 100, 255}
	target := uniformRGBA(image.Rect(0, 0, 16, 16), background.NRGBA())
	want := runModel(target, background, 8, 11, ShapeTypeRectangle, 1)
	for i := 0; i < 20; i++ {
		if got := runModel(target, background, 8, 11, ShapeTypeRectangle, 1); got != want {
			t.Fatalf("run %d differs:\n%s\nwant:\n%s", i, got, want)
		}
	}
}

// TestEqualEnergyPicksFirstWorker pins the rule behind the repeatable pick: any
// fixed choice would do, and <= in runWorkers would move it to the last worker.
func TestEqualEnergyPicksFirstWorker(t *testing.T) {
	background := Color{100, 100, 100, 255}
	target := uniformRGBA(image.Rect(0, 0, 16, 16), background.NRGBA())
	model := NewModel(target, background, 16, 8, 11)
	state := model.runWorkers(ShapeTypeRectangle, 128, 10, 10, 8, 0.5)
	if state.Worker != model.Workers[0] {
		t.Error("the winner of a tie is not the first worker")
	}
}

// TestStepRunsAnyWorkerCount pins the share of climbs per worker: with numWorkers
// 3 a rounded-down share loses climbs, and with 16 a share of zero leaves a worker
// without a state. The counts are set here, so the result does not depend on the
// cores of the host.
func TestStepRunsAnyWorkerCount(t *testing.T) {
	for _, workers := range []int{3, 16} {
		model := NewModel(buildNoisyTarget(8, 8), Color{0, 0, 0, 255}, 8, workers, 1)
		model.Step(ShapeTypeTriangle, 128, 0, 0.5)
		if len(model.Shapes) != 1 {
			t.Errorf("workers %d: got %d shapes, want 1", workers, len(model.Shapes))
		}
	}
}

// TestClimbsTotalIndependentOfWorkers pins the climb budget: n=1 and age=0 make one
// climb cost one Energy call on an exact mode, so the counter is the number of
// climbs. Rounding the share up per worker gives 9 climbs at j=3, and one climb per
// worker gives 16 at j=16, with twice the CPU.
func TestClimbsTotalIndependentOfWorkers(t *testing.T) {
	for _, workers := range []int{1, 2, 3, 4, 5, 7, 8, 9, 16} {
		model := NewModel(buildNoisyTarget(8, 8), Color{0, 0, 0, 255}, 8, workers, 1)
		model.runWorkers(ShapeTypeQuadratic, 128, 1, 0, 8, 0.5)
		climbs := 0
		for _, worker := range model.Workers {
			climbs += worker.Counter
		}
		if climbs != 8 {
			t.Errorf("workers %d: got %d climbs, want 8", workers, climbs)
		}
	}
}

// TestEnergyCallsStopGrowingPastClimbs pins the CPU cost of a large -j: at j=16
// the first 8 workers repeat j=8 and the other 8 stay idle. An idle worker spends
// nothing, so the Energy calls match up to one percent and the pictures are equal.
func TestEnergyCallsStopGrowingPastClimbs(t *testing.T) {
	target := buildNoisyTarget(32, 32)
	background := MakeColor(AverageImageColor(target))
	run := func(workers int) (int, string) {
		model := NewModel(target, background, 64, workers, 5)
		calls := 0
		for step := 0; step < 3; step++ {
			calls += model.Step(ShapeTypeTriangle, 128, 0, 0.5)
		}
		return calls, model.SVG()
	}
	callsAt8, svgAt8 := run(8)
	callsAt16, svgAt16 := run(16)
	if float64(callsAt16) > 1.01*float64(callsAt8) {
		t.Errorf("Energy calls at j=16: %d, at j=8: %d", callsAt16, callsAt8)
	}
	if svgAt16 != svgAt8 {
		t.Error("j=16 and j=8 gave different pictures")
	}
}
