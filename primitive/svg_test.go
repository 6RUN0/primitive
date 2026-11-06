package primitive

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"math"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var (
	svgNumberPattern  = regexp.MustCompile(`-?(\d+\.\d+|\.\d+|\d+)`)
	svgCanonicalValue = regexp.MustCompile(`^(0|-?[1-9]\d*(\.\d?[1-9])?|-?\.\d?[1-9])$`)
	svgOpacityValue   = regexp.MustCompile(`^(0|1|\.\d{0,2}[1-9])$`)
	svgPathCommands   = regexp.MustCompile(`^M([-.\d ]+)(?:l([-.\d ]+)z|q([-.\d ]+))$`)
	svgTransform      = regexp.MustCompile(`^translate\((\S+) (\S+)\) rotate\((\S+)\)( scale\((\S+) (\S+)\))?$`)
)

// Limits of TestSVGRenderMatchesPNG, RMSE from 0 to 1. computeRMSE averages 4 channels
// with alpha at zero, the 3 channels of magick compare in the README read sqrt(4/3)
// times higher. Measured on rsvg-convert 2.62.3:
// at most 0.0040 at scale 4 and 0.0044 at scale 64, both on curves. Without translate
// all 18 cases fail, without stroke-opacity both curve cases, at 0.070 and 0.090.
const (
	svgRenderLimitNormal = 0.006
	svgRenderLimitLarge  = 0.007
)

// parseSVGPathNumbers returns the numbers of path data in order, with the commands
// dropped. It reads the shortest form, where "1.5.5-2" is three numbers.
func parseSVGPathNumbers(t *testing.T, data string) []float64 {
	t.Helper()
	var numbers []float64
	for _, text := range svgNumberPattern.FindAllString(data, -1) {
		number, err := strconv.ParseFloat(text, 64)
		if err != nil {
			t.Fatal(err)
		}
		numbers = append(numbers, number)
	}
	return numbers
}

// TestFormatSVGNumber pins the output form of every number: the characters a
// parser does not need are gone, and a value rounded to zero has no sign.
func TestFormatSVGNumber(t *testing.T) {
	cases := []struct {
		value    float64
		decimals int
		want     string
	}{
		{0, 2, "0"},
		{1, 2, "1"},
		{10, 2, "10"},
		{100, 2, "100"},
		{0.5, 2, ".5"},
		{-0.5, 2, "-.5"},
		{0.05, 2, ".05"},
		{1.5, 2, "1.5"},
		{1.25, 2, "1.25"},
		{1.999, 2, "2"},
		{-1.999, 2, "-2"},
		{0.004, 2, "0"},
		{-0.004, 2, "0"},
		{math.Copysign(0, -1), 2, "0"},
		{128.0 / 255, 3, ".502"},
		{1.0 / 255, 3, ".004"},
		{1e6, 2, "1000000"},
		{math.NaN(), 2, "0"},
		{math.Inf(1), 2, "0"},
		{math.Inf(-1), 2, "0"},
	}
	for _, testCase := range cases {
		if got := formatSVGNumber(testCase.value, testCase.decimals); got != testCase.want {
			t.Errorf("formatSVGNumber(%v, %d) = %q, want %q", testCase.value, testCase.decimals, got, testCase.want)
		}
	}
}

// TestRoundSVGCoordinateRejectsNonFinite pins the path side of the rule that NaN
// stays out of the document: converting NaN to an integer is undefined in Go.
func TestRoundSVGCoordinateRejectsNonFinite(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := roundSVGCoordinate(value); got != 0 {
			t.Errorf("roundSVGCoordinate(%v) = %d, want 0", value, got)
		}
	}
}

// TestJoinSVGPathNumbers pins where a separator can go: a number glued to a digit
// merges with it, and ".5" after "5" would read as 5.5.
func TestJoinSVGPathNumbers(t *testing.T) {
	cases := []struct {
		values []int64
		want   string
	}{
		{[]int64{5900, 14300}, "59 143"},
		{[]int64{-700, -10100, -4200, 8000}, "-7-101-42 80"},
		{[]int64{150, 50, 50}, "1.5.5.5"},
		{[]int64{500, 50}, "5 .5"},
		{[]int64{150, 500, 50}, "1.5 5 .5"},
		{[]int64{150, -50}, "1.5-.5"},
		{[]int64{0, 0}, "0 0"},
	}
	for _, testCase := range cases {
		if got := joinSVGPathNumbers(testCase.values...); got != testCase.want {
			t.Errorf("joinSVGPathNumbers(%v) = %q, want %q", testCase.values, got, testCase.want)
		}
	}
}

// TestClosedSVGPathKeepsRoundingError pins deltas between rounded points. Rounding
// a delta of exact points instead shifts the vertices of this path by 0.0033 each,
// so the last one ends 0.33 away.
func TestClosedSVGPathKeepsRoundingError(t *testing.T) {
	const vertices = 100
	xs := make([]float64, vertices)
	ys := make([]float64, vertices)
	for i := range xs {
		xs[i] = 3.3333 * float64(i)
		ys[i] = -1.6667 * float64(i)
	}
	numbers := parseSVGPathNumbers(t, closedSVGPath(xs, ys))
	if len(numbers) != 2*vertices {
		t.Fatalf("got %d numbers, want %d", len(numbers), 2*vertices)
	}
	x, y := numbers[0], numbers[1]
	for i := 1; i < vertices; i++ {
		x += numbers[2*i]
		y += numbers[2*i+1]
		if math.Abs(x-xs[i]) > 0.00501 || math.Abs(y-ys[i]) > 0.00501 {
			t.Fatalf("vertex %d at (%.4f, %.4f), want (%.4f, %.4f)", i, x, y, xs[i], ys[i])
		}
	}
}

// TestShapeSVGForms pins the element of each shape: a transform on the element
// instead of a wrapper group, and paths with relative coordinates, where both
// control points of q count from the start of the curve.
func TestShapeSVGForms(t *testing.T) {
	cases := []struct {
		name  string
		shape Shape
		want  string
	}{
		{"triangle", &Triangle{nil, 59, 143, 52, 42, 10, 122}, `<path a="b" d="M59 143l-7-101-42 80z"/>`},
		{"polygon", &Polygon{nil, 3, false, []float64{-1.5, 2.25, 3}, []float64{0.1, 0.6, 0.1}}, `<path a="b" d="M-1.5.1l3.75.5.75-.5z"/>`},
		{"curve", &Quadratic{nil, 10, 20, 15, 18, 30, 21.5, 1, 1}, `<path a="b" d="M10 20q5-2 20 1.5"/>`},
		{"rotated ellipse", &RotatedEllipse{nil, 1.5, 2, 3.25, 4, -30}, `<ellipse a="b" rx="3.25" ry="4" transform="translate(1.5 2) rotate(330)"/>`},
		{"rotated ellipse past a turn", &RotatedEllipse{nil, 1, 2, 3, 4, 725.5}, `<ellipse a="b" rx="3" ry="4" transform="translate(1 2) rotate(5.5)"/>`},
		{"rotated rectangle", &RotatedRectangle{nil, 1, 2, 3, 4, -30}, `<rect a="b" x="-.5" y="-.5" width="1" height="1" transform="translate(1 2) rotate(330) scale(3 4)"/>`},
		{"rotated rectangle past a turn", &RotatedRectangle{nil, 1, 2, 3, 4, 725}, `<rect a="b" x="-.5" y="-.5" width="1" height="1" transform="translate(1 2) rotate(5) scale(3 4)"/>`},
		{"ellipse", &Ellipse{nil, 1, 2, 3, 4, false}, `<ellipse a="b" cx="1" cy="2" rx="3" ry="4"/>`},
	}
	for _, testCase := range cases {
		if got := testCase.shape.SVG(`a="b"`); got != testCase.want {
			t.Errorf("%s: got %s, want %s", testCase.name, got, testCase.want)
		}
	}
	if got, want := (&Ellipse{nil, 1, 2, 3, 4, false}).SVG(""), `<ellipse cx="1" cy="2" rx="3" ry="4"/>`; got != want {
		t.Errorf("no attributes: got %s, want %s", got, want)
	}
}

// buildSVGModel returns a model with shapes and colors set directly, so that a test
// names the group attributes it expects.
func buildSVGModel(background Color, shapes []Shape, colors []Color) *Model {
	model := NewModel(buildNoisyTarget(8, 4), background, 16, 1, 1)
	model.Shapes = shapes
	model.Colors = colors
	return model
}

// TestModelSVGSharesAttributesOnGroup pins what the group carries. The two defects
// behind stroke-opacity and translate: curves ignore fill-opacity, so a translucent
// curve came out opaque, and the half pixel shift of the raster was missing.
func TestModelSVGSharesAttributesOnGroup(t *testing.T) {
	triangle := func() Shape { return &Triangle{nil, 0, 0, 4, 0, 0, 4} }
	curve := func(width float64) Shape { return &Quadratic{nil, 0, 0, 2, 2, 4, 0, width, 1} }
	const prefix = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 8 4"><rect width="100%" height="100%" fill="#010203"/>` +
		`<clipPath id="c8x4"><path d="M-.5-.5h8v4H-.5z"/></clipPath><g clip-path="url(#c8x4)" transform="translate(.5 .5)"`
	const trianglePath = `d="M0 0l4 0-4 4z"/>`
	const curvePath = `d="M0 0q2 2 4 0"/>`
	red, green := Color{255, 0, 0, 128}, Color{0, 255, 0, 200}
	cases := []struct {
		name   string
		shapes []Shape
		colors []Color
		want   string
	}{
		{"no shapes", nil, nil, `></g></svg>`},
		{"one alpha", []Shape{triangle(), triangle()}, []Color{red, green},
			` fill-opacity=".502"><path fill="#ff0000" ` + trianglePath + `<path fill="#00ff00" fill-opacity=".784" ` + trianglePath + `</g></svg>`},
		{"common alpha wins", []Shape{triangle(), triangle(), triangle()}, []Color{green, red, green},
			` fill-opacity=".784"><path fill="#00ff00" ` + trianglePath + `<path fill="#ff0000" fill-opacity=".502" ` + trianglePath + `<path fill="#00ff00" ` + trianglePath + `</g></svg>`},
		{"opaque needs no group opacity", []Shape{triangle()}, []Color{{1, 2, 3, 255}},
			`><path fill="#010203" ` + trianglePath + `</g></svg>`},
		{"curves share everything", []Shape{curve(0.5), curve(0.504)}, []Color{red, red},
			` stroke-opacity=".502" fill="none" stroke-linecap="round" stroke-width=".5"><path stroke="#ff0000" ` + curvePath + `<path stroke="#ff0000" ` + curvePath + `</g></svg>`},
		{"curve widths differ", []Shape{curve(0.5), curve(2)}, []Color{red, red},
			` stroke-opacity=".502" fill="none" stroke-linecap="round"><path stroke="#ff0000" stroke-width=".5" ` + curvePath + `<path stroke="#ff0000" stroke-width="2" ` + curvePath + `</g></svg>`},
		{"mixed shapes", []Shape{curve(1), triangle()}, []Color{green, red},
			` fill-opacity=".502" stroke-opacity=".502" stroke-linecap="round" stroke-width="1"><path fill="none" stroke="#00ff00" stroke-opacity=".784" ` + curvePath + `<path fill="#ff0000" ` + trianglePath + `</g></svg>`},
		{"curve in a mix", []Shape{triangle(), curve(1), curve(2)}, []Color{red, red, red},
			` fill-opacity=".502" stroke-opacity=".502" stroke-linecap="round"><path fill="#ff0000" ` + trianglePath + `<path fill="none" stroke="#ff0000" stroke-width="1" ` + curvePath + `<path fill="none" stroke="#ff0000" stroke-width="2" ` + curvePath + `</g></svg>`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := buildSVGModel(Color{1, 2, 3, 255}, testCase.shapes, testCase.colors).SVG()
			if want := prefix + testCase.want; got != want {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

// TestModelSVGKeepsBackgroundAlpha pins -bg with alpha: the raster is translucent,
// and SVG lost the alpha.
func TestModelSVGKeepsBackgroundAlpha(t *testing.T) {
	for alpha, want := range map[int]string{0: ` fill-opacity="0"`, 128: ` fill-opacity=".502"`} {
		svg := buildSVGModel(Color{1, 2, 3, alpha}, nil, nil).SVG()
		if !strings.Contains(svg, `fill="#010203"`+want+`/>`) {
			t.Errorf("alpha %d: no %s:\n%s", alpha, want, svg)
		}
	}
}

// renderSVG rasterizes svg with rsvg-convert, the renderer the deviations in the
// README were measured with.
func renderSVG(t *testing.T, rsvg, svg string, width, height int) *image.RGBA {
	t.Helper()
	output := filepath.Join(t.TempDir(), "render.png")
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, rsvg, "-w", strconv.Itoa(width), "-h", strconv.Itoa(height), "-o", output)
	command.Stdin = bytes.NewReader([]byte(svg))
	if combined, err := command.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, combined)
	}
	rendered, err := LoadImage(output)
	if err != nil {
		t.Fatal(err)
	}
	return imageToRGBA(rendered)
}

func computeRMSE(a, b *image.RGBA) float64 {
	var sum float64
	for i := range a.Pix {
		delta := float64(a.Pix[i]) - float64(b.Pix[i])
		sum += delta * delta
	}
	return math.Sqrt(sum/float64(len(a.Pix))) / 255
}

// TestSVGRenderMatchesPNG pins the SVG output to the raster of one model in every
// mode: a half pixel shift, opaque curves or a rounding too coarse for the output
// scale lift the error well above the limit. It runs on demand: CI has no renderer.
func TestSVGRenderMatchesPNG(t *testing.T) {
	rsvg, err := exec.LookPath("rsvg-convert")
	if err != nil {
		t.Skip("rsvg-convert is not installed; the test renders SVG with it")
	}
	cases := []struct {
		targetSize, outputSize int
		maxRMSE                float64
	}{
		{64, 256, svgRenderLimitNormal},
		{16, 1024, svgRenderLimitLarge},
	}
	for _, testCase := range cases {
		target := buildNoisyTarget(testCase.targetSize, testCase.targetSize)
		for mode := ShapeTypeAny; mode <= ShapeTypePolygon; mode++ {
			t.Run(fmt.Sprintf("size%d_scale%d_mode%d", testCase.targetSize, testCase.outputSize/testCase.targetSize, mode), func(t *testing.T) {
				model := NewModel(target, MakeColor(AverageImageColor(target)), testCase.outputSize, 1, 11)
				for step := 0; step < 30; step++ {
					model.Step(mode, 128, 0, 0.5)
				}
				rendered := renderSVG(t, rsvg, model.SVG(), model.Sw, model.Sh)
				rmse := computeRMSE(imageToRGBA(model.Context.Image()), rendered)
				t.Logf("rmse %.5f", rmse)
				if rmse > testCase.maxRMSE {
					t.Errorf("rmse %.5f, limit %.5f", rmse, testCase.maxRMSE)
				}
			})
		}
	}
}

// TestSVGClipStopsAtFrame pins the clip against the group translate: shapes reach
// past the frame by 16 pixels, and a viewer with a wider view must not show them.
// A clip in untranslated coordinates would leave half a pixel on the top and left.
func TestSVGClipStopsAtFrame(t *testing.T) {
	rsvg, err := exec.LookPath("rsvg-convert")
	if err != nil {
		t.Skip("rsvg-convert is not installed; the test renders SVG with it")
	}
	const pixelsPerUnit, marginUnits, frameWidth, frameHeight = 10, 8, 8, 4
	shape := &Triangle{nil, -16, -16, 40, -16, -16, 40}
	svg := buildSVGModel(Color{0, 0, 255, 255}, []Shape{shape}, []Color{{255, 0, 0, 255}}).SVG()
	widened := strings.Replace(svg, `viewBox="0 0 8 4"`, `viewBox="-8 -8 24 20"`, 1)
	width, height := 24*pixelsPerUnit, 20*pixelsPerUnit
	rendered := renderSVG(t, rsvg, widened, width, height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			frameX, frameY := x-marginUnits*pixelsPerUnit, y-marginUnits*pixelsPerUnit
			isInFrame := frameX >= 0 && frameX < frameWidth*pixelsPerUnit && frameY >= 0 && frameY < frameHeight*pixelsPerUnit
			if isRed := rendered.RGBAAt(x, y).R == 255; isRed != isInFrame {
				t.Fatalf("pixel (%d, %d): red %v, in the frame %v", x, y, isRed, isInFrame)
			}
		}
	}
}
