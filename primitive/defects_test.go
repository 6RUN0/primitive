package primitive

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

// TestRotatedEllipseRadiiStayInsideImage pins the bound of Ry: a limit taken
// from the width lets a wide image grow ellipses far taller than the image.
func TestRotatedEllipseRadiiStayInsideImage(t *testing.T) {
	const width, height = 200, 10
	worker := newTestWorker(width, height, 1)
	ellipse := &RotatedEllipse{Worker: worker, X: 5, Y: 5, Rx: 3, Ry: 3}
	for i := 0; i < 5000; i++ {
		ellipse.Mutate()
		if ellipse.Rx > width-1 {
			t.Fatalf("mutation %d: Rx=%f, limit %d", i, ellipse.Rx, width-1)
		}
		if ellipse.Ry > height-1 {
			t.Fatalf("mutation %d: Ry=%f, limit %d", i, ellipse.Ry, height-1)
		}
	}
}

// TestSVGWithoutShapes pins the empty model: an opacity read from Colors[0]
// panics with index out of range before the first shape.
func TestSVGWithoutShapes(t *testing.T) {
	model := NewModel(buildNoisyTarget(8, 8), Color{1, 2, 3, 255}, 16, 1, 1)
	if svg := model.SVG(); !strings.Contains(svg, `fill="#010203"`) {
		t.Errorf("background rectangle missing:\n%s", svg)
	}
}

// TestAverageImageColorEmptyImage pins the zero-area input: dividing the sum by
// the pixel count panics with integer divide by zero.
func TestAverageImageColorEmptyImage(t *testing.T) {
	got := AverageImageColor(image.NewRGBA(image.Rect(5, 5, 5, 9)))
	if want := (color.NRGBA{A: 255}); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestModelOutputSmallerThanTarget pins -s below the target width: a viewBox
// derived from the truncated scale divides by zero for a scale below 1.
func TestModelOutputSmallerThanTarget(t *testing.T) {
	model := NewModel(buildNoisyTarget(256, 128), Color{0, 0, 0, 255}, 128, 1, 1)
	if model.Vw != 256 || model.Vh != 128 {
		t.Errorf("viewBox size %dx%d, want 256x128", model.Vw, model.Vh)
	}
	if model.Sw != 128 || model.Sh != 64 {
		t.Errorf("output size %dx%d, want 128x64", model.Sw, model.Sh)
	}
}

// TestModelViewBoxMatchesTarget pins the viewBox for a fractional scale: shape
// coordinates are target pixels, and a viewBox derived from the truncated scale
// leaves the picture short of the frame (333 units for 256 pixels at -s 1000).
func TestModelViewBoxMatchesTarget(t *testing.T) {
	model := NewModel(buildNoisyTarget(256, 171), Color{0, 0, 0, 255}, 1000, 1, 1)
	if model.Vw != 256 || model.Vh != 171 {
		t.Errorf("viewBox size %dx%d, want 256x171", model.Vw, model.Vh)
	}
}

// TestModelOutputKeepsOnePixel pins the lower limit of the output size: a very
// elongated target at a small size truncates the short side to zero, and a
// zero-sized image cannot be encoded.
func TestModelOutputKeepsOnePixel(t *testing.T) {
	cases := []struct {
		width, height, wantWidth, wantHeight int
	}{
		{256, 2, 16, 1},
		{2, 256, 1, 16},
	}
	for _, testCase := range cases {
		model := NewModel(buildNoisyTarget(testCase.width, testCase.height), Color{0, 0, 0, 255}, 16, 1, 1)
		if model.Sw != testCase.wantWidth || model.Sh != testCase.wantHeight {
			t.Errorf("target %dx%d: output size %dx%d, want %dx%d", testCase.width, testCase.height,
				model.Sw, model.Sh, testCase.wantWidth, testCase.wantHeight)
		}
	}
}

// TestModelAcceptsNonZeroOrigin pins targets whose bounds do not start at zero,
// such as a GIF frame with an offset: differenceFull indexes pixels from zero
// and panics with index out of range on an image kept at its own origin.
func TestModelAcceptsNonZeroOrigin(t *testing.T) {
	bounds := image.Rect(20, 20, 70, 70)
	target := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			target.SetRGBA(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}

	model := NewModel(target, Color{0, 0, 0, 255}, 100, 2, 1)
	model.Step(ShapeTypeTriangle, 128, 0, 0.5)

	if got := model.Target.Bounds(); got != image.Rect(0, 0, 50, 50) {
		t.Errorf("target bounds %v, want origin at zero", got)
	}
	if got := model.Target.RGBAAt(0, 0); got != target.RGBAAt(20, 20) {
		t.Errorf("first pixel %v, want %v", got, target.RGBAAt(20, 20))
	}
}

// TestAverageImageColorNonZeroOrigin pins the average of an offset image: pixels
// read from zero lie outside the bounds and count as black.
func TestAverageImageColorNonZeroOrigin(t *testing.T) {
	target := uniformRGBA(image.Rect(20, 20, 70, 70), color.NRGBA{R: 200, G: 100, B: 50, A: 255})
	got := AverageImageColor(target)
	if want := (color.NRGBA{R: 200, G: 100, B: 50, A: 255}); got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestMakeHexColorRejectsMalformedValue pins strict parsing: fmt.Sscanf takes
// "0X00" as a prefixed number and gives alpha 4095, and it turns other junk or
// lengths into opaque black without an error.
func TestMakeHexColorRejectsMalformedValue(t *testing.T) {
	for _, value := range []string{
		"", "#", "##fff", "fff#", "0X00", "0x00", "0x", "ggg", "12", "12345", "1234567",
		"123456789", "+fff", "-fff", " fff", "fff ", "1_0", "ff\x00f",
	} {
		if got, err := MakeHexColor(value); err == nil {
			t.Errorf("%q: got %+v, want an error", value, got)
		}
	}
}

// TestMakeHexColorAcceptsAllForms pins the four accepted lengths, the optional
// "#", upper case and the default alpha.
func TestMakeHexColorAcceptsAllForms(t *testing.T) {
	cases := []struct {
		value string
		want  Color
	}{
		{"000", Color{0, 0, 0, 255}},
		{"#fff", Color{255, 255, 255, 255}},
		{"1aF", Color{0x11, 0xaa, 0xff, 255}},
		{"1234", Color{0x11, 0x22, 0x33, 0x44}},
		{"#102030", Color{0x10, 0x20, 0x30, 255}},
		{"AbCdEf", Color{0xab, 0xcd, 0xef, 255}},
		{"10203040", Color{0x10, 0x20, 0x30, 0x40}},
		{"#00000000", Color{0, 0, 0, 0}},
	}
	for _, testCase := range cases {
		got, err := MakeHexColor(testCase.value)
		if err != nil {
			t.Errorf("%q: %v", testCase.value, err)
		} else if got != testCase.want {
			t.Errorf("%q: got %+v, want %+v", testCase.value, got, testCase.want)
		}
	}
}
