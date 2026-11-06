package primitive

import (
	"math"
	"math/rand"
	"testing"
)

// The tests below replay the random stream of a shape against a copy of its
// rules: sigma 16, margin 16 for vertices, image bounds for anchored coordinates,
// a spread of 20 around the first vertex. They detect changes and are no
// independent oracle: a wrong rule copied here passes, and an intended change of
// the search updates them together with the golden files. They pin every constant
// and sign of NewRandom* and Mutate, which the property checks of
// TestMutateKeepsCoordinateLimits cannot tell apart from a different but legal
// distribution. Floating point fields are compared with a
// tolerance, because FMA changes the last bit of x+r*16 on GOAMD64=v3.

const referenceSteps = 1500

// scriptedSource gives one fixed Int63, then the stream of rest.
type scriptedSource struct {
	first  int64
	isUsed bool
	rest   rand.Source
}

func (s *scriptedSource) Int63() int64 {
	if s.isUsed {
		return s.rest.Int63()
	}
	s.isUsed = true
	return s.first
}

func (s *scriptedSource) Seed(seed int64) {
	s.rest.Seed(seed)
}

// TestPolygonMutateMovesAtSwapBoundary pins the branch at a draw of exactly 0.25:
// it moves one vertex, and only draws below 0.25 swap two.
func TestPolygonMutateMovesAtSwapBoundary(t *testing.T) {
	worker := newTestWorker(64, 48, 1)
	worker.Rnd = rand.New(&scriptedSource{first: 1 << 61, rest: rand.NewSource(1)})
	xs := []float64{10, 50, 50, 10}
	ys := []float64{10, 10, 40, 40}
	shape := &Polygon{worker, len(xs), true, append([]float64{}, xs...), append([]float64{}, ys...)}
	if !shape.Valid() {
		t.Fatal("the starting square must be valid")
	}
	shape.Mutate()
	moved := 0
	for i := range xs {
		if shape.X[i] != xs[i] || shape.Y[i] != ys[i] {
			moved++
		}
	}
	if moved != 1 {
		t.Errorf("%d vertices changed, want one moved vertex", moved)
	}
}

// reseed gives worker a fresh stream and returns a second stream with the same
// seed for the reference.
func reseed(worker *Worker, seed int64) *rand.Rand {
	worker.Rnd = rand.New(rand.NewSource(seed))
	return rand.New(rand.NewSource(seed))
}

func bound[T int | float64](value, lo, hi T) T {
	return max(lo, min(value, hi))
}

func isNear(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9
}

func checkNear(t *testing.T, label string, got, want []float64) {
	t.Helper()
	for i := range want {
		if !isNear(got[i], want[i]) {
			t.Fatalf("%s: got %v, want %v", label, got, want)
		}
	}
}

func quadraticFields(q *Quadratic) []float64 {
	return []float64{q.X1, q.Y1, q.X2, q.Y2, q.X3, q.Y3, q.Width}
}

func rotatedEllipseFields(e *RotatedEllipse) []float64 {
	return []float64{e.X, e.Y, e.Rx, e.Ry, e.Angle}
}

func polygonFields(p *Polygon) []float64 {
	return append(append([]float64{}, p.X...), p.Y...)
}

func referenceMutateTriangle(want *Triangle, rb *rand.Rand, w, h int) {
	const m = 16
	for {
		switch rb.Intn(3) {
		case 0:
			want.X1 = bound(want.X1+int(rb.NormFloat64()*16), -m, w-1+m)
			want.Y1 = bound(want.Y1+int(rb.NormFloat64()*16), -m, h-1+m)
		case 1:
			want.X2 = bound(want.X2+int(rb.NormFloat64()*16), -m, w-1+m)
			want.Y2 = bound(want.Y2+int(rb.NormFloat64()*16), -m, h-1+m)
		case 2:
			want.X3 = bound(want.X3+int(rb.NormFloat64()*16), -m, w-1+m)
			want.Y3 = bound(want.Y3+int(rb.NormFloat64()*16), -m, h-1+m)
		}
		if want.Valid() {
			return
		}
	}
}

func referenceMutateQuadratic(want *Quadratic, rb *rand.Rand, w, h int) {
	const m = 16.0
	for {
		switch rb.Intn(4) {
		case 0:
			want.X1 = bound(want.X1+rb.NormFloat64()*16, -m, float64(w-1)+m)
			want.Y1 = bound(want.Y1+rb.NormFloat64()*16, -m, float64(h-1)+m)
		case 1:
			want.X2 = bound(want.X2+rb.NormFloat64()*16, -m, float64(w-1)+m)
			want.Y2 = bound(want.Y2+rb.NormFloat64()*16, -m, float64(h-1)+m)
		case 2:
			want.X3 = bound(want.X3+rb.NormFloat64()*16, -m, float64(w-1)+m)
			want.Y3 = bound(want.Y3+rb.NormFloat64()*16, -m, float64(h-1)+m)
		case 3:
			want.Width = bound(want.Width+rb.NormFloat64(), 0.5, want.MaxQuadWidth)
		}
		if want.Valid() {
			return
		}
	}
}

func referenceMutatePolygon(want *Polygon, rb *rand.Rand, w, h int) {
	const m = 16.0
	for {
		if rb.Float64() < 0.25 {
			i, j := rb.Intn(want.Order), rb.Intn(want.Order)
			want.X[i], want.X[j] = want.X[j], want.X[i]
			want.Y[i], want.Y[j] = want.Y[j], want.Y[i]
		} else {
			i := rb.Intn(want.Order)
			want.X[i] = bound(want.X[i]+rb.NormFloat64()*16, -m, float64(w-1)+m)
			want.Y[i] = bound(want.Y[i]+rb.NormFloat64()*16, -m, float64(h-1)+m)
		}
		if want.Valid() {
			return
		}
	}
}

func referenceMutateRotatedRectangle(want *RotatedRectangle, rb *rand.Rand, w, h int) {
	switch rb.Intn(3) {
	case 0:
		want.X = bound(want.X+int(rb.NormFloat64()*16), 0, w-1)
		want.Y = bound(want.Y+int(rb.NormFloat64()*16), 0, h-1)
	case 1:
		want.Sx = bound(want.Sx+int(rb.NormFloat64()*16), 1, w-1)
		want.Sy = bound(want.Sy+int(rb.NormFloat64()*16), 1, h-1)
	case 2:
		want.Angle += int(rb.NormFloat64() * 32)
	}
}

// TestRectangleMutateMatchesReference pins which corner moves, by how much, and
// the clamp to the image.
func TestRectangleMutateMatchesReference(t *testing.T) {
	const w, h = 40, 20
	worker := newTestWorker(w, h, 3)
	rb := reseed(worker, 3)
	got := &Rectangle{worker, 20, 10, 25, 12}
	want := *got
	for i := 0; i < referenceSteps; i++ {
		got.Mutate()
		switch rb.Intn(2) {
		case 0:
			want.X1 = bound(want.X1+int(rb.NormFloat64()*16), 0, w-1)
			want.Y1 = bound(want.Y1+int(rb.NormFloat64()*16), 0, h-1)
		case 1:
			want.X2 = bound(want.X2+int(rb.NormFloat64()*16), 0, w-1)
			want.Y2 = bound(want.Y2+int(rb.NormFloat64()*16), 0, h-1)
		}
		if *got != want {
			t.Fatalf("step %d: got %+v, want %+v", i, *got, want)
		}
	}
}

// TestNewRandomRectangleMatchesReference pins the size limit of 32 and the clamp
// of the second corner.
func TestNewRandomRectangleMatchesReference(t *testing.T) {
	const w, h = 50, 40
	worker := newTestWorker(w, h, 1)
	for seed := int64(1); seed <= 200; seed++ {
		rb := reseed(worker, seed)
		x1, y1 := rb.Intn(w), rb.Intn(h)
		want := Rectangle{worker, x1, y1, bound(x1+rb.Intn(32)+1, 0, w-1), bound(y1+rb.Intn(32)+1, 0, h-1)}
		if got := NewRandomRectangle(worker); *got != want {
			t.Fatalf("seed %d: got %+v, want %+v", seed, *got, want)
		}
	}
}

// TestRotatedRectangleMutateMatchesReference pins the three moves of a rotated
// rectangle: center, sides and angle. No move retries through Valid, so the aspect
// ratio stays unlimited.
func TestRotatedRectangleMutateMatchesReference(t *testing.T) {
	const w, h = 40, 20
	worker := newTestWorker(w, h, 3)
	rb := reseed(worker, 4)
	got := &RotatedRectangle{worker, 20, 10, 6, 4, 30}
	want := *got
	for i := 0; i < referenceSteps; i++ {
		got.Mutate()
		referenceMutateRotatedRectangle(&want, rb, w, h)
		if *got != want {
			t.Fatalf("step %d: got %+v, want %+v", i, *got, want)
		}
	}
}

// TestNewRandomRotatedRectangleMatchesReference pins the ranges of the initial
// sides and angle, and that the constructor mutates once.
func TestNewRandomRotatedRectangleMatchesReference(t *testing.T) {
	const w, h = 50, 40
	worker := newTestWorker(w, h, 1)
	for seed := int64(1); seed <= 200; seed++ {
		rb := reseed(worker, seed)
		want := RotatedRectangle{worker, rb.Intn(w), rb.Intn(h), rb.Intn(32) + 1, rb.Intn(32) + 1, rb.Intn(360)}
		referenceMutateRotatedRectangle(&want, rb, w, h)
		if got := NewRandomRotatedRectangle(worker); *got != want {
			t.Fatalf("seed %d: got %+v, want %+v", seed, *got, want)
		}
	}
}

// TestEllipseMutateMatchesReference pins the moves of an ellipse and of a circle,
// whose second radius follows the first.
func TestEllipseMutateMatchesReference(t *testing.T) {
	const w, h = 40, 20
	for _, isCircle := range []bool{false, true} {
		worker := newTestWorker(w, h, 3)
		rb := reseed(worker, 5)
		got := &Ellipse{Worker: worker, X: 20, Y: 10, Rx: 6, Ry: 6, Circle: isCircle}
		want := *got
		for i := 0; i < referenceSteps; i++ {
			got.Mutate()
			switch rb.Intn(3) {
			case 0:
				want.X = bound(want.X+int(rb.NormFloat64()*16), 0, w-1)
				want.Y = bound(want.Y+int(rb.NormFloat64()*16), 0, h-1)
			case 1:
				want.Rx = bound(want.Rx+int(rb.NormFloat64()*16), 1, w-1)
				if isCircle {
					want.Ry = want.Rx
				}
			case 2:
				want.Ry = bound(want.Ry+int(rb.NormFloat64()*16), 1, h-1)
				if isCircle {
					want.Rx = want.Ry
				}
			}
			if *got != want {
				t.Fatalf("circle %v, step %d: got %+v, want %+v", isCircle, i, *got, want)
			}
		}
	}
}

// TestNewRandomEllipseMatchesReference pins the radius limit of 32 of an ellipse
// and of a circle.
func TestNewRandomEllipseMatchesReference(t *testing.T) {
	const w, h = 50, 40
	worker := newTestWorker(w, h, 1)
	for seed := int64(1); seed <= 100; seed++ {
		rb := reseed(worker, seed)
		wantEllipse := Ellipse{worker, rb.Intn(w), rb.Intn(h), rb.Intn(32) + 1, rb.Intn(32) + 1, false}
		if got := NewRandomEllipse(worker); *got != wantEllipse {
			t.Fatalf("seed %d: got %+v, want %+v", seed, *got, wantEllipse)
		}
		rb = reseed(worker, seed)
		x, y, r := rb.Intn(w), rb.Intn(h), rb.Intn(32)+1
		wantCircle := Ellipse{worker, x, y, r, r, true}
		if got := NewRandomCircle(worker); *got != wantCircle {
			t.Fatalf("seed %d: got %+v, want %+v", seed, *got, wantCircle)
		}
	}
}

// TestRotatedEllipseMutateMatchesReference pins the moves of a rotated ellipse.
func TestRotatedEllipseMutateMatchesReference(t *testing.T) {
	const w, h = 40, 20
	worker := newTestWorker(w, h, 3)
	rb := reseed(worker, 6)
	got := &RotatedEllipse{worker, 20, 10, 6, 4, 30}
	want := *got
	for i := 0; i < referenceSteps; i++ {
		got.Mutate()
		switch rb.Intn(3) {
		case 0:
			want.X = bound(want.X+rb.NormFloat64()*16, 0, float64(w-1))
			want.Y = bound(want.Y+rb.NormFloat64()*16, 0, float64(h-1))
		case 1:
			want.Rx = bound(want.Rx+rb.NormFloat64()*16, 1, float64(w-1))
			want.Ry = bound(want.Ry+rb.NormFloat64()*16, 1, float64(h-1))
		case 2:
			want.Angle += rb.NormFloat64() * 32
		}
		checkNear(t, "rotated ellipse", rotatedEllipseFields(got), rotatedEllipseFields(&want))
	}
}

// TestNewRandomRotatedEllipseMatchesReference pins the center, the radius range
// of 1 to 33 and the angle range of a new rotated ellipse.
func TestNewRandomRotatedEllipseMatchesReference(t *testing.T) {
	const w, h = 50, 40
	worker := newTestWorker(w, h, 1)
	for seed := int64(1); seed <= 100; seed++ {
		rb := reseed(worker, seed)
		x, y := rb.Float64()*w, rb.Float64()*h
		want := RotatedEllipse{worker, x, y, rb.Float64()*32 + 1, rb.Float64()*32 + 1, rb.Float64() * 360}
		checkNear(t, "rotated ellipse", rotatedEllipseFields(NewRandomRotatedEllipse(worker)), rotatedEllipseFields(&want))
	}
}

// TestTriangleMutateMatchesReference pins which vertex moves and the margin of 16
// around the image.
func TestTriangleMutateMatchesReference(t *testing.T) {
	const w, h = 40, 20
	worker := newTestWorker(w, h, 3)
	rb := reseed(worker, 7)
	got := &Triangle{worker, 10, 5, 30, 5, 20, 15}
	want := *got
	for i := 0; i < referenceSteps; i++ {
		got.Mutate()
		referenceMutateTriangle(&want, rb, w, h)
		if *got != want {
			t.Fatalf("step %d: got %+v, want %+v", i, *got, want)
		}
	}
}

// TestNewRandomTriangleMatchesReference pins the spread of 15 pixels of the
// vertices around the first one.
func TestNewRandomTriangleMatchesReference(t *testing.T) {
	const w, h = 50, 40
	worker := newTestWorker(w, h, 1)
	for seed := int64(1); seed <= 200; seed++ {
		rb := reseed(worker, seed)
		x1, y1 := rb.Intn(w), rb.Intn(h)
		want := Triangle{worker, x1, y1, x1 + rb.Intn(31) - 15, y1 + rb.Intn(31) - 15, x1 + rb.Intn(31) - 15, y1 + rb.Intn(31) - 15}
		referenceMutateTriangle(&want, rb, w, h)
		if got := NewRandomTriangle(worker); *got != want {
			t.Fatalf("seed %d: got %+v, want %+v", seed, *got, want)
		}
	}
}

// TestQuadraticMutateMatchesReference pins the moves of a curve: three control
// points with a margin of 16 and the stroke width.
func TestQuadraticMutateMatchesReference(t *testing.T) {
	const w, h = 40, 20
	worker := newTestWorker(w, h, 3)
	rb := reseed(worker, 8)
	got := &Quadratic{worker, 5, 5, 15, 10, 30, 12, 0.5, 3}
	want := *got
	for i := 0; i < referenceSteps; i++ {
		got.Mutate()
		referenceMutateQuadratic(&want, rb, w, h)
		checkNear(t, "quadratic", quadraticFields(got), quadraticFields(&want))
	}
}

// TestNewRandomQuadraticMatchesReference pins the start width of 0.5 and the
// spread of 20 pixels between control points.
func TestNewRandomQuadraticMatchesReference(t *testing.T) {
	const w, h = 50, 40
	worker := newTestWorker(w, h, 1)
	for seed := int64(1); seed <= 200; seed++ {
		rb := reseed(worker, seed)
		x1, y1 := rb.Float64()*w, rb.Float64()*h
		x2, y2 := x1+(rb.Float64()*40-20), y1+(rb.Float64()*40-20)
		x3, y3 := x2+(rb.Float64()*40-20), y2+(rb.Float64()*40-20)
		want := Quadratic{worker, x1, y1, x2, y2, x3, y3, 0.5, 4}
		referenceMutateQuadratic(&want, rb, w, h)
		checkNear(t, "quadratic", quadraticFields(NewRandomQuadratic(worker, 4)), quadraticFields(&want))
	}
}

// TestPolygonMutateMatchesReference pins the vertex move, the swap of two
// vertices at a chance of 0.25 and the retry until a convex polygon stays convex.
func TestPolygonMutateMatchesReference(t *testing.T) {
	const w, h = 40, 20
	for _, isConvex := range []bool{false, true} {
		worker := newTestWorker(w, h, 3)
		rb := reseed(worker, 9)
		got := &Polygon{worker, 4, isConvex, []float64{10, 30, 30, 10}, []float64{3, 3, 15, 15}}
		want := got.Copy().(*Polygon)
		for i := 0; i < referenceSteps; i++ {
			got.Mutate()
			referenceMutatePolygon(want, rb, w, h)
			checkNear(t, "polygon", polygonFields(got), polygonFields(want))
		}
	}
}

// TestNewRandomPolygonMatchesReference pins the spread of 20 pixels of the
// vertices around the first one.
func TestNewRandomPolygonMatchesReference(t *testing.T) {
	const w, h, order = 50, 40, 5
	worker := newTestWorker(w, h, 1)
	for seed := int64(1); seed <= 200; seed++ {
		for _, isConvex := range []bool{false, true} {
			rb := reseed(worker, seed)
			want := &Polygon{worker, order, isConvex, make([]float64, order), make([]float64, order)}
			want.X[0], want.Y[0] = rb.Float64()*w, rb.Float64()*h
			for i := 1; i < order; i++ {
				want.X[i] = want.X[0] + (rb.Float64()*40 - 20)
				want.Y[i] = want.Y[0] + (rb.Float64()*40 - 20)
			}
			referenceMutatePolygon(want, rb, w, h)
			checkNear(t, "polygon", polygonFields(NewRandomPolygon(worker, order, isConvex)), polygonFields(want))
		}
	}
}
