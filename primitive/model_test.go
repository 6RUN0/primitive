package primitive

import (
	"reflect"
	"testing"
)

// TestModelOutputSizeKeepsAspect pins the output size and scale of a landscape and
// of a portrait target: the longer side is size, the scale maps target pixels to
// output pixels.
func TestModelOutputSizeKeepsAspect(t *testing.T) {
	cases := []struct {
		name           string
		width, height  int
		wantSw, wantSh int
	}{
		{"landscape", 40, 20, 80, 40},
		{"portrait", 20, 40, 40, 80},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			model := NewModel(buildNoisyTarget(testCase.width, testCase.height), Color{128, 128, 128, 255}, 80, 1, 1)
			if model.Sw != testCase.wantSw || model.Sh != testCase.wantSh || model.Scale != 2 {
				t.Errorf("got %dx%d at scale %v, want %dx%d at scale 2", model.Sw, model.Sh, model.Scale, testCase.wantSw, testCase.wantSh)
			}
		})
	}
}

// TestFramesKeepsShapesThatGainScoreDelta pins the frame filter: a frame is kept
// when the score fell by at least scoreDelta since the last kept frame, and the
// last frame is always kept. The scores are exact in binary, so a fall equal to
// scoreDelta is one.
func TestFramesKeepsShapesThatGainScoreDelta(t *testing.T) {
	model := NewModel(buildNoisyTarget(16, 16), Color{128, 128, 128, 255}, 16, 1, 1)
	for i := 0; i < 5; i++ {
		model.Shapes = append(model.Shapes, &Rectangle{model.Workers[0], i, i, i + 5, i + 5})
		model.Colors = append(model.Colors, Color{200, 50, 50, 255})
	}
	model.Scores = []float64{0.75, 0.625, 0.5, 0.4375, 0.375}
	every := model.Frames(0)
	if len(every) != 6 {
		t.Fatalf("got %d frames at delta 0, want the background and 5 shapes", len(every))
	}
	want := []int{0, 1, 3, 5}
	got := model.Frames(0.25)
	if len(got) != len(want) {
		t.Fatalf("got %d frames, want %d: the background, shapes 1 and 3, and the last", len(got), len(want))
	}
	for i, frame := range want {
		if !reflect.DeepEqual(got[i], every[frame]) {
			t.Errorf("frame %d differs from frame %d of the full run", i, frame)
		}
	}
}
