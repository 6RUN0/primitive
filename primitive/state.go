package primitive

// State is one search candidate: a shape and its alpha. Score caches the energy.
type State struct {
	Worker      *Worker
	Shape       Shape
	Alpha       int
	MutateAlpha bool
	Score       float64
}

// NewState returns a state for shape. alpha 0 starts at 128 and lets the moves
// change it.
func NewState(worker *Worker, shape Shape, alpha int) *State {
	var mutateAlpha bool
	if alpha == 0 {
		alpha = 128
		mutateAlpha = true
	}
	return &State{worker, shape, alpha, mutateAlpha, -1}
}

// Energy returns the score of the picture with the shape added; lower is better.
func (state *State) Energy() float64 {
	if state.Score < 0 {
		state.Score = state.Worker.Energy(state.Shape, state.Alpha)
	}
	return state.Score
}

// DoMove mutates the shape and, if allowed, the alpha. The result undoes the move
// through UndoMove.
func (state *State) DoMove() interface{} {
	rnd := state.Worker.Rnd
	oldState := state.Copy()
	state.Shape.Mutate()
	if state.MutateAlpha {
		state.Alpha = clampInt(state.Alpha+rnd.Intn(21)-10, 1, 255)
	}
	state.Score = -1
	return oldState
}

// UndoMove restores the state saved by DoMove.
func (state *State) UndoMove(undo interface{}) {
	oldState := undo.(*State)
	state.Shape = oldState.Shape
	state.Alpha = oldState.Alpha
	state.Score = oldState.Score
}

// Copy returns an independent state with a copy of the shape.
func (state *State) Copy() Annealable {
	return &State{
		state.Worker, state.Shape.Copy(), state.Alpha, state.MutateAlpha, state.Score}
}
