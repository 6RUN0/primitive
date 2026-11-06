package primitive

import (
	"image"
	"math"
	"math/rand"
	"testing"
)

// buildFlatTarget returns a w x h picture of a few flat rectangles, which a run of
// opaque rectangles can match exactly.
func buildFlatTarget(rnd *rand.Rand, w, h int) *image.RGBA {
	target := newUniformRGBA(w, h, Color{rnd.Intn(256), rnd.Intn(256), rnd.Intn(256), 255})
	for block := 0; block < 3; block++ {
		left, top := rnd.Intn(w), rnd.Intn(h)
		fill := [3]uint8{uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), uint8(rnd.Intn(256))}
		for y := top; y < h; y++ {
			for x := left; x < w; x++ {
				copy(target.Pix[target.PixOffset(x, y):], fill[:])
			}
		}
	}
	return target
}

// TestScoreAfterChangeRemovesAllError pins the recovery of the integer total from
// the float score: truncation lands one below the true total on some values, and a
// shape that removes all the remaining error then wraps uint64 and the score jumps
// to millions.
func TestScoreAfterChangeRemovesAllError(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {3, 3}, {16, 9}, {256, 256}, {10000, 10000}} {
		channels := size[0] * size[1] * 4
		for total := uint64(1); total < 20000; total++ {
			score := math.Sqrt(float64(total)/float64(channels)) / 255
			if got := scoreAfterChange(size[0], size[1], score, total, 0); got != 0 {
				t.Fatalf("%v: total %d removed, score %v", size, total, got)
			}
		}
	}
}

// TestScoreMatchesFullOnFlatGraphics pins Score where the search can reach an exact
// match: a wrapped score stops -ma and -mp from firing and makes the search reject
// the best shapes, so the score has to follow the picture down to 0.
func TestScoreMatchesFullOnFlatGraphics(t *testing.T) {
	solved := 0
	for seed := int64(0); seed < 300; seed++ {
		rnd := rand.New(rand.NewSource(seed))
		w, h := 2+rnd.Intn(12), 2+rnd.Intn(12)
		target := buildFlatTarget(rnd, w, h)
		background := Color{rnd.Intn(256), rnd.Intn(256), rnd.Intn(256), 255}
		model := NewModel(target, background, w, 1, seed)
		model.Candidates = 30
		for step := 0; step < 40; step++ {
			model.Step(ShapeTypeRectangle, 255, 0, 0.5)
			if full := differenceFull(model.Target, model.Current); math.Abs(model.Score-full) > 1e-9 {
				t.Fatalf("seed %d step %d: score %v, full %v", seed, step, model.Score, full)
			}
		}
		if model.Score == 0 {
			solved++
		}
	}
	if solved == 0 {
		t.Error("no run reached an exact match, the test proves nothing")
	}
}

// TestScoreMatchesFullOnTransparentPictures pins the four-channel path of Model.Add:
// the opaque kernel there ignores alpha, and -ma and -mp then read a wrong score.
func TestScoreMatchesFullOnTransparentPictures(t *testing.T) {
	translucent := buildNoisyTarget(32, 32)
	for i := 3; i < len(translucent.Pix); i += 4 {
		translucent.Pix[i] = uint8(40 + i%200)
	}
	cases := []struct {
		name       string
		background Color
		alpha      int
	}{
		{"translucent target", Color{128, 128, 128, 255}, 128},
		{"translucent target and background", Color{10, 200, 30, 90}, 128},
		{"per-shape alpha", Color{10, 200, 30, 90}, 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := NewModel(translucent, testCase.background, 32, 2, 3)
			for step := 0; step < 12; step++ {
				model.Step(ShapeTypeTriangle, testCase.alpha, 1, 0.5)
				if full := differenceFull(model.Target, model.Current); math.Abs(model.Score-full) > 1e-9 {
					t.Fatalf("step %d: score %v, full %v", step, model.Score, full)
				}
			}
		})
	}
}

// TestIsOpaqueSeesEveryPixel pins the guard of estimateEnergy and of the opaque
// kernel: a single translucent pixel anywhere has to turn both off, or the search
// ranks shapes by an error without alpha.
func TestIsOpaqueSeesEveryPixel(t *testing.T) {
	const width, height = 5, 4
	if !isOpaque(newUniformRGBA(width, height, Color{1, 2, 3, 255})) {
		t.Error("an opaque picture was rejected")
	}
	for _, pixel := range []int{0, 1, width*height/2 + 1, width*height - 1} {
		im := newUniformRGBA(width, height, Color{1, 2, 3, 255})
		im.Pix[pixel*4+3] = 254
		if isOpaque(im) {
			t.Errorf("pixel %d of %d translucent, reported opaque", pixel, width*height)
		}
	}
}
