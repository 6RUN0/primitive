package primitive

import (
	"fmt"
	"image"
	"testing"
)

// BenchmarkEnergy prices one candidate on each path through Worker.Energy. The
// frozen kernels of energy_reference_test.go are the base for benchstat.
//
//	go test ./primitive -run '^$' -bench Energy -benchtime 30000x -count 6
func BenchmarkEnergy(b *testing.B) {
	model := NewModel(buildNoisyTarget(256, 256), Color{128, 128, 128, 255}, 256, 1, 1)
	worker := model.Workers[0]
	for step := 0; step < 3; step++ {
		model.Step(ShapeTypeTriangle, 128, 0, 0.5)
	}
	worker.Init(model.Current, model.Score)
	buffer := image.NewRGBA(model.Target.Bounds())
	paths := []struct {
		name   string
		energy func(shape Shape) float64
	}{
		{"reference", func(shape Shape) float64 { return refEnergyInto(buffer, worker, shape, 128) }},
		{"kernels", func(shape Shape) float64 {
			tables := worker.tables
			worker.tables = nil
			energy := worker.Energy(shape, 128)
			worker.tables = tables
			return energy
		}},
		{"tables", func(shape Shape) float64 { return worker.Energy(shape, 128) }},
		{"moments", func(shape Shape) float64 {
			worker.isEstimating = true
			energy := worker.Energy(shape, 128)
			worker.isEstimating = false
			return energy
		}},
	}
	for _, shapeType := range allShapeTypes {
		shapes := make([]Shape, 256)
		for i := range shapes {
			shapes[i] = worker.RandomState(shapeType, 128, 0.5).Shape
		}
		for _, path := range paths {
			b.Run(fmt.Sprintf("mode=%d/path=%s", shapeType, path.name), func(b *testing.B) {
				b.ReportAllocs()
				var total float64
				for i := 0; b.Loop(); i++ {
					total += path.energy(shapes[i&255])
				}
				if total < 0 {
					b.Fatal("negative energy")
				}
			})
		}
	}
}
