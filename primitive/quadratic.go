package primitive

import (
	"github.com/fogleman/gg"
	"github.com/golang/freetype/raster"
)

// Quadratic is a stroked quadratic Bezier curve of Width pixels.
type Quadratic struct {
	Worker       *Worker
	X1, Y1       float64
	X2, Y2       float64
	X3, Y3       float64
	Width        float64
	MaxQuadWidth float64
}

// NewRandomQuadratic returns a curve with control points near a random point and
// a stroke width limited by maxQuadWidth.
func NewRandomQuadratic(worker *Worker, maxQuadWidth float64) *Quadratic {
	rnd := worker.Rnd
	x1 := rnd.Float64() * float64(worker.W)
	y1 := rnd.Float64() * float64(worker.H)
	x2 := x1 + rnd.Float64()*40 - 20
	y2 := y1 + rnd.Float64()*40 - 20
	x3 := x2 + rnd.Float64()*40 - 20
	y3 := y2 + rnd.Float64()*40 - 20
	width := 1.0 / 2
	q := &Quadratic{worker, x1, y1, x2, y2, x3, y3, width, float64(maxQuadWidth)}
	q.Mutate()
	return q
}

// Draw strokes the curve on dc; scale converts the width to output pixels.
func (q *Quadratic) Draw(dc *gg.Context, scale float64) {
	dc.MoveTo(q.X1, q.Y1)
	dc.QuadraticTo(q.X2, q.Y2, q.X3, q.Y3)
	dc.SetLineWidth(q.Width * scale)
	dc.Stroke()
}

// SVG returns the element of the shape with attrs added. attrs must set the stroke:
// Model.SVG writes what curves share on the group. Both control points of q are
// relative to the start of the segment.
func (q *Quadratic) SVG(attrs string) string {
	x1, y1 := roundSVGCoordinate(q.X1), roundSVGCoordinate(q.Y1)
	data := "M" + joinSVGPathNumbers(x1, y1) + "q" + joinSVGPathNumbers(
		roundSVGCoordinate(q.X2)-x1, roundSVGCoordinate(q.Y2)-y1,
		roundSVGCoordinate(q.X3)-x1, roundSVGCoordinate(q.Y3)-y1)
	return buildSVGElement("path", attrs, "d=\""+data+"\"")
}

// Copy returns an independent copy.
func (q *Quadratic) Copy() Shape {
	a := *q
	return &a
}

// Mutate moves one control point or changes the width, repeating until the curve
// is valid.
func (q *Quadratic) Mutate() {
	const m = 16
	w := q.Worker.W
	h := q.Worker.H
	rnd := q.Worker.Rnd
	for {
		switch rnd.Intn(4) {
		case 0:
			q.X1 = clamp(q.X1+rnd.NormFloat64()*16, -m, float64(w-1+m))
			q.Y1 = clamp(q.Y1+rnd.NormFloat64()*16, -m, float64(h-1+m))
		case 1:
			q.X2 = clamp(q.X2+rnd.NormFloat64()*16, -m, float64(w-1+m))
			q.Y2 = clamp(q.Y2+rnd.NormFloat64()*16, -m, float64(h-1+m))
		case 2:
			q.X3 = clamp(q.X3+rnd.NormFloat64()*16, -m, float64(w-1+m))
			q.Y3 = clamp(q.Y3+rnd.NormFloat64()*16, -m, float64(h-1+m))
		case 3:
			q.Width = clamp(q.Width+rnd.NormFloat64(), 0.5, q.MaxQuadWidth)
			// q.Width = clamp(rnd.NormFloat64(), 0.5, 64)
		}
		if q.Valid() {
			break
		}
	}
}

// Valid reports whether the end points are the farthest pair of the three points.
func (q *Quadratic) Valid() bool {
	dx12 := int(q.X1 - q.X2)
	dy12 := int(q.Y1 - q.Y2)
	dx23 := int(q.X2 - q.X3)
	dy23 := int(q.Y2 - q.Y3)
	dx13 := int(q.X1 - q.X3)
	dy13 := int(q.Y1 - q.Y3)
	d12 := dx12*dx12 + dy12*dy12
	d23 := dx23*dx23 + dy23*dy23
	d13 := dx13*dx13 + dy13*dy13
	return d13 > d12 && d13 > d23
}

// Rasterize returns the covered scanlines, cropped to the image.
func (q *Quadratic) Rasterize() []Scanline {
	var path raster.Path
	p1 := fixp(q.X1, q.Y1)
	p2 := fixp(q.X2, q.Y2)
	p3 := fixp(q.X3, q.Y3)
	path.Start(p1)
	path.Add2(p2, p3)
	width := fix(q.Width)
	return strokePath(q.Worker, path, width, raster.RoundCapper, raster.RoundJoiner)
}
