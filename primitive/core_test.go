package primitive

import (
	"image"
	"math"
	"testing"
)

func newUniformRGBA(width, height int, c Color) *image.RGBA {
	return uniformRGBA(image.Rect(0, 0, width, height), c.NRGBA())
}

// TestComputeColorSolvesBlend pins the color choice: with alpha below 255 the
// fill overshoots the target so that the blended result lands on it, and the
// overshoot is clamped to a valid channel value.
func TestComputeColorSolvesBlend(t *testing.T) {
	cases := []struct {
		name    string
		target  Color
		current Color
		alpha   int
		want    Color
	}{
		{"opaque fill equals target", Color{200, 10, 90, 255}, Color{0, 0, 0, 255}, 255, Color{200, 10, 90, 255}},
		{"half alpha overshoots", Color{150, 100, 50, 255}, Color{100, 100, 100, 255}, 128, Color{200, 100, 0, 128}},
		{"upper clamp", Color{255, 255, 255, 255}, Color{100, 100, 100, 255}, 64, Color{255, 255, 255, 64}},
		{"lower clamp", Color{0, 0, 0, 255}, Color{100, 100, 100, 255}, 64, Color{0, 0, 0, 64}},
	}
	lines := []Scanline{{Y: 1, X1: 0, X2: 3, Alpha: 0xffff}}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			target := newUniformRGBA(4, 3, testCase.target)
			current := newUniformRGBA(4, 3, testCase.current)
			if got, _ := colorAndError(target, current, lines, testCase.alpha); got != testCase.want {
				t.Errorf("got %+v, want %+v", got, testCase.want)
			}
		})
	}
}

// TestComputeColorAveragesCoveredPixels pins that only scanline pixels count and
// that the mean is taken per pixel, not per scanline.
func TestComputeColorAveragesCoveredPixels(t *testing.T) {
	target := newUniformRGBA(4, 2, Color{0, 0, 0, 255})
	target.Pix[0] = 240
	current := newUniformRGBA(4, 2, Color{0, 0, 0, 255})
	lines := []Scanline{{Y: 0, X1: 0, X2: 2, Alpha: 0xffff}, {Y: 1, X1: 3, X2: 3, Alpha: 0xffff}}
	if got, _ := colorAndError(target, current, lines, 255); got != (Color{60, 0, 0, 255}) {
		t.Errorf("got %+v", got)
	}
}

// TestComputeColorWithoutPixels pins the empty shape: a shape fully outside the
// image has no pixels to average and must not divide by zero.
func TestComputeColorWithoutPixels(t *testing.T) {
	image := newUniformRGBA(2, 2, Color{1, 2, 3, 255})
	if got, _ := colorAndError(image, image, nil, 128); got != (Color{}) {
		t.Errorf("got %+v, want the zero color", got)
	}
}

// TestDrawLinesBlendsOnlyCoveredPixels pins source-over blending with both the
// color alpha and the scanline coverage, and that pixels off the scanlines stay.
func TestDrawLinesBlendsOnlyCoveredPixels(t *testing.T) {
	cases := []struct {
		name     string
		color    Color
		coverage uint32
		wantRed  uint8
	}{
		{"opaque full coverage", Color{255, 0, 0, 255}, 0xffff, 255},
		{"half alpha", Color{255, 0, 0, 128}, 0xffff, 128},
		{"half coverage", Color{255, 0, 0, 255}, 0x8000, 128},
		{"transparent color", Color{255, 0, 0, 0}, 0xffff, 0},
		{"no coverage", Color{255, 0, 0, 255}, 0, 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			canvas := newUniformRGBA(3, 2, Color{0, 0, 0, 255})
			drawLines(canvas, testCase.color, []Scanline{{Y: 1, X1: 1, X2: 2, Alpha: testCase.coverage}})
			if got := canvas.RGBAAt(1, 1); got.R != testCase.wantRed || got.G != 0 || got.A != 255 {
				t.Errorf("covered pixel: got %v, want red %d", got, testCase.wantRed)
			}
			if got := canvas.RGBAAt(0, 1); got.R != 0 {
				t.Errorf("pixel left of the line: got %v", got)
			}
			if got := canvas.RGBAAt(1, 0); got.R != 0 {
				t.Errorf("pixel above the line: got %v", got)
			}
		})
	}
}

// TestDifferenceFullIsRootMeanSquare pins the score scale: 0 for equal images,
// 1 when all four channels differ by 255, and each channel weighs one quarter.
func TestDifferenceFullIsRootMeanSquare(t *testing.T) {
	black := newUniformRGBA(3, 2, Color{0, 0, 0, 255})
	cases := []struct {
		name  string
		other *image.RGBA
		want  float64
	}{
		{"equal", newUniformRGBA(3, 2, Color{0, 0, 0, 255}), 0},
		{"color channels", newUniformRGBA(3, 2, Color{255, 255, 255, 255}), math.Sqrt(0.75)},
		{"alpha channel", newUniformRGBA(3, 2, Color{0, 0, 0, 0}), 0.5},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := differenceFull(black, testCase.other); math.Abs(got-testCase.want) > 1e-12 {
				t.Errorf("got %v, want %v", got, testCase.want)
			}
		})
	}
}

// TestScoreAfterChangeTracksFull pins the incremental score used by the search:
// it must stay equal to the full recomputation after every drawn shape, otherwise
// shapes are ranked by a drifting value.
func TestScoreAfterChangeTracksFull(t *testing.T) {
	worker := newTestWorker(40, 30, 3)
	current := newUniformRGBA(40, 30, Color{20, 40, 60, 255})
	score := differenceFull(worker.Target, current)
	for i := 0; i < 200; i++ {
		lines := NewRandomTriangle(worker).Rasterize()
		color, before := colorAndError(worker.Target, current, lines, 128)
		after := errorAfterBlend(worker.Target, current, color, lines)
		drawLines(current, color, lines)
		score = scoreAfterChange(40, 30, score, before, after)
		if want := differenceFull(worker.Target, current); math.Abs(score-want) > 1e-6 {
			t.Fatalf("shape %d: partial %v, full %v", i, score, want)
		}
	}
}

// TestBlendKeepsOpaquePixels pins the proof behind errorAfterBlendOpaque: alpha
// has to stay at exactly 255 for every color alpha and coverage, or skipping the
// channel changes the score of an opaque picture.
func TestBlendKeepsOpaquePixels(t *testing.T) {
	coverages := []uint32{0, 1, 0x101, 0x7fff, 0x8000, 0xfffe, 0xffff}
	for alpha := 1; alpha <= 255; alpha++ {
		for _, coverage := range coverages {
			canvas := newUniformRGBA(1, 1, Color{9, 200, 77, 255})
			drawLines(canvas, Color{250, 3, 90, alpha}, []Scanline{{Y: 0, X1: 0, X2: 0, Alpha: coverage}})
			if canvas.Pix[3] != 255 {
				t.Fatalf("alpha %d, coverage %#x: result alpha %d", alpha, coverage, canvas.Pix[3])
			}
		}
	}
}

// TestCropScanlines pins clipping to the image rectangle, including lines that
// lie fully outside and the reuse of the input slice.
func TestCropScanlines(t *testing.T) {
	const opaque = 0xffff
	input := []Scanline{
		{Y: 0, X1: 1, X2: 2, Alpha: opaque},
		{Y: -1, X1: 0, X2: 1, Alpha: opaque},
		{Y: 4, X1: 0, X2: 1, Alpha: opaque},
		{Y: 1, X1: 5, X2: 9, Alpha: opaque},
		{Y: 1, X1: -9, X2: -1, Alpha: opaque},
		{Y: 2, X1: -3, X2: 9, Alpha: 7},
		{Y: 3, X1: 3, X2: 2, Alpha: opaque},
		{Y: 3, X1: 4, X2: 4, Alpha: opaque},
	}
	want := []Scanline{
		{Y: 0, X1: 1, X2: 2, Alpha: opaque},
		{Y: 2, X1: 0, X2: 4, Alpha: 7},
		{Y: 3, X1: 4, X2: 4, Alpha: opaque},
	}
	got := cropScanlines(input, 5, 4)
	if len(got) != len(want) {
		t.Fatalf("got %d lines %v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}
