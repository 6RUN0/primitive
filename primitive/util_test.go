package primitive

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// TestClamp pins both limits and the inclusive ends for the float and int forms.
func TestClamp(t *testing.T) {
	cases := []struct{ value, want int }{{-1, 0}, {0, 0}, {5, 5}, {10, 10}, {11, 10}}
	for _, testCase := range cases {
		if got := clampInt(testCase.value, 0, 10); got != testCase.want {
			t.Errorf("clampInt(%d): got %d, want %d", testCase.value, got, testCase.want)
		}
		if got := clamp(float64(testCase.value), 0, 10); got != float64(testCase.want) {
			t.Errorf("clamp(%d): got %v, want %d", testCase.value, got, testCase.want)
		}
	}
}

// TestNumberString pins the suffix steps in the verbose log, including the
// largest suffix, which has no upper limit.
func TestNumberString(t *testing.T) {
	cases := []struct {
		value float64
		want  string
	}{
		{0, "0.0"},
		{999.9, "999.9"},
		{1000, "1.0k"},
		{1500000, "1.5M"},
		{2e9, "2.0G"},
		{3e12, "3.0T"},
		{4e15, "4000.0T"},
	}
	for _, testCase := range cases {
		if got := NumberString(testCase.value); got != testCase.want {
			t.Errorf("NumberString(%v): got %q, want %q", testCase.value, got, testCase.want)
		}
	}
}

// TestSaveAndLoadRepeatPixels pins the file helpers against the lossless formats
// the CLI writes, and that a missing path is reported, not panicked on.
func TestSaveAndLoadRepeatPixels(t *testing.T) {
	picture := buildNoisyTarget(8, 8)
	path := filepath.Join(t.TempDir(), "picture.png")
	if err := SavePNG(path, picture); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := color.NRGBAModel.Convert(loaded.At(3, 5)), picture.At(3, 5); got != want {
		t.Errorf("pixel: got %v, want %v", got, want)
	}
	if _, err := LoadImage(filepath.Join(t.TempDir(), "absent.png")); !os.IsNotExist(err) {
		t.Errorf("missing file: got error %v", err)
	}
}

// TestSaveFileWritesContents pins the SVG path of the CLI.
func TestSaveFileWritesContents(t *testing.T) {
	path := filepath.Join(t.TempDir(), "picture.svg")
	if err := SaveFile(path, "<svg/>"); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "<svg/>" {
		t.Errorf("got %q, error %v", got, err)
	}
}
