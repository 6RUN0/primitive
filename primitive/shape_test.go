package primitive

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

var allShapeTypes = []ShapeType{
	ShapeTypeTriangle, ShapeTypeRectangle, ShapeTypeEllipse, ShapeTypeCircle,
	ShapeTypeRotatedRectangle, ShapeTypeQuadratic, ShapeTypeRotatedEllipse, ShapeTypePolygon,
}

// TestRasterizeStaysInsideImage pins the contract of the pixel kernels, which
// index Pix without bounds checks: every scanline lies inside the worker image,
// for shapes mutated at the margins and on images of one pixel.
func TestRasterizeStaysInsideImage(t *testing.T) {
	sizes := []struct{ width, height int }{{64, 16}, {16, 64}, {1, 1}, {1, 9}, {9, 1}}
	for _, size := range sizes {
		worker := newTestWorker(size.width, size.height, 5)
		for _, shapeType := range allShapeTypes {
			state := worker.RandomState(shapeType, 128, 4)
			for i := 0; i < 300; i++ {
				state.Shape.Mutate()
				for _, line := range state.Shape.Rasterize() {
					if line.Y < 0 || line.Y >= size.height || line.X1 < 0 || line.X2 >= size.width || line.X1 > line.X2 {
						t.Fatalf("%dx%d mode %d: line %+v outside the image", size.width, size.height, shapeType, line)
					}
				}
			}
		}
	}
}

// TestCombinedModePicksConcreteShape pins ShapeTypeAny: the default branch of
// RandomState draws a real shape type instead of recursing forever on zero.
func TestCombinedModePicksConcreteShape(t *testing.T) {
	worker := newTestWorker(16, 16, 2)
	for i := 0; i < 200; i++ {
		if state := worker.RandomState(ShapeTypeAny, 128, 0.5); state.Shape == nil {
			t.Fatal("got no shape")
		}
	}
}

// TestMutateKeepsCoordinateLimits pins the mutation limits of each shape: the
// margin of 16 pixels for vertices, the image size for anchored coordinates.
func TestMutateKeepsCoordinateLimits(t *testing.T) {
	const width, height, margin = 40, 20, 16
	worker := newTestWorker(width, height, 9)
	isInside := func(value, size int) bool { return value >= 0 && value < size }
	isVertexX := func(value float64) bool { return value >= -margin && value <= width-1+margin }
	isVertexY := func(value float64) bool { return value >= -margin && value <= height-1+margin }

	triangle := &Triangle{worker, 5, 5, 15, 5, 5, 15}
	rectangle := &Rectangle{worker, 1, 1, 2, 2}
	ellipse := &Ellipse{Worker: worker, X: 5, Y: 5, Rx: 2, Ry: 2}
	rotatedRectangle := &RotatedRectangle{worker, 5, 5, 2, 2, 0}
	polygon := &Polygon{worker, 4, false, []float64{1, 9, 9, 1}, []float64{1, 1, 9, 9}}
	quadratic := &Quadratic{worker, 1, 1, 6, 6, 12, 12, 0.5, 3}
	cases := []struct {
		name          string
		shape         Shape
		isWithinLimit func() bool
	}{
		{"triangle", triangle, func() bool {
			return triangle.Valid() &&
				isVertexX(float64(triangle.X1)) && isVertexY(float64(triangle.Y1)) &&
				isVertexX(float64(triangle.X2)) && isVertexY(float64(triangle.Y2)) &&
				isVertexX(float64(triangle.X3)) && isVertexY(float64(triangle.Y3))
		}},
		{"rectangle", rectangle, func() bool {
			return isInside(rectangle.X1, width) && isInside(rectangle.Y1, height) &&
				isInside(rectangle.X2, width) && isInside(rectangle.Y2, height)
		}},
		{"ellipse", ellipse, func() bool {
			return isInside(ellipse.X, width) && isInside(ellipse.Y, height) &&
				ellipse.Rx >= 1 && ellipse.Rx < width && ellipse.Ry >= 1 && ellipse.Ry < height
		}},
		{"rotated rectangle", rotatedRectangle, func() bool {
			return isInside(rotatedRectangle.X, width) && isInside(rotatedRectangle.Y, height) &&
				rotatedRectangle.Sx >= 1 && rotatedRectangle.Sx < width &&
				rotatedRectangle.Sy >= 1 && rotatedRectangle.Sy < height
		}},
		{"polygon", polygon, func() bool {
			for i := range polygon.X {
				if !isVertexX(polygon.X[i]) || !isVertexY(polygon.Y[i]) {
					return false
				}
			}
			return true
		}},
		{"quadratic", quadratic, func() bool {
			return quadratic.Valid() && quadratic.Width >= 0.5 && quadratic.Width <= 3 &&
				isVertexX(quadratic.X1) && isVertexY(quadratic.Y1) &&
				isVertexX(quadratic.X2) && isVertexY(quadratic.Y2) &&
				isVertexX(quadratic.X3) && isVertexY(quadratic.Y3)
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			for i := 0; i < 3000; i++ {
				testCase.shape.Mutate()
				if !testCase.isWithinLimit() {
					t.Fatalf("mutation %d: %s", i, testCase.shape.SVG(""))
				}
			}
		})
	}
}

// TestCircleKeepsEqualRadii pins the circle variant of Ellipse: each mutation of
// one radius copies it to the other.
func TestCircleKeepsEqualRadii(t *testing.T) {
	circle := NewRandomCircle(newTestWorker(40, 40, 4))
	for i := 0; i < 500; i++ {
		circle.Mutate()
		if circle.Rx != circle.Ry {
			t.Fatalf("mutation %d: radii %d and %d", i, circle.Rx, circle.Ry)
		}
	}
}

// TestShapeValidRules pins the rejection rules that Mutate retries on.
func TestShapeValidRules(t *testing.T) {
	cases := []struct {
		name  string
		shape interface{ Valid() bool }
		want  bool
	}{
		{"wide triangle", &Triangle{X1: 0, Y1: 0, X2: 10, Y2: 0, X3: 5, Y3: 10}, true},
		{"needle triangle", &Triangle{X1: 0, Y1: 0, X2: 100, Y2: 0, X3: 100, Y3: 5}, false},
		{"collinear triangle", &Triangle{X1: 0, Y1: 0, X2: 5, Y2: 5, X3: 10, Y3: 10}, false},
		{"coincident vertices", &Triangle{X1: 3, Y1: 3, X2: 3, Y2: 3, X3: 10, Y3: 0}, false},
		{"aspect 5", &RotatedRectangle{Sx: 25, Sy: 5}, true},
		{"aspect 6", &RotatedRectangle{Sx: 5, Sy: 30}, false},
		{"square polygon", &Polygon{Order: 4, Convex: true, X: []float64{0, 9, 9, 0}, Y: []float64{0, 0, 9, 9}}, true},
		{"crossed polygon", &Polygon{Order: 4, Convex: true, X: []float64{0, 9, 0, 9}, Y: []float64{0, 9, 9, 0}}, false},
		{"crossed polygon, convexity off", &Polygon{Order: 4, X: []float64{0, 9, 0, 9}, Y: []float64{0, 9, 9, 0}}, true},
		{"open curve", &Quadratic{X1: 0, Y1: 0, X2: 5, Y2: 2, X3: 10, Y3: 0}, true},
		{"curve folded back", &Quadratic{X1: 0, Y1: 0, X2: 20, Y2: 0, X3: 5, Y3: 0}, false},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := testCase.shape.Valid(); got != testCase.want {
				t.Errorf("got %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestShapeCopyIsIndependent pins Copy, which hill climbing relies on to undo a
// move: Polygon holds slices, and a shallow copy lets a mutation leak back.
func TestShapeCopyIsIndependent(t *testing.T) {
	worker := newTestWorker(32, 32, 6)
	for _, shapeType := range allShapeTypes {
		original := worker.RandomState(shapeType, 128, 4).Shape
		wantSVG := original.SVG("")
		clone := original.Copy()
		for i := 0; i < 20; i++ {
			clone.Mutate()
		}
		if got := original.SVG(""); got != wantSVG {
			t.Errorf("mode %d: mutating a copy changed the original:\n%s\nwant:\n%s", shapeType, got, wantSVG)
		}
	}
}

// TestRectangleRasterizeAcceptsSwappedCorners pins that a mutated rectangle with
// crossed corners covers the same pixels and reports the same SVG box.
func TestRectangleRasterizeAcceptsSwappedCorners(t *testing.T) {
	worker := newTestWorker(16, 16, 1)
	rectangle := &Rectangle{worker, 5, 4, 2, 3}
	lines := rectangle.Rasterize()
	if len(lines) != 2 || lines[0] != (Scanline{3, 2, 5, 0xffff}) || lines[1] != (Scanline{4, 2, 5, 0xffff}) {
		t.Errorf("got lines %+v", lines)
	}
	want := `<rect a="b" x="2" y="3" width="4" height="2"/>`
	if got := rectangle.SVG(`a="b"`); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestUndoMoveRestoresState pins the rollback of a rejected hill climbing move.
func TestUndoMoveRestoresState(t *testing.T) {
	worker := newTestWorker(32, 32, 8)
	state := worker.RandomState(ShapeTypeTriangle, 0, 0.5)
	wantSVG, wantEnergy, wantAlpha := state.Shape.SVG(""), state.Energy(), state.Alpha
	undo := state.DoMove()
	state.UndoMove(undo)
	if state.Shape.SVG("") != wantSVG || state.Energy() != wantEnergy || state.Alpha != wantAlpha {
		t.Errorf("state after undo differs: %s, energy %v, alpha %d", state.Shape.SVG(""), state.Energy(), state.Alpha)
	}
}

// TestHillClimbNeverWorsensEnergy pins the acceptance rule of the search.
func TestHillClimbNeverWorsensEnergy(t *testing.T) {
	worker := newTestWorker(32, 32, 10)
	for _, shapeType := range allShapeTypes {
		start := worker.RandomState(shapeType, 128, 0.5)
		before := start.Energy()
		if after := HillClimb(start, 20).Energy(); after > before {
			t.Errorf("mode %d: energy rose from %v to %v", shapeType, before, after)
		}
	}
}

// TestStepKeepsModelConsistent pins the bookkeeping of Step with -rep and -a 0:
// parallel slices stay equal in length, alpha stays a valid channel value, and
// the running score stays equal to a full comparison of the pictures.
func TestStepKeepsModelConsistent(t *testing.T) {
	model := NewModel(buildNoisyTarget(32, 32), Color{128, 128, 128, 255}, 64, 2, 21)
	for step := 0; step < 3; step++ {
		model.Step(ShapeTypeQuadratic, 0, 2, 4)
	}
	if len(model.Shapes) < 3 || len(model.Shapes) != len(model.Colors) || len(model.Shapes) != len(model.Scores) {
		t.Fatalf("got %d shapes, %d colors, %d scores", len(model.Shapes), len(model.Colors), len(model.Scores))
	}
	for i, c := range model.Colors {
		if c.A < 1 || c.A > 255 {
			t.Errorf("shape %d: alpha %d", i, c.A)
		}
	}
	if full := differenceFull(model.Target, model.Current); math.Abs(model.Score-full) > 1e-6 {
		t.Errorf("score %v, full comparison %v", model.Score, full)
	}
	frames := model.Frames(0)
	if len(frames) != len(model.Shapes)+1 {
		t.Errorf("got %d frames for %d shapes", len(frames), len(model.Shapes))
	}
	if got := frames[0].Bounds().Size(); got.X != 64 || got.Y != 64 {
		t.Errorf("frame size %v, want 64x64", got)
	}
}

// TestQuadraticValidMatchesFarthestPair pins Valid against its definition on a
// grid small enough to hold many equal distances: the end points must be the
// farthest pair, and a tie is not valid.
func TestQuadraticValidMatchesFarthestPair(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	squared := func(ax, ay, bx, by float64) float64 { return (ax-bx)*(ax-bx) + (ay-by)*(ay-by) }
	for i := 0; i < 3000; i++ {
		var p [6]float64
		for j := range p {
			p[j] = float64(rnd.Intn(13) - 6)
		}
		d12, d23, d13 := squared(p[0], p[1], p[2], p[3]), squared(p[2], p[3], p[4], p[5]), squared(p[0], p[1], p[4], p[5])
		want := d13 > d12 && d13 > d23
		curve := &Quadratic{X1: p[0], Y1: p[1], X2: p[2], Y2: p[3], X3: p[4], Y3: p[5]}
		if got := curve.Valid(); got != want {
			t.Fatalf("points %v: got %v, want %v", p, got, want)
		}
	}
}

// TestPolygonValidCountsStraightVertexAsNegativeTurn pins the rule for a vertex
// on the line of its neighbours, which clamping to the margin produces: a zero
// cross product is a turn to the negative side, in the first triple and in the
// later ones. A convex shape with such a vertex passes in one direction of the walk
// only.
func TestPolygonValidCountsStraightVertexAsNegativeTurn(t *testing.T) {
	cases := []struct {
		name string
		x, y []float64
		want bool
	}{
		{"straight first, turns positive", []float64{0, 5, 10, 10, 0}, []float64{0, 0, 0, 10, 10}, false},
		{"straight first, turns negative", []float64{0, 5, 10, 10, 0}, []float64{0, 0, 0, -10, -10}, true},
		{"straight last, turns positive", []float64{5, 10, 10, 0, 0}, []float64{0, 0, 10, 10, 0}, false},
		{"straight last, turns negative", []float64{5, 10, 10, 0, 0}, []float64{0, 0, -10, -10, 0}, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			polygon := &Polygon{Order: len(testCase.x), Convex: true, X: testCase.x, Y: testCase.y}
			if got := polygon.Valid(); got != testCase.want {
				t.Errorf("got %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestBestRandomStateKeepsFirstOfEqualEnergies pins the tie rule of the searches:
// when current already equals the target every candidate scores zero, and the
// first one wins, which keeps a run reproducible when scores tie.
func TestBestRandomStateKeepsFirstOfEqualEnergies(t *testing.T) {
	newWorker := func() *Worker {
		target := newUniformRGBA(32, 32, Color{90, 120, 150, 255})
		worker := NewWorker(target, 11)
		worker.Init(copyRGBA(target), 0)
		return worker
	}
	for _, shapeType := range allShapeTypes {
		first := newWorker().RandomState(shapeType, 128, 0.5).Shape.SVG("")
		if got := newWorker().BestRandomState(shapeType, 128, 6, 0.5).Shape.SVG(""); got != first {
			t.Errorf("mode %d: best of six is not the first candidate:\n%s\nwant:\n%s", shapeType, got, first)
		}
		if got := newWorker().BestHillClimbState(shapeType, 128, 1, 2, 3, 0.5).Shape.SVG(""); got != newWorker().BestHillClimbState(shapeType, 128, 1, 2, 1, 0.5).Shape.SVG("") {
			t.Errorf("mode %d: best of three climbs is not the first climb", shapeType)
		}
	}
}

// TestCombinedModeReachesEveryShape pins ShapeTypeAny: all eight shapes are
// drawn, the polygon and the rotated ellipse included.
func TestCombinedModeReachesEveryShape(t *testing.T) {
	worker := newTestWorker(32, 32, 3)
	seen := map[string]bool{}
	for i := 0; i < 400; i++ {
		switch shape := worker.RandomState(ShapeTypeAny, 128, 0.5).Shape.(type) {
		case *Ellipse:
			seen[fmt.Sprintf("ellipse circle=%v", shape.Circle)] = true
		default:
			seen[fmt.Sprintf("%T", shape)] = true
		}
	}
	if len(seen) != 8 {
		t.Errorf("got %d kinds of shape, want 8: %v", len(seen), seen)
	}
}
