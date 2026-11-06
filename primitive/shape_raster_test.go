package primitive

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/fogleman/gg"
)

// The tests below compare Rasterize and Draw with the outline of the shape in
// plain geometry. A pixel deeper than margin inside the outline must be covered
// and a pixel farther than margin outside must be empty; the band between them
// holds the rounding of the integer vertices. The margins are far below the
// size of all but the smallest shapes, where only the outside check applies, so a wrong sign, factor or index in the corner arithmetic
// moves a whole edge out of the band. No pixel may be covered twice: a row
// emitted twice shows as coverage above 1.

const (
	rasterWidth  = 64
	rasterHeight = 48
)

// signedDistance returns the distance in pixels from a point to the border of a
// shape, negative inside. It may underestimate the distance far from the border.
type signedDistance func(x, y float64) float64

// rotatedFrame returns the offset of (x, y) from (cx, cy) in the frame turned by
// angle degrees, the inverse of rotate.
func rotatedFrame(x, y, cx, cy, angle float64) (u, v float64) {
	sin, cos := math.Sincos(angle * math.Pi / 180)
	dx, dy := x-cx, y-cy
	return dx*cos + dy*sin, -dx*sin + dy*cos
}

func rectangleOutline(cx, cy, sx, sy, angle float64) signedDistance {
	return func(x, y float64) float64 {
		u, v := rotatedFrame(x, y, cx, cy, angle)
		return max(math.Abs(u)-sx/2, math.Abs(v)-sy/2)
	}
}

func ellipseOutline(cx, cy, rx, ry, angle float64) signedDistance {
	return func(x, y float64) float64 {
		u, v := rotatedFrame(x, y, cx, cy, angle)
		slope := 2 * math.Hypot(u/(rx*rx), v/(ry*ry))
		if slope == 0 {
			return -min(rx, ry)
		}
		return (u*u/(rx*rx) + v*v/(ry*ry) - 1) / slope
	}
}

// convexOutline is the outline of a convex polygon with vertices in either order.
func convexOutline(xs, ys []float64) signedDistance {
	var area float64
	for i := range xs {
		j := (i + 1) % len(xs)
		area += xs[i]*ys[j] - xs[j]*ys[i]
	}
	orientation := math.Copysign(1, area)
	return func(x, y float64) float64 {
		farthest := math.Inf(-1)
		for i := range xs {
			j := (i + 1) % len(xs)
			ex, ey := xs[j]-xs[i], ys[j]-ys[i]
			side := orientation * (ex*(y-ys[i]) - ey*(x-xs[i])) / math.Hypot(ex, ey)
			farthest = max(farthest, -side)
		}
		return farthest
	}
}

// curveOutline is the outline of a quadratic curve stroked with round caps.
func curveOutline(q *Quadratic) signedDistance {
	const samples = 400
	return func(x, y float64) float64 {
		nearest := math.Inf(1)
		for i := 0; i <= samples; i++ {
			t := float64(i) / samples
			a, b, c := (1-t)*(1-t), 2*(1-t)*t, t*t
			nearest = min(nearest, math.Hypot(x-(a*q.X1+b*q.X2+c*q.X3), y-(a*q.Y1+b*q.Y2+c*q.Y3)))
		}
		return nearest - q.Width/2
	}
}

func scanlineCoverage(t *testing.T, lines []Scanline, w, h int) []float64 {
	t.Helper()
	cover := make([]float64, w*h)
	for _, line := range lines {
		if line.Y < 0 || line.Y >= h || line.X1 < 0 || line.X2 >= w || line.X1 > line.X2 {
			t.Fatalf("line %+v outside the %dx%d image", line, w, h)
		}
		for x := line.X1; x <= line.X2; x++ {
			cover[line.Y*w+x] += float64(line.Alpha) / 0xffff
		}
	}
	return cover
}

func alphaCoverage(im *image.RGBA) []float64 {
	size := im.Bounds().Size()
	cover := make([]float64, size.X*size.Y)
	for y := 0; y < size.Y; y++ {
		for x := 0; x < size.X; x++ {
			cover[y*size.X+x] = float64(im.Pix[im.PixOffset(x, y)+3]) / 255
		}
	}
	return cover
}

// checkOutline compares cover with outline. offset is the position of the sample
// point in a pixel: 0 for integer vertices read as pixel indices, 0.5 for the
// anti-aliased rasterizer and gg, which sample at pixel centers.
func checkOutline(t *testing.T, cover []float64, w, h int, offset, margin float64, outline signedDistance) {
	t.Helper()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cover[y*w+x]
			d := outline(float64(x)+offset, float64(y)+offset)
			switch {
			case c > 1+1e-6:
				t.Fatalf("pixel (%d, %d) is covered %v times", x, y, c)
			case d <= -margin && c < 0.99:
				t.Fatalf("pixel (%d, %d) is %.2f px inside the outline and has coverage %v", x, y, -d, c)
			case d >= margin && c > 0.01:
				t.Fatalf("pixel (%d, %d) is %.2f px outside the outline and has coverage %v", x, y, d, c)
			}
		}
	}
}

func checkRasterize(t *testing.T, shape Shape, offset, margin float64, outline signedDistance) {
	t.Helper()
	cover := scanlineCoverage(t, shape.Rasterize(), rasterWidth, rasterHeight)
	checkOutline(t, cover, rasterWidth, rasterHeight, offset, margin, outline)
}

// TestRotatedRectangleRasterizeFollowsOutline pins the corners of a rotated
// rectangle, the scan of its edges and the clip to the image, for angles in
// every quadrant, beyond 360 degrees and below zero, and for centers on each
// corner of the image. It also pins the left clip: a row more than a pixel left
// of the image gets no pixel at x = 0.
func TestRotatedRectangleRasterizeFollowsOutline(t *testing.T) {
	worker := newTestWorker(rasterWidth, rasterHeight, 1)
	centers := [][2]int{{30, 24}, {0, 0}, {63, 0}, {0, 47}, {63, 47}}
	angles := []int{0, 15, 30, 45, 90, 135, 200, 300, -30, 390}
	sides := [][2]int{{20, 10}, {9, 30}, {3, 3}}
	for _, center := range centers {
		for _, side := range sides {
			for _, angle := range angles {
				shape := &RotatedRectangle{worker, center[0], center[1], side[0], side[1], angle}
				t.Run(fmt.Sprintf("%v/%v/%d", center, side, angle), func(t *testing.T) {
					outline := rectangleOutline(float64(center[0]), float64(center[1]), float64(side[0]), float64(side[1]), float64(angle))
					checkRasterize(t, shape, 0, 2, outline)
				})
			}
		}
	}
}

// TestEllipseRasterizeFollowsOutline pins the width of each row of an ellipse
// and a circle, the clip at every side of the image, for centers on the border
// and just outside it, and that the middle row is emitted once.
func TestEllipseRasterizeFollowsOutline(t *testing.T) {
	worker := newTestWorker(rasterWidth, rasterHeight, 1)
	centers := [][2]int{{30, 24}, {0, 0}, {63, 0}, {0, 47}, {63, 47}, {30, 0}, {30, 47}, {0, 24}, {63, 24}, {30, -3}, {30, 50}}
	radii := [][2]int{{12, 7}, {7, 12}, {10, 10}, {1, 1}, {2, 1}, {1, 5}}
	for _, center := range centers {
		for _, radius := range radii {
			shape := &Ellipse{Worker: worker, X: center[0], Y: center[1], Rx: radius[0], Ry: radius[1]}
			t.Run(fmt.Sprintf("%v/%v", center, radius), func(t *testing.T) {
				outline := ellipseOutline(float64(center[0]), float64(center[1]), float64(radius[0]), float64(radius[1]), 0)
				checkRasterize(t, shape, 0, 1, outline)
			})
		}
	}
}

// TestRotatedEllipseRasterizeFollowsOutline pins the sixteen arcs of a rotated
// ellipse: radii, angle and center.
func TestRotatedEllipseRasterizeFollowsOutline(t *testing.T) {
	worker := newTestWorker(rasterWidth, rasterHeight, 1)
	for _, angle := range []float64{0, 30, 90, 135, 250, -40, 400} {
		for _, radius := range [][2]float64{{14, 6}, {6, 14}, {9, 9}} {
			shape := &RotatedEllipse{worker, 30, 24, radius[0], radius[1], angle}
			t.Run(fmt.Sprintf("%v/%v", radius, angle), func(t *testing.T) {
				checkRasterize(t, shape, 0.5, 1, ellipseOutline(30, 24, radius[0], radius[1], angle))
			})
		}
	}
}

// coverageMoments returns the covered area and the center and second moments of
// the covered pixels, measured at pixel centers.
func coverageMoments(cover []float64, w int) (area, mx, my, sxx, syy, sxy float64) {
	for i, c := range cover {
		x, y := float64(i%w)+0.5, float64(i/w)+0.5
		area += c
		mx += c * x
		my += c * y
	}
	mx /= area
	my /= area
	for i, c := range cover {
		x, y := float64(i%w)+0.5-mx, float64(i/w)+0.5-my
		sxx += c * x * x
		syy += c * y * y
		sxy += c * x * y
	}
	return area, mx, my, sxx / area, syy / area, sxy / area
}

// TestRotatedEllipseRasterizeKeepsAreaAndTurn pins the area, the center and the
// direction of the long axis of a rotated ellipse with moments, which an arc
// that is off by a few percent changes too.
func TestRotatedEllipseRasterizeKeepsAreaAndTurn(t *testing.T) {
	const side = 120
	worker := newTestWorker(side, side, 1)
	shape := &RotatedEllipse{worker, 50, 60, 30, 12, 35}
	cover := scanlineCoverage(t, shape.Rasterize(), side, side)
	area, mx, my, sxx, syy, sxy := coverageMoments(cover, side)
	sin, cos := math.Sincos(35 * math.Pi / 180)
	wantXX := (30*30*cos*cos + 12*12*sin*sin) / 4
	wantYY := (30*30*sin*sin + 12*12*cos*cos) / 4
	wantXY := (30*30 - 12*12) * sin * cos / 4
	if want := math.Pi * 30 * 12; math.Abs(area-want) > 0.02*want {
		t.Errorf("area %v, want %v", area, want)
	}
	if math.Abs(mx-50) > 0.2 || math.Abs(my-60) > 0.2 {
		t.Errorf("center (%v, %v), want (50, 60)", mx, my)
	}
	for _, moment := range []struct {
		name      string
		got, want float64
	}{{"xx", sxx, wantXX}, {"yy", syy, wantYY}, {"xy", sxy, wantXY}} {
		if math.Abs(moment.got-moment.want) > 0.03*wantXX {
			t.Errorf("moment %s %v, want %v", moment.name, moment.got, moment.want)
		}
	}
}

// TestPolygonRasterizeFollowsOutline pins the closing of the path: a convex
// polygon is filled between all its vertices, whichever the first one is.
func TestPolygonRasterizeFollowsOutline(t *testing.T) {
	worker := newTestWorker(rasterWidth, rasterHeight, 1)
	xs := []float64{10, 40, 55, 30, 12}
	ys := []float64{8, 4, 25, 42, 30}
	for start := range xs {
		rotatedX := append(append([]float64{}, xs[start:]...), xs[:start]...)
		rotatedY := append(append([]float64{}, ys[start:]...), ys[:start]...)
		shape := &Polygon{worker, len(xs), true, rotatedX, rotatedY}
		t.Run(fmt.Sprintf("start %d", start), func(t *testing.T) {
			checkRasterize(t, shape, 0.5, 1, convexOutline(xs, ys))
			var area float64
			for i := range xs {
				j := (i + 1) % len(xs)
				area += xs[i]*ys[j] - xs[j]*ys[i]
			}
			var covered float64
			for _, c := range scanlineCoverage(t, shape.Rasterize(), rasterWidth, rasterHeight) {
				covered += c
			}
			if want := math.Abs(area) / 2; math.Abs(covered-want) > 0.01*want {
				t.Errorf("covered %v, want %v", covered, want)
			}
		})
	}
}

// TestQuadraticRasterizeFollowsOutline pins the stroke of a curve: its width
// and the round caps at both ends.
func TestQuadraticRasterizeFollowsOutline(t *testing.T) {
	worker := newTestWorker(rasterWidth, rasterHeight, 1)
	for _, shape := range []*Quadratic{
		{worker, 8, 10, 30, 40, 56, 12, 6, 8},
		{worker, 8, 24, 32, 24, 56, 24, 3, 8},
		{worker, 56, 40, 30, 5, 10, 38, 9, 12},
	} {
		t.Run(fmt.Sprintf("%v", *shape), func(t *testing.T) {
			checkRasterize(t, shape, 0.5, 1, curveOutline(shape))
		})
	}
}

// TestTriangleRasterizeFollowsOutline pins the scan of flat top, flat bottom and
// general triangles, and the clip to the image.
func TestTriangleRasterizeFollowsOutline(t *testing.T) {
	worker := newTestWorker(rasterWidth, rasterHeight, 1)
	for _, v := range [][6]int{
		{5, 5, 40, 8, 20, 40},
		{10, 10, 50, 10, 30, 40},
		{30, 5, 10, 40, 50, 40},
		{-10, -8, 70, 20, 20, 60},
		{50, 3, 5, 44, 38, 30},
	} {
		shape := &Triangle{worker, v[0], v[1], v[2], v[3], v[4], v[5]}
		t.Run(fmt.Sprintf("%v", v), func(t *testing.T) {
			xs := []float64{float64(v[0]), float64(v[2]), float64(v[4])}
			ys := []float64{float64(v[1]), float64(v[3]), float64(v[5])}
			checkRasterize(t, shape, 0, 2, convexOutline(xs, ys))
		})
	}
}

// TestTriangleRasterizeIgnoresVertexOrder pins that the three sorts of the
// vertices by row give one picture whichever vertex comes first, ties included.
func TestTriangleRasterizeIgnoresVertexOrder(t *testing.T) {
	worker := newTestWorker(rasterWidth, rasterHeight, 1)
	orders := [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}}
	for _, v := range [][6]int{
		{5, 5, 40, 8, 20, 40},
		{10, 10, 50, 10, 30, 40},
		{30, 5, 10, 40, 50, 40},
		{10, 5, 30, 25, 50, 25},
		{10, 25, 30, 5, 50, 25},
		{10, 25, 30, 25, 50, 5},
	} {
		var want []float64
		for _, order := range orders {
			shape := &Triangle{worker, v[2*order[0]], v[2*order[0]+1], v[2*order[1]], v[2*order[1]+1], v[2*order[2]], v[2*order[2]+1]}
			got := scanlineCoverage(t, shape.Rasterize(), rasterWidth, rasterHeight)
			if want == nil {
				want = got
				continue
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("triangle %v, order %v: pixel (%d, %d) differs", v, order, i%rasterWidth, i/rasterWidth)
				}
			}
		}
	}
}

func drawOnCanvas(shape Shape, scale float64, w, h int) []float64 {
	dc := gg.NewContext(w, h)
	dc.Scale(scale, scale)
	dc.SetRGBA(1, 1, 1, 1)
	shape.Draw(dc, scale)
	return alphaCoverage(dc.Image().(*image.RGBA))
}

// TestRectangleDrawCoversItsPixels pins the box that Draw paints: x1..x2 and
// y1..y2 inclusive, as Rasterize and SVG report it.
func TestRectangleDrawCoversItsPixels(t *testing.T) {
	worker := newTestWorker(rasterWidth, rasterHeight, 1)
	for _, shape := range []*Rectangle{{worker, 5, 4, 20, 12}, {worker, 20, 12, 5, 4}} {
		outline := func(x, y float64) float64 {
			return max(max(5-x, x-21), max(4-y, y-13))
		}
		checkOutline(t, drawOnCanvas(shape, 1, rasterWidth, rasterHeight), rasterWidth, rasterHeight, 0.5, 0.5, outline)
	}
}

// TestRotatedRectangleDrawFollowsOutline pins the corners that Draw paints.
func TestRotatedRectangleDrawFollowsOutline(t *testing.T) {
	worker := newTestWorker(rasterWidth, rasterHeight, 1)
	for _, angle := range []int{0, 30, 90, 135, -40} {
		shape := &RotatedRectangle{worker, 30, 24, 20, 10, angle}
		t.Run(fmt.Sprint(angle), func(t *testing.T) {
			cover := drawOnCanvas(shape, 1, rasterWidth, rasterHeight)
			checkOutline(t, cover, rasterWidth, rasterHeight, 0.5, 1, rectangleOutline(30, 24, 20, 10, float64(angle)))
		})
	}
}

// TestQuadraticDrawScalesTheStroke pins that the line width follows the output
// scale, because gg strokes in device pixels: the stroke is 2 x 3 pixels wide on
// a canvas scaled by 2.
func TestQuadraticDrawScalesTheStroke(t *testing.T) {
	worker := newTestWorker(rasterWidth, rasterHeight, 1)
	shape := &Quadratic{worker, 5, 10, 15, 10, 25, 10, 3, 8}
	outline := func(x, y float64) float64 {
		segment := math.Hypot(x-clamp(x, 10, 50), y-20)
		return segment - 3
	}
	checkOutline(t, drawOnCanvas(shape, 2, rasterWidth, rasterHeight), rasterWidth, rasterHeight, 0.5, 1, outline)
}

// TestPolygonDrawFollowsOutline pins that Draw paints every vertex of the
// polygon, the last one included.
func TestPolygonDrawFollowsOutline(t *testing.T) {
	worker := newTestWorker(rasterWidth, rasterHeight, 1)
	xs := []float64{10, 40, 55, 30, 12}
	ys := []float64{8, 4, 25, 42, 30}
	shape := &Polygon{worker, len(xs), true, xs, ys}
	checkOutline(t, drawOnCanvas(shape, 1, rasterWidth, rasterHeight), rasterWidth, rasterHeight, 0.5, 1, convexOutline(xs, ys))
}

// TestRotatedRectangleRasterizeKeepsRows pins the rows of two rotated rectangles
// exactly: where the edge samples of the four sides end, and that a row with a
// single pixel, the tip of a corner, is kept. FMA does not move a row: the corners
// lie away from integers, and the error of x + dx*t stays below half a unit of the
// sum.
func TestRotatedRectangleRasterizeKeepsRows(t *testing.T) {
	worker := newTestWorker(rasterWidth, rasterHeight, 1)
	cases := []struct {
		shape *RotatedRectangle
		want  string
	}{
		{&RotatedRectangle{worker, 30, 24, 12, 7, 37}, "18:27-29 19:26-30 20:26-31 21:25-33 22:24-34 23:24-35 24:24-36 25:25-35 26:26-34 27:28-33 28:29-33 29:30-32 30:32-32"},
		{&RotatedRectangle{worker, 20, 20, 10, 6, 70}, "15:18-21 16:16-21 17:16-22 18:16-22 19:16-22 20:17-23 21:17-23 22:18-23 23:18-24 24:18-21 25:19-19"},
	}
	for _, testCase := range cases {
		var got []string
		for _, line := range testCase.shape.Rasterize() {
			got = append(got, fmt.Sprintf("%d:%d-%d", line.Y, line.X1, line.X2))
		}
		if joined := strings.Join(got, " "); joined != testCase.want {
			t.Errorf("%+v:\ngot  %s\nwant %s", *testCase.shape, joined, testCase.want)
		}
	}
}

// TestTriangleRasterizeIgnoresVertexOrderOnSmallGrid repeats the order check for
// every triangle with area on a 4x4 grid, which holds all ties of rows.
func TestTriangleRasterizeIgnoresVertexOrderOnSmallGrid(t *testing.T) {
	const side = 4
	var coordinates [6]int
	var visit func(index int)
	checked := 0
	visit = func(index int) {
		if index < len(coordinates) {
			for value := 0; value < side; value++ {
				coordinates[index] = value
				visit(index + 1)
			}
			return
		}
		v := coordinates
		if (v[2]-v[0])*(v[5]-v[1])-(v[3]-v[1])*(v[4]-v[0]) == 0 {
			return
		}
		want := fmt.Sprint(rasterizeTriangle(v[0], v[1], v[2], v[3], v[4], v[5], nil))
		for _, order := range [][3]int{{0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
			got := fmt.Sprint(rasterizeTriangle(v[2*order[0]], v[2*order[0]+1], v[2*order[1]], v[2*order[1]+1], v[2*order[2]], v[2*order[2]+1], nil))
			if got != want {
				t.Fatalf("triangle %v, order %v: got %s, want %s", v, order, got, want)
			}
		}
		checked++
	}
	visit(0)
	if checked == 0 {
		t.Fatal("no triangle checked")
	}
}
