package primitive

// newTestWorker returns a worker over a width x height target prepared for
// Energy and Mutate calls.
func newTestWorker(width, height int, seed int64) *Worker {
	target := imageToRGBA(buildNoisyTarget(width, height))
	worker := NewWorker(target, seed)
	current := copyRGBA(target)
	worker.Init(current, differenceFull(target, current))
	return worker
}
