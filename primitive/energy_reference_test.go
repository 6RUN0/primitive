package primitive

import (
	"image"
	"math"
	"testing"
)

// The ref* functions are frozen plain versions of what Worker.Energy computes: the
// color, a blended copy of the covered pixels and the change of the error. Any
// rewrite of the kernels has to stay equal to them bit for bit, or the output for a
// given seed changes. refDifferencePartial recovers the total as scoreAfterChange
// does, by rounding.

func refComputeColor(target, current *image.RGBA, lines []Scanline, alpha int) Color {
	var rsum, gsum, bsum, count int64
	a := 0x101 * 255 / alpha
	for _, line := range lines {
		i := target.PixOffset(line.X1, line.Y)
		for x := line.X1; x <= line.X2; x++ {
			tr := int(target.Pix[i])
			tg := int(target.Pix[i+1])
			tb := int(target.Pix[i+2])
			cr := int(current.Pix[i])
			cg := int(current.Pix[i+1])
			cb := int(current.Pix[i+2])
			i += 4
			rsum += int64((tr-cr)*a + cr*0x101)
			gsum += int64((tg-cg)*a + cg*0x101)
			bsum += int64((tb-cb)*a + cb*0x101)
			count++
		}
	}
	if count == 0 {
		return Color{}
	}
	r := clampInt(int(rsum/count)>>8, 0, 255)
	g := clampInt(int(gsum/count)>>8, 0, 255)
	b := clampInt(int(bsum/count)>>8, 0, 255)
	return Color{r, g, b, alpha}
}

func refCopyLines(dst, src *image.RGBA, lines []Scanline) {
	for _, line := range lines {
		a := dst.PixOffset(line.X1, line.Y)
		b := a + (line.X2-line.X1+1)*4
		copy(dst.Pix[a:b], src.Pix[a:b])
	}
}

func refDrawLines(im *image.RGBA, c Color, lines []Scanline) {
	const m = 0xffff
	sr, sg, sb, sa := c.NRGBA().RGBA()
	for _, line := range lines {
		ma := line.Alpha
		sra := sr * ma
		sga := sg * ma
		sba := sb * ma
		saa := sa * ma
		a := (m - saa/m) * 0x101
		i := im.PixOffset(line.X1, line.Y)
		for x := line.X1; x <= line.X2; x++ {
			dr := uint32(im.Pix[i+0])
			dg := uint32(im.Pix[i+1])
			db := uint32(im.Pix[i+2])
			da := uint32(im.Pix[i+3])
			im.Pix[i+0] = uint8((dr*a + sra) / m >> 8)
			im.Pix[i+1] = uint8((dg*a + sga) / m >> 8)
			im.Pix[i+2] = uint8((db*a + sba) / m >> 8)
			im.Pix[i+3] = uint8((da*a + saa) / m >> 8)
			i += 4
		}
	}
}

func refDifferencePartial(target, before, after *image.RGBA, score float64, lines []Scanline) float64 {
	size := target.Bounds().Size()
	w, h := size.X, size.Y
	total := uint64(math.Round(math.Pow(score*255, 2) * float64(w*h*4))) //nolint:staticcheck // QF1005: the reference keeps the kernel as frozen
	for _, line := range lines {
		i := target.PixOffset(line.X1, line.Y)
		for x := line.X1; x <= line.X2; x++ {
			tr := int(target.Pix[i])
			tg := int(target.Pix[i+1])
			tb := int(target.Pix[i+2])
			ta := int(target.Pix[i+3])
			br := int(before.Pix[i])
			bg := int(before.Pix[i+1])
			bb := int(before.Pix[i+2])
			ba := int(before.Pix[i+3])
			ar := int(after.Pix[i])
			ag := int(after.Pix[i+1])
			ab := int(after.Pix[i+2])
			aa := int(after.Pix[i+3])
			i += 4
			dr1 := tr - br
			dg1 := tg - bg
			db1 := tb - bb
			da1 := ta - ba
			dr2 := tr - ar
			dg2 := tg - ag
			db2 := tb - ab
			da2 := ta - aa
			total -= uint64(dr1*dr1 + dg1*dg1 + db1*db1 + da1*da1)
			total += uint64(dr2*dr2 + dg2*dg2 + db2*db2 + da2*da2)
		}
	}
	return math.Sqrt(float64(total)/float64(w*h*4)) / 255
}

func refEnergy(worker *Worker, shape Shape, alpha int) float64 {
	return refEnergyInto(image.NewRGBA(worker.Target.Bounds()), worker, shape, alpha)
}

func refEnergyInto(buffer *image.RGBA, worker *Worker, shape Shape, alpha int) float64 {
	lines := shape.Rasterize()
	color := refComputeColor(worker.Target, worker.Current, lines, alpha)
	refCopyLines(buffer, worker.Current, lines)
	refDrawLines(buffer, color, lines)
	return refDifferencePartial(worker.Target, worker.Current, buffer, worker.Score, lines)
}

// TestEnergyMatchesReference pins Worker.Energy to the frozen kernels with exact
// float64 equality: an optimized kernel that rounds differently passes a
// tolerance check and still changes which shape wins. Each case takes another
// path through Energy: skipping alpha on an image that has it gives a wrong error,
// and stale or wrapped tables a wrong color.
func TestEnergyMatchesReference(t *testing.T) {
	opaque := buildNoisyTarget(48, 48)
	translucent := buildNoisyTarget(48, 48)
	for i := 3; i < len(translucent.Pix); i += 4 {
		translucent.Pix[i] = uint8(i)
	}
	cases := []struct {
		name          string
		target        image.Image
		background    Color
		wantOpaque    bool
		withoutTables bool
	}{
		{"opaque", opaque, Color{128, 128, 128, 255}, true, false},
		{"opaque without tables", opaque, Color{128, 128, 128, 255}, true, true},
		{"transparent background", opaque, Color{128, 128, 128, 90}, false, false},
		{"transparent background without tables", opaque, Color{128, 128, 128, 90}, false, true},
		{"translucent target", translucent, Color{128, 128, 128, 255}, false, false},
		{"translucent target without tables", translucent, Color{128, 128, 128, 255}, false, true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := NewModel(testCase.target, testCase.background, 96, 1, 1)
			worker := model.Workers[0]
			if worker.isOpaque != testCase.wantOpaque {
				t.Fatalf("isOpaque = %v", worker.isOpaque)
			}
			if testCase.withoutTables {
				model.tables, worker.tables = nil, nil
			}
			for step := 0; step < 4; step++ {
				model.Step(ShapeTypeTriangle, 128, 0, 0.5)
			}
			worker.Init(model.Current, model.Score)
			for _, shapeType := range allShapeTypes {
				for _, alpha := range []int{1, 128, 255} {
					for i := 0; i < 100; i++ {
						state := worker.RandomState(shapeType, alpha, 0.5)
						state.Shape.Mutate()
						got := worker.Energy(state.Shape, state.Alpha)
						want := refEnergy(worker, state.Shape, state.Alpha)
						if got != want {
							t.Fatalf("mode %d alpha %d: got %v, want %v", shapeType, alpha, got, want)
						}
					}
				}
			}
		})
	}
}

// TestScanlinesDoNotOverlap pins what errorAfterBlend relies on: a pixel listed
// twice is blended twice by drawLines and once by the kernel, and Energy then
// ranks shapes by a picture that Model.Add does not draw.
func TestScanlinesDoNotOverlap(t *testing.T) {
	worker := newTestWorker(40, 30, 9)
	for _, shapeType := range allShapeTypes {
		for i := 0; i < 500; i++ {
			state := worker.RandomState(shapeType, 128, 4)
			state.Shape.Mutate()
			covered := map[[2]int]bool{}
			for _, line := range state.Shape.Rasterize() {
				for x := line.X1; x <= line.X2; x++ {
					if covered[[2]int{x, line.Y}] {
						t.Fatalf("mode %d: pixel %d,%d listed twice", shapeType, x, line.Y)
					}
					covered[[2]int{x, line.Y}] = true
				}
			}
		}
	}
}
