package primitive

import (
	"bytes"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	_ "golang.org/x/image/webp"
)

// maxFuzzPixels keeps one decoded input small: the targets look for wrong
// results on odd headers, not for the cost of large images.
const maxFuzzPixels = 1 << 16

var (
	hexColorPattern = regexp.MustCompile(`^#?([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	// nonFiniteNumber matches what the fmt verbs print for a NaN or an infinity.
	nonFiniteNumber = regexp.MustCompile(`NaN|Inf`)
)

// FuzzMakeHexColor checks that a value is accepted exactly when it has one of
// the documented forms, and that each channel equals the digits of the value.
func FuzzMakeHexColor(f *testing.F) {
	for _, seed := range []string{"", "#", "fff", "#ffff", "102030", "#10203040", "0X00", "0x0000", "##fff", "+123"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		got, err := MakeHexColor(value)
		if isValid := hexColorPattern.MatchString(value); isValid != (err == nil) {
			t.Fatalf("%q: pattern match %v, error %v", value, isValid, err)
		}
		if err != nil {
			return
		}
		if want := parseHexChannels(strings.TrimPrefix(value, "#")); got != want {
			t.Fatalf("%q: got %+v, want %+v", value, got, want)
		}
	})
}

// parseHexChannels reads channels straight from valid digits, one digit per
// channel in the short forms, and is independent of the bit shifts of MakeHexColor.
func parseHexChannels(digits string) Color {
	width := 2
	if len(digits) < 6 {
		width = 1
	}
	channels := [4]int{255, 255, 255, 255}
	for i := 0; (i+1)*width <= len(digits); i++ {
		value, err := strconv.ParseUint(digits[i*width:(i+1)*width], 16, 8)
		if err != nil {
			panic(err)
		}
		channels[i] = int(value)
		if width == 1 {
			channels[i] *= 0x11
		}
	}
	return Color{channels[0], channels[1], channels[2], channels[3]}
}

func encodeFuzzImage(f *testing.F, encode func(io.Writer) error) []byte {
	f.Helper()
	var encoded bytes.Buffer
	if err := encode(&encoded); err != nil {
		f.Fatal(err)
	}
	return encoded.Bytes()
}

// FuzzDecodeAverage feeds arbitrary files through the decoders the CLI accepts.
// AverageImageColor has to return an opaque color inside the range of the image
// channels for any bounds a decoder can produce, including empty and offset ones,
// and NewModel with one shape has to accept every image with pixels.
func FuzzDecodeAverage(f *testing.F) {
	picture := buildNoisyTarget(4, 4)
	offsetFrame := image.NewPaletted(image.Rect(2, 3, 6, 7), color.Palette{color.Black, color.White})
	offsetFrame.SetColorIndex(3, 4, 1)
	f.Add(encodeFuzzImage(f, func(w io.Writer) error { return png.Encode(w, picture) }))
	f.Add(encodeFuzzImage(f, func(w io.Writer) error { return jpeg.Encode(w, picture, nil) }))
	f.Add(encodeFuzzImage(f, func(w io.Writer) error { return gif.Encode(w, picture, nil) }))
	f.Add(encodeFuzzImage(f, func(w io.Writer) error {
		return gif.EncodeAll(w, &gif.GIF{
			Image: []*image.Paletted{offsetFrame},
			Delay: []int{0},
			Config: image.Config{
				ColorModel: offsetFrame.Palette,
				Width:      8,
				Height:     8,
			},
		})
	}))
	// 4x4 lossy WebP from cwebp: x/image has no WebP encoder.
	lossyWebp, err := hex.DecodeString("524946464e0000005745425056503820420000003003009d012a0400040002c04c25a802746b80fe018401fa01efff76000c0d66e15000fef2e960293ff2d52ea2bff42e654bfffc833f552f25dc42f781d108000000")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(lossyWebp)
	// 4x4 lossless WebP with a translucent pixel from cwebp -lossless.
	losslessWebp, err := hex.DecodeString("5249464630000000574542505650384c230000002f03c000001730800282a2e99e131014f93fdafc077706080428631b0b1344f43fc60700")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(losslessWebp)
	f.Fuzz(func(t *testing.T, contents []byte) {
		config, _, err := image.DecodeConfig(bytes.NewReader(contents))
		if err != nil || config.Width*config.Height > maxFuzzPixels {
			return
		}
		decoded, _, err := image.Decode(bytes.NewReader(contents))
		if err != nil {
			return
		}
		average := AverageImageColor(decoded)
		if average.A != 255 {
			t.Fatalf("got alpha %d", average.A)
		}
		rgba := imageToRGBA(decoded)
		if got := rgba.Bounds().Min; got != (image.Point{}) {
			t.Fatalf("origin %v", got)
		}
		if decoded.Bounds().Empty() {
			return
		}
		lowest := [3]uint8{255, 255, 255}
		highest := [3]uint8{}
		for i := 0; i < len(rgba.Pix); i += 4 {
			for channel := 0; channel < 3; channel++ {
				lowest[channel] = min(lowest[channel], rgba.Pix[i+channel])
				highest[channel] = max(highest[channel], rgba.Pix[i+channel])
			}
		}
		for channel, got := range []uint8{average.R, average.G, average.B} {
			if got < lowest[channel] || got > highest[channel] {
				t.Fatalf("channel %d: mean %d outside %d-%d", channel, got, lowest[channel], highest[channel])
			}
		}
		model := NewModel(decoded, MakeColor(average), 16, 1, 1)
		worker := model.Workers[0]
		worker.Init(model.Current, model.Score)
		state := worker.RandomState(ShapeTypeTriangle, 128, 0.5)
		model.Add(state.Shape, state.Alpha)
		if len(model.Shapes) != 1 || !(model.Score >= 0 && model.Score <= 1) {
			t.Fatalf("%d shapes, score %f", len(model.Shapes), model.Score)
		}
	})
}

// FuzzCropScanlines compares cropScanlines with a direct intersection of each
// line and the image rectangle.
func FuzzCropScanlines(f *testing.F) {
	f.Add(uint8(5), uint8(4), []byte{0, 1, 2, 0, 255, 0, 1, 0, 2, 253, 9, 7})
	f.Add(uint8(0), uint8(0), []byte{0, 0, 0, 0})
	f.Add(uint8(1), uint8(1), []byte{0, 255, 1, 0, 1, 0, 0, 0})
	f.Fuzz(func(t *testing.T, width, height uint8, encodedLines []byte) {
		const bytesPerLine = 4
		var input, want []Scanline
		for i := 0; i+bytesPerLine <= len(encodedLines); i += bytesPerLine {
			line := Scanline{
				Y:     int(int8(encodedLines[i])),
				X1:    int(int8(encodedLines[i+1])),
				X2:    int(int8(encodedLines[i+2])),
				Alpha: uint32(encodedLines[i+3]),
			}
			input = append(input, line)
			first, last := max(line.X1, 0), min(line.X2, int(width)-1)
			if line.Y >= 0 && line.Y < int(height) && first <= last {
				want = append(want, Scanline{line.Y, first, last, line.Alpha})
			}
		}
		got := cropScanlines(input, int(width), int(height))
		if len(got) != len(want) {
			t.Fatalf("got %d lines %v, want %d %v", len(got), got, len(want), want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("line %d: got %+v, want %+v", i, got[i], want[i])
			}
		}
	})
}

// FuzzSVG checks that Model.SVG is well-formed XML with a viewBox of the target
// size, one element per shape and a background that keeps its alpha, and that every
// number has the canonical form, for any shape mix, including an empty model.
// Shapes are added directly: Step would spend the budget on the search.
func FuzzSVG(f *testing.F) {
	f.Add(int64(1), uint8(8), uint8(8), uint8(0), uint8(1), uint8(128), uint8(0))
	f.Add(int64(2), uint8(1), uint8(1), uint8(40), uint8(0), uint8(0), uint8(0))
	f.Add(int64(3), uint8(30), uint8(2), uint8(9), uint8(6), uint8(255), uint8(200))
	f.Fuzz(func(t *testing.T, seed int64, width, height, shapeCount, mode, alpha, backgroundAlpha uint8) {
		targetWidth, targetHeight := int(width%32)+1, int(height%32)+1
		target := buildNoisyTarget(targetWidth, targetHeight)
		model := NewModel(target, Color{10, 20, 30, int(backgroundAlpha)}, 64, 1, seed)
		worker := model.Workers[0]
		wantShapes := int(shapeCount % 50)
		shapeType := ShapeType(mode % 9)
		for i := 0; i < wantShapes; i++ {
			worker.Init(model.Current, model.Score)
			state := worker.RandomState(shapeType, int(alpha), 4)
			state.Shape.Mutate()
			model.Add(state.Shape, state.Alpha)
		}

		decoder := xml.NewDecoder(bytes.NewReader([]byte(model.SVG())))
		depth, gotShapes := 0, 0
		isInClip := false
		hasBackgroundOpacity := false
		viewBox := ""
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("invalid XML: %v\n%s", err, model.SVG())
			}
			switch element := token.(type) {
			case xml.StartElement:
				depth++
				for _, attribute := range element.Attr {
					if nonFiniteNumber.MatchString(attribute.Value) {
						t.Fatalf("%s=%q in <%s>", attribute.Name.Local, attribute.Value, element.Name.Local)
					}
					if !isInClip {
						checkSVGAttribute(t, attribute)
					}
				}
				switch element.Name.Local {
				case "svg":
					for _, attribute := range element.Attr {
						if attribute.Name.Local == "viewBox" {
							viewBox = attribute.Value
						}
					}
				case "clipPath":
					isInClip = true
				case "rect", "ellipse", "path":
					if isInClip {
						break
					}
					gotShapes++
					if gotShapes == 1 {
						for _, attribute := range element.Attr {
							hasBackgroundOpacity = hasBackgroundOpacity || attribute.Name.Local == "fill-opacity"
						}
					}
				}
			case xml.EndElement:
				depth--
				if element.Name.Local == "clipPath" {
					isInClip = false
				}
			}
		}
		if depth != 0 {
			t.Fatalf("unbalanced elements: depth %d", depth)
		}
		if want := fmt.Sprintf("0 0 %d %d", targetWidth, targetHeight); viewBox != want {
			t.Fatalf("viewBox %q, want %q", viewBox, want)
		}
		// One extra rect is the background.
		if gotShapes != wantShapes+1 {
			t.Fatalf("got %d elements, want %d shapes and the background", gotShapes, wantShapes)
		}
		if isOpaqueBackground := backgroundAlpha == 255; hasBackgroundOpacity == isOpaqueBackground {
			t.Fatalf("background alpha %d, fill-opacity present: %v", backgroundAlpha, hasBackgroundOpacity)
		}
	})
}

// checkSVGPathCommands fails unless data is M with one point and then a closed l
// with whole points or an open q with two, which also shows two merged numbers. It
// returns the numbers.
func checkSVGPathCommands(t *testing.T, data string) []string {
	t.Helper()
	match := svgPathCommands.FindStringSubmatch(data)
	if match == nil {
		t.Fatalf("d=%q is not a path of relative commands", data)
	}
	start := svgNumberPattern.FindAllString(match[1], -1)
	lines := svgNumberPattern.FindAllString(match[2], -1)
	curve := svgNumberPattern.FindAllString(match[3], -1)
	isCurve := match[3] != ""
	if len(start) != 2 || (isCurve && len(curve) != 4) || (!isCurve && (len(lines) == 0 || len(lines)%2 != 0)) {
		t.Fatalf("d=%q has %d, %d and %d numbers in M, l and q", data, len(start), len(lines), len(curve))
	}
	return append(append(start, lines...), curve...)
}

// checkSVGAttribute fails on a number that is not in the canonical form of
// formatSVGNumber. A path is read with the same pattern a parser would apply.
func checkSVGAttribute(t *testing.T, attribute xml.Attr) {
	t.Helper()
	name, value := attribute.Name.Local, attribute.Value
	var numbers []string
	pattern := svgCanonicalValue
	switch name {
	case "x", "y", "cx", "cy", "rx", "ry", "stroke-width":
		numbers = []string{value}
	case "width", "height":
		if value != "100%" {
			numbers = []string{value}
		}
	case "fill-opacity", "stroke-opacity":
		numbers = []string{value}
		pattern = svgOpacityValue
	case "d":
		numbers = checkSVGPathCommands(t, value)
	case "transform":
		if value == "translate(.5 .5)" {
			return
		}
		match := svgTransform.FindStringSubmatch(value)
		if match == nil {
			t.Fatalf("transform=%q", value)
		}
		numbers = []string{match[1], match[2], match[3]}
		if match[4] != "" {
			numbers = append(numbers, match[5], match[6])
		}
	}
	for _, number := range numbers {
		if !pattern.MatchString(number) {
			t.Fatalf("%s=%q has the number %q", name, value, number)
		}
	}
}
