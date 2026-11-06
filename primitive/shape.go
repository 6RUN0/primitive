package primitive

import "github.com/fogleman/gg"

// Shape is a candidate of the search. Coordinates are in target pixels.
type Shape interface {
	// Rasterize returns the covered scanlines, cropped to the image.
	Rasterize() []Scanline
	// Copy returns an independent copy.
	Copy() Shape
	// Mutate changes the shape at random, keeping it valid.
	Mutate()
	// Draw adds the shape to the path of dc, or strokes it, at the output scale.
	Draw(dc *gg.Context, scale float64)
	// SVG returns the element of the shape with attrs added.
	SVG(attrs string) string
}

// ShapeType selects the shapes of a search; the values are those of the -m flag.
type ShapeType int

// The shape types.
const (
	ShapeTypeAny ShapeType = iota
	ShapeTypeTriangle
	ShapeTypeRectangle
	ShapeTypeEllipse
	ShapeTypeCircle
	ShapeTypeRotatedRectangle
	ShapeTypeQuadratic
	ShapeTypeRotatedEllipse
	ShapeTypePolygon
)
