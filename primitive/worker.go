package primitive

import (
	"image"
	"math/rand"

	"github.com/golang/freetype/raster"
)

// Worker runs one search with its own buffers and random stream, so that workers
// share only read access to the target and the current picture.
type Worker struct {
	W, H       int
	Target     *image.RGBA
	Current    *image.RGBA
	Rasterizer *raster.Rasterizer
	Lines      []Scanline
	Rnd        *rand.Rand
	Score      float64
	Counter    int
	// tables and isOpaque are set by NewModel. nil tables select the pixel kernels.
	tables   *pixelTables
	isOpaque bool
	// isEstimating ranks candidates by estimateEnergy until the next Init.
	isEstimating bool
}

// NewWorker returns a worker for target whose random stream depends only on
// seed. target must start at the origin, unlike the argument of NewModel: the
// pixel kernels index it from zero. Init must run before the first search.
func NewWorker(target *image.RGBA, seed int64) *Worker {
	w := target.Bounds().Size().X
	h := target.Bounds().Size().Y
	worker := Worker{}
	worker.W = w
	worker.H = h
	worker.Target = target
	worker.Rasterizer = raster.NewRasterizer(w, h)
	worker.Lines = make([]Scanline, 0, 4096)
	worker.Rnd = rand.New(rand.NewSource(seed))
	return &worker
}

// Init prepares a search over current, whose score against the target is score.
func (worker *Worker) Init(current *image.RGBA, score float64) {
	worker.Current = current
	worker.Score = score
	worker.Counter = 0
	worker.isEstimating = false
}

// Energy returns the score of current with shape added at the given alpha.
func (worker *Worker) Energy(shape Shape, alpha int) float64 {
	worker.Counter++
	lines := shape.Rasterize()
	if worker.isEstimating {
		return worker.estimateEnergy(lines, alpha)
	}
	var color Color
	var before uint64
	if worker.tables != nil {
		color, before = worker.tables.colorAndError(lines, alpha)
	} else {
		color, before = colorAndError(worker.Target, worker.Current, lines, alpha)
	}
	var after uint64
	if worker.isOpaque {
		after = errorAfterBlendOpaque(worker.Target, worker.Current, color, lines)
	} else {
		after = errorAfterBlend(worker.Target, worker.Current, color, lines)
	}
	return scoreAfterChange(worker.W, worker.H, worker.Score, before, after)
}

// BestHillClimbState runs m hill climbs from the best of n random shapes each, with
// limit age, and returns the best result.
func (worker *Worker) BestHillClimbState(t ShapeType, a, n, age, m int, maxQuadWidth float64) *State {
	var bestEnergy float64
	var bestState *State
	for i := 0; i < m; i++ {
		state := worker.BestRandomState(t, a, n, maxQuadWidth)
		before := state.Energy()
		state = HillClimb(state, age).(*State)
		energy := worker.finalEnergy(state)
		vv("%dx random: %.6f -> %dx hill climb: %.6f\n", n, before, age, energy)
		if i == 0 || energy < bestEnergy {
			bestEnergy = energy
			bestState = state
		}
	}
	return bestState
}

// finalEnergy scores the result of a climb exactly when the climb ran on estimates:
// climbs and workers compete on exact scores, and Model.Add draws the winner.
func (worker *Worker) finalEnergy(state *State) float64 {
	if !worker.isEstimating {
		return state.Energy()
	}
	worker.isEstimating = false
	state.Score = -1
	energy := state.Energy()
	worker.isEstimating = true
	return energy
}

// BestRandomState returns the best of n random shapes of type t.
func (worker *Worker) BestRandomState(t ShapeType, a, n int, maxQuadWidth float64) *State {
	var bestEnergy float64
	var bestState *State
	for i := 0; i < n; i++ {
		state := worker.RandomState(t, a, maxQuadWidth)
		energy := state.Energy()
		if i == 0 || energy < bestEnergy {
			bestEnergy = energy
			bestState = state
		}
	}
	return bestState
}

// RandomState returns a random shape of type t, ShapeTypeAny picks the type at
// random.
func (worker *Worker) RandomState(t ShapeType, a int, maxQuadWidth float64) *State {
	switch t {
	default:
		return worker.RandomState(ShapeType(worker.Rnd.Intn(8)+1), a, maxQuadWidth)
	case ShapeTypeTriangle:
		return NewState(worker, NewRandomTriangle(worker), a)
	case ShapeTypeRectangle:
		return NewState(worker, NewRandomRectangle(worker), a)
	case ShapeTypeEllipse:
		return NewState(worker, NewRandomEllipse(worker), a)
	case ShapeTypeCircle:
		return NewState(worker, NewRandomCircle(worker), a)
	case ShapeTypeRotatedRectangle:
		return NewState(worker, NewRandomRotatedRectangle(worker), a)
	case ShapeTypeQuadratic:
		return NewState(worker, NewRandomQuadratic(worker, maxQuadWidth), a)
	case ShapeTypeRotatedEllipse:
		return NewState(worker, NewRandomRotatedEllipse(worker), a)
	case ShapeTypePolygon:
		return NewState(worker, NewRandomPolygon(worker, 4, false), a)
	}
}
