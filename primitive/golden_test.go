package primitive

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite the files in testdata/golden")

// TestGoldenOutput pins the whole search for a fixed seed: SVG text and rendered
// pixels for three shape families, with one and four workers, and the -rep branch
// with per-shape alpha. Pixels are compared after decoding, so a change of the PNG
// encoder does not fail the test.
// Rewrite after an intended change: go test ./primitive -run TestGoldenOutput -update
func TestGoldenOutput(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skipf("golden files come from amd64 without FMA; %s may fuse floating-point operations", runtime.GOARCH)
	}
	if isFusedFloatBuild {
		t.Skip("golden files come from GOAMD64=v1; FMA instructions of GOAMD64=v3 and above can change the search")
	}
	target := buildNoisyTarget(32, 32)
	background := MakeColor(AverageImageColor(target))
	cases := []struct {
		shapeType      ShapeType
		workers        int
		alpha, repeats int
	}{
		{ShapeTypeTriangle, 1, 128, 0},
		{ShapeTypeTriangle, 4, 128, 0},
		{ShapeTypeEllipse, 1, 128, 0},
		{ShapeTypeEllipse, 4, 128, 0},
		{ShapeTypeRotatedEllipse, 1, 128, 0},
		{ShapeTypeRotatedEllipse, 4, 128, 0},
		{ShapeTypeTriangle, 4, 0, 2},
	}
	for _, testCase := range cases {
		name := fmt.Sprintf("mode%d_j%d", testCase.shapeType, testCase.workers)
		if testCase.repeats > 0 {
			name += fmt.Sprintf("_rep%d_alpha%d", testCase.repeats, testCase.alpha)
		}
		t.Run(name, func(t *testing.T) {
			model := NewModel(target, background, 96, testCase.workers, 1000)
			for step := 0; step < 4; step++ {
				model.Step(testCase.shapeType, testCase.alpha, testCase.repeats, 0.5)
			}
			compareGoldenSVG(t, filepath.Join("testdata", "golden", name+".svg"), model.SVG())
			compareGoldenPNG(t, filepath.Join("testdata", "golden", name+".png"), imageToRGBA(model.Context.Image()))
		})
	}
}

func compareGoldenSVG(t *testing.T, path, got string) {
	t.Helper()
	if *updateGolden {
		writeGolden(t, path, []byte(got))
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s differs:\n%s\nwant:\n%s", path, got, want)
	}
}

func compareGoldenPNG(t *testing.T, path string, got *image.RGBA) {
	t.Helper()
	if *updateGolden {
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, got); err != nil {
			t.Fatal(err)
		}
		writeGolden(t, path, encoded.Bytes())
	}
	want, err := LoadImage(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Pix, imageToRGBA(want).Pix) {
		t.Errorf("%s differs from the rendered pixels", path)
	}
}

func writeGolden(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}
