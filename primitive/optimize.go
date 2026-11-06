package primitive

// Annealable is a search state that HillClimb can change and restore.
type Annealable interface {
	Energy() float64
	DoMove() interface{}
	UndoMove(interface{})
	Copy() Annealable
}

// HillClimb returns the best state reached by random moves from state, stopping
// after maxAge moves in a row without an improvement. state stays unchanged.
func HillClimb(state Annealable, maxAge int) Annealable {
	state = state.Copy()
	bestState := state.Copy()
	bestEnergy := state.Energy()
	step := 0
	for age := 0; age < maxAge; age++ {
		undo := state.DoMove()
		energy := state.Energy()
		if energy >= bestEnergy {
			state.UndoMove(undo)
		} else {
			// fmt.Printf("step: %d, energy: %.6f\n", step, energy)
			bestEnergy = energy
			bestState = state.Copy()
			age = -1
		}
		step++
	}
	return bestState
}
