// Package primitive reproduces an image with geometric shapes, adding the one that
// lowers the error most at each step.
package primitive

import (
	"fmt"
	"image"
	"strings"
	"sync"

	"github.com/fogleman/gg"
)

// DefaultCandidates is the default of Model.Candidates. Against 500, on 5 images
// with 4 seeds and j=1: -53% CPU at +0.6% rmse over 100 shapes, -59% at +0.9% over
// 200 shapes. Re-measure with -nc, a change here rewrites the golden files.
const DefaultCandidates = 125

// Model holds the picture built so far and the shapes that led to it.
type Model struct {
	Sw, Sh, Vw, Vh int
	Scale          float64
	Background     Color
	Target         *image.RGBA
	Current        *image.RGBA
	Context        *gg.Context
	Score          float64
	Shapes         []Shape
	Colors         []Color
	Scores         []float64
	Workers        []*Worker
	// Candidates is the number of random shapes behind each climb of Step, at
	// least 1: NewModel sets DefaultCandidates, and 0 leaves a climb without a start.
	Candidates int
	tables     *pixelTables
}

// NewModel returns a model of target rendered at size pixels along the longer
// side. Each worker gets its own stream from deriveWorkerSeed. Output repeats
// for equal seed and numWorkers.
//
// target must have pixels, size must be at least 1 and numWorkers at least 1;
// the CLI validates them, and NewModel does not. target may start at any origin:
// Model.Target is a copy with its origin at zero.
func NewModel(target image.Image, background Color, size, numWorkers int, seed int64) *Model {
	w := target.Bounds().Size().X
	h := target.Bounds().Size().Y
	aspect := float64(w) / float64(h)
	var sw, sh int
	var scale float64
	if aspect >= 1 {
		sw = size
		sh = max(1, int(float64(size)/aspect))
		scale = float64(size) / float64(w)
	} else {
		sw = max(1, int(float64(size)*aspect))
		sh = size
		scale = float64(size) / float64(h)
	}

	model := &Model{}
	model.Sw = sw
	model.Sh = sh
	// Shapes are stored in target pixels, whatever the output size.
	model.Vw = w
	model.Vh = h
	model.Scale = scale
	model.Background = background
	model.Target = imageToRGBA(target)
	model.Current = uniformRGBA(model.Target.Bounds(), background.NRGBA())
	model.Score = differenceFull(model.Target, model.Current)
	model.Context = model.newContext()
	model.Candidates = DefaultCandidates
	model.tables = newPixelTables(model.Target, model.Current)
	opaque := isOpaque(model.Target) && isOpaque(model.Current)
	for i := 0; i < numWorkers; i++ {
		worker := NewWorker(model.Target, deriveWorkerSeed(seed, i))
		worker.tables = model.tables
		worker.isOpaque = opaque
		model.Workers = append(model.Workers, worker)
	}
	return model
}

// deriveWorkerSeed returns the seed of worker index with the splitmix64 finalizer.
// seed+index would give adjacent seeds all workers but one in common, and math/rand
// reduces seeds modulo 2^31-1, so small negative and huge values would meet too.
func deriveWorkerSeed(seed int64, index int) int64 {
	mixed := uint64(seed) + uint64(index)*0x9E3779B97F4A7C15
	mixed = (mixed ^ (mixed >> 30)) * 0xBF58476D1CE4E5B9
	mixed = (mixed ^ (mixed >> 27)) * 0x94D049BB133111EB
	return int64(mixed ^ (mixed >> 31))
}

func (model *Model) newContext() *gg.Context {
	dc := gg.NewContext(model.Sw, model.Sh)
	dc.Scale(model.Scale, model.Scale)
	dc.Translate(0.5, 0.5)
	dc.SetColor(model.Background.NRGBA())
	dc.Clear()
	return dc
}

// Frames returns the background, then the picture after each shape that lowered
// the score by at least scoreDelta since the last kept frame. The last frame is
// always the finished picture, whatever the last shapes gained.
func (model *Model) Frames(scoreDelta float64) []image.Image {
	var result []image.Image
	dc := model.newContext()
	result = append(result, imageToRGBA(dc.Image()))
	previous := 10.0
	for i, shape := range model.Shapes {
		c := model.Colors[i]
		dc.SetRGBA255(c.R, c.G, c.B, c.A)
		shape.Draw(dc, model.Scale)
		dc.Fill()
		score := model.Scores[i]
		delta := previous - score
		if delta >= scoreDelta || i == len(model.Shapes)-1 {
			previous = score
			result = append(result, imageToRGBA(dc.Image()))
		}
	}
	return result
}

// SVG returns the picture as a standalone SVG document without line breaks. The
// group repeats Translate of newContext, and its clip stays in the translated
// coordinates. What all shapes share sits on the group.
func (model *Model) SVG() string {
	bg := model.Background
	groupAlpha := findCommonAlpha(model.Colors)
	curveCount, sharedWidth := surveySVGCurves(model.Shapes)
	hasOnlyCurves := curveCount > 0 && curveCount == len(model.Shapes)

	var document strings.Builder
	fmt.Fprintf(&document, "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 %d %d\">", model.Vw, model.Vh)
	background := fmt.Sprintf("width=\"100%%\" height=\"100%%\" fill=\"#%02x%02x%02x\"", bg.R, bg.G, bg.B)
	if bg.A != 255 {
		background += " fill-opacity=\"" + formatSVGOpacity(bg.A) + "\""
	}
	document.WriteString(buildSVGElement("rect", "", background))
	// The size in the id keeps documents inlined in one page apart: the first
	// definition of an id wins, and equal ids then mean an equal clip.
	clipID := fmt.Sprintf("c%dx%d", model.Vw, model.Vh)
	fmt.Fprintf(&document, "<clipPath id=\"%s\"><path d=\"M-.5-.5h%dv%dH-.5z\"/></clipPath>", clipID, model.Vw, model.Vh)

	fmt.Fprintf(&document, "<g clip-path=\"url(#%s)\" transform=\"translate(.5 .5)\"", clipID)
	if groupAlpha != 255 {
		if curveCount < len(model.Shapes) {
			document.WriteString(" fill-opacity=\"" + formatSVGOpacity(groupAlpha) + "\"")
		}
		if curveCount > 0 {
			document.WriteString(" stroke-opacity=\"" + formatSVGOpacity(groupAlpha) + "\"")
		}
	}
	if hasOnlyCurves {
		document.WriteString(" fill=\"none\"")
	}
	if curveCount > 0 {
		// gg strokes with round caps, the SVG default is butt.
		document.WriteString(" stroke-linecap=\"round\"")
	}
	if sharedWidth != "" {
		document.WriteString(" stroke-width=\"" + sharedWidth + "\"")
	}
	document.WriteString(">")

	for i, shape := range model.Shapes {
		c := model.Colors[i]
		paint, opacity := "fill", "fill-opacity"
		curve, isCurve := shape.(*Quadratic)
		var attrs []string
		if isCurve {
			paint, opacity = "stroke", "stroke-opacity"
			if !hasOnlyCurves {
				attrs = append(attrs, "fill=\"none\"")
			}
		}
		attrs = append(attrs, fmt.Sprintf("%s=\"#%02x%02x%02x\"", paint, c.R, c.G, c.B))
		if c.A != groupAlpha {
			attrs = append(attrs, opacity+"=\""+formatSVGOpacity(c.A)+"\"")
		}
		if isCurve && sharedWidth == "" {
			attrs = append(attrs, "stroke-width=\""+formatSVGCoordinate(curve.Width)+"\"")
		}
		document.WriteString(shape.SVG(strings.Join(attrs, " ")))
	}
	document.WriteString("</g></svg>")
	return document.String()
}

// Add draws shape with the color that fits the target best and records the new
// score. alpha is the opacity of that color, from 1 to 255.
func (model *Model) Add(shape Shape, alpha int) {
	lines := shape.Rasterize()
	color, before := colorAndError(model.Target, model.Current, lines, alpha)
	after := errorAfterBlend(model.Target, model.Current, color, lines)
	score := scoreAfterChange(model.Vw, model.Vh, model.Score, before, after)
	drawLines(model.Current, color, lines)
	if model.tables != nil {
		model.tables.updateRows(model.Target, model.Current, lines)
	}

	model.Score = score
	model.Shapes = append(model.Shapes, shape)
	model.Colors = append(model.Colors, color)
	model.Scores = append(model.Scores, score)

	model.Context.SetRGBA255(color.R, color.G, color.B, color.A)
	shape.Draw(model.Context, model.Scale)
}

// Step searches for the best shape of shapeType and adds it, then up to repeat more
// shapes grown from it by a shorter hill climb. alpha 0 lets the search choose the
// opacity per shape. It returns the number of shapes evaluated.
func (model *Model) Step(shapeType ShapeType, alpha, repeat int, maxQuadWidth float64) int {
	state := model.runWorkers(shapeType, alpha, model.Candidates, 50, 8, maxQuadWidth)
	model.Add(state.Shape, state.Alpha)

	for i := 0; i < repeat; i++ {
		state.Worker.Init(model.Current, model.Score)
		a := state.Energy()
		state = HillClimb(state, 100).(*State)
		b := state.Energy()
		if a == b {
			break
		}
		model.Add(state.Shape, state.Alpha)
	}

	counter := 0
	for _, worker := range model.Workers {
		counter += worker.Counter
	}
	return counter
}

func (model *Model) runWorkers(t ShapeType, alphaVal, n, age, m int, maxQuadWidth float64) *State {
	// CPU grows with the climbs and rmse barely does: 16 climbs instead of 8 give
	// -1.2% rmse for twice the CPU (-m 1, lenna at 256 px, 30 shapes, 8 seeds). So
	// the climb count does not follow the worker count.
	wn := min(len(model.Workers), m)
	states := make([]*State, wn)
	var running sync.WaitGroup
	for i := 0; i < wn; i++ {
		worker := model.Workers[i]
		worker.Init(model.Current, model.Score)
		worker.isEstimating = worker.isOpaque && worker.tables != nil && isEstimated(t)
		wm := m / wn
		if i < m%wn {
			wm++
		}
		running.Go(func() {
			states[i] = worker.BestHillClimbState(t, alphaVal, n, age, wm, maxQuadWidth)
		})
	}
	running.Wait()
	// Results are compared in worker order, not in order of completion: on equal
	// energy the lowest worker wins, so the output does not depend on the scheduler.
	var bestEnergy float64
	var bestState *State
	for i, state := range states {
		energy := state.Energy()
		if i == 0 || energy < bestEnergy {
			bestEnergy = energy
			bestState = state
		}
	}
	return bestState
}

// isEstimated reports whether a search of type t ranks candidates by estimateEnergy.
// A search of curves only stays exact: strokes break into short runs, where the
// float terms of each run cost more than its pixels, +11% CPU on 5 images, 4 seeds,
// 50 shapes at -r 256. ShapeTypeAny estimates all shapes, curves too: -28% CPU there.
// The other types lost no rmse beyond noise.
func isEstimated(t ShapeType) bool {
	return t != ShapeTypeQuadratic
}
