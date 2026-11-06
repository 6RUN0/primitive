package primitive

import (
	"image"
	"math"
	"reflect"
	"sort"
	"testing"
)

// TestTablesFollowAdd pins the update after an accepted shape: Energy of the next
// shapes reads these sums, so a row missed by updateRows ranks them against a
// picture that no longer exists. Polygons and curves leave gaps between rows, and
// a shape can stop before the last column.
func TestTablesFollowAdd(t *testing.T) {
	for _, shapeType := range allShapeTypes {
		model := NewModel(buildNoisyTarget(40, 30), Color{128, 128, 128, 255}, 40, 1, 2)
		worker := model.Workers[0]
		for i := 0; i < 40; i++ {
			worker.Init(model.Current, model.Score)
			state := worker.RandomState(shapeType, 128, 4)
			state.Shape.Mutate()
			model.Add(state.Shape, state.Alpha)
		}
		fresh := newPixelTables(model.Target, model.Current)
		if !reflect.DeepEqual(model.tables.totals, fresh.totals) {
			t.Errorf("mode %d: updated tables differ from rebuilt ones", shapeType)
		}
	}
}

// TestTablesMatchPixelKernel pins the O(1) sums to the kernel they stand in for,
// on scanlines of one pixel, of a whole row and with a gap between runs.
func TestTablesMatchPixelKernel(t *testing.T) {
	target := imageToRGBA(buildNoisyTarget(16, 8))
	current := newUniformRGBA(16, 8, Color{30, 60, 90, 200})
	tables := newPixelTables(target, current)
	lines := []Scanline{
		{Y: 0, X1: 0, X2: 0, Alpha: 0xffff},
		{Y: 3, X1: 0, X2: 15, Alpha: 0xffff},
		{Y: 7, X1: 2, X2: 5, Alpha: 0x8000},
		{Y: 7, X1: 9, X2: 15, Alpha: 0x8000},
	}
	for _, alpha := range []int{1, 128, 255} {
		wantColor, wantError := colorAndError(target, current, lines, alpha)
		gotColor, gotError := tables.colorAndError(lines, alpha)
		if gotColor != wantColor || gotError != wantError {
			t.Errorf("alpha %d: got %+v, %d, want %+v, %d", alpha, gotColor, gotError, wantColor, wantError)
		}
	}
}

// TestLargeImageGoesWithoutTables pins the memory bound: 56 bytes per pixel on
// 8193 x 8192 pixels, one column above the limit, is past 3.5 GiB. The size alone
// decides, so the image here needs no pixels.
func TestLargeImageGoesWithoutTables(t *testing.T) {
	large := &image.RGBA{Rect: image.Rect(0, 0, 8193, 8192)}
	if newPixelTables(large, large) != nil {
		t.Error("got tables above maxTablePixels")
	}
	wide := &image.RGBA{Rect: image.Rect(0, 0, maxTableWidth+1, 1)}
	if newPixelTables(wide, wide) != nil {
		t.Error("got tables for a row above maxTableWidth")
	}
}

// TestEstimateOnWidestRow pins maxTableWidth: at this width all-white pixels bring
// the sums of squares of a row right below 2^32, and a limit one notch too high
// wraps them, so the estimate of a shape over the whole row falls apart.
func TestEstimateOnWidestRow(t *testing.T) {
	target := newUniformRGBA(maxTableWidth, 1, Color{255, 255, 255, 255})
	model := NewModel(target, Color{255, 255, 255, 255}, 16, 1, 1)
	worker := model.Workers[0]
	if worker.tables == nil {
		t.Fatal("no tables at maxTableWidth")
	}
	worker.Init(model.Current, model.Score)
	lines := []Scanline{{Y: 0, X1: 0, X2: maxTableWidth - 1, Alpha: 0xffff}}
	if estimate := worker.estimateEnergy(lines, 128); estimate > 0.01 {
		t.Errorf("estimate %v over a picture equal to the target", estimate)
	}
}

// TestEstimateRanksLikeExact pins what the search needs from estimateEnergy: the
// order of the candidates. The shape it prefers has to be among the best by the
// exact score, and the estimate has to stay close enough that a climb sees the same
// improvements. A wrong term in the expansion keeps the values plausible and
// shuffles the order.
func TestEstimateRanksLikeExact(t *testing.T) {
	for _, shapeType := range []ShapeType{ShapeTypeTriangle, ShapeTypeRectangle, ShapeTypeEllipse, ShapeTypeRotatedEllipse, ShapeTypePolygon} {
		for _, alpha := range []int{128, 40, 255} {
			model := NewModel(buildNoisyTarget(64, 64), Color{128, 128, 128, 255}, 64, 1, 3)
			for step := 0; step < 5; step++ {
				model.Step(ShapeTypeTriangle, 128, 0, 0.5)
			}
			worker := model.Workers[0]
			worker.Init(model.Current, model.Score)
			const count = 300
			exact := make([]float64, count)
			estimated := make([]float64, count)
			for i := range exact {
				shape := worker.RandomState(shapeType, alpha, 0.5).Shape
				exact[i] = worker.Energy(shape, alpha)
				estimated[i] = worker.estimateEnergy(shape.Rasterize(), alpha)
				if math.Abs(estimated[i]-exact[i]) > 1e-3 {
					t.Fatalf("mode %d alpha %d: estimate %v, exact %v", shapeType, alpha, estimated[i], exact[i])
				}
			}
			best := 0
			for i := range estimated {
				if estimated[i] < estimated[best] {
					best = i
				}
			}
			sorted := append([]float64(nil), exact...)
			sort.Float64s(sorted)
			if exact[best] > sorted[count/100] {
				t.Errorf("mode %d alpha %d: the estimate picks a shape outside the best 1%%: %v, best %v", shapeType, alpha, exact[best], sorted[0])
			}
		}
	}
}

// TestSearchEstimatesOnlyWhereValid pins the guard around estimateEnergy: it drops
// alpha, so on a transparent target or background it ranks shapes by a wrong error,
// and it reads the tables, so a large image has to go exact. Curves cost more on
// the estimate than on the pixels.
func TestSearchEstimatesOnlyWhereValid(t *testing.T) {
	opaque := buildNoisyTarget(16, 16)
	translucent := buildNoisyTarget(16, 16)
	translucent.Pix[3] = 200
	translucentLast := buildNoisyTarget(16, 16)
	translucentLast.Pix[len(translucentLast.Pix)-1] = 200
	gray := Color{128, 128, 128, 255}
	cases := []struct {
		name          string
		target        image.Image
		background    Color
		shapeType     ShapeType
		withoutTables bool
		want          bool
	}{
		{"opaque triangle", opaque, gray, ShapeTypeTriangle, false, true},
		{"opaque combo", opaque, gray, ShapeTypeAny, false, true},
		{"curve", opaque, gray, ShapeTypeQuadratic, false, false},
		{"transparent background", opaque, Color{128, 128, 128, 254}, ShapeTypeTriangle, false, false},
		{"translucent target", translucent, gray, ShapeTypeTriangle, false, false},
		{"last pixel translucent", translucentLast, gray, ShapeTypeTriangle, false, false},
		{"without tables", opaque, gray, ShapeTypeTriangle, true, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := NewModel(testCase.target, testCase.background, 16, 1, 1)
			if testCase.withoutTables {
				model.tables, model.Workers[0].tables = nil, nil
			}
			model.runWorkers(testCase.shapeType, 128, 2, 2, 1, 0.5)
			if got := model.Workers[0].isEstimating; got != testCase.want {
				t.Errorf("isEstimating = %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestClimbResultIsScoredExactly pins the check of finalists: workers compare the
// score of a climb, and an estimate there lets a shape win on rounding noise. The
// next search on the same worker has to run on estimates again.
func TestClimbResultIsScoredExactly(t *testing.T) {
	model := NewModel(buildNoisyTarget(32, 32), Color{128, 128, 128, 255}, 32, 1, 1)
	worker := model.Workers[0]
	state := model.runWorkers(ShapeTypeTriangle, 128, 5, 5, 2, 0.5)
	if want := refEnergy(worker, state.Shape, state.Alpha); state.Score != want {
		t.Errorf("score %v, exact %v", state.Score, want)
	}
	if !worker.isEstimating {
		t.Error("finalEnergy left the estimate off")
	}
}

// TestRepeatRunsOnExactScores pins -rep: its climb starts from the exact score of
// the winner, and mixing that with estimates decides moves on the rounding offset
// between the two. The score of the -rep shape comes from the exact kernels whatever
// the climb used, so the check is the energy that climb left behind.
func TestRepeatRunsOnExactScores(t *testing.T) {
	model := NewModel(buildNoisyTarget(32, 32), Color{128, 128, 128, 255}, 32, 1, 1)
	worker := model.Workers[0]
	state := model.runWorkers(ShapeTypeTriangle, 128, 5, 5, 1, 0.5)
	model.Add(state.Shape, state.Alpha)
	worker.Init(model.Current, model.Score)
	state = HillClimb(state, 20).(*State)
	if want := refEnergy(worker, state.Shape, state.Alpha); state.Score != want {
		t.Errorf("climb after Init: score %v, exact %v", state.Score, want)
	}
}

// TestEstimateHasNoBias pins the fraction terms of estimateEnergy. They add a small
// constant per pixel, so the order of candidates barely moves without them, but
// shapes of different area shift by different amounts. Without the term of 1/3 the
// mean falls to -3e-7, far beyond the -4e-8 of the sound estimate; only at alpha
// near 128 are the fractions uniform enough for a limit this tight.
func TestEstimateHasNoBias(t *testing.T) {
	const alpha, count, limit = 128, 2000, 1.5e-7
	model := NewModel(buildNoisyTarget(128, 128), Color{128, 128, 128, 255}, 128, 1, 7)
	for step := 0; step < 20; step++ {
		model.Step(ShapeTypeTriangle, alpha, 0, 0.5)
	}
	worker := model.Workers[0]
	worker.Init(model.Current, model.Score)
	var bias float64
	for i := 0; i < count; i++ {
		shape := worker.RandomState(ShapeTypeEllipse, alpha, 0.5).Shape
		bias += worker.estimateEnergy(shape.Rasterize(), alpha) - worker.Energy(shape, alpha)
	}
	if bias /= count; math.Abs(bias) > limit {
		t.Errorf("mean estimate - exact: %v", bias)
	}
}
