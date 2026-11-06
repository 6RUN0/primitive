package primitive

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// buildBlockFrames returns frames where each one adds a colored block, so that all
// colors of the earlier frames survive into the last one.
func buildBlockFrames(width, height int) []image.Image {
	blocks := []struct {
		area image.Rectangle
		fill color.RGBA
	}{
		{image.Rect(2, 2, 6, 6), color.RGBA{R: 200, A: 255}},
		{image.Rect(8, 3, 12, 9), color.RGBA{G: 180, A: 255}},
		{image.Rect(1, 9, 5, 11), color.RGBA{B: 220, A: 255}},
	}
	canvas := uniformRGBA(image.Rect(0, 0, width, height), color.RGBA{R: 10, G: 20, B: 30, A: 255})
	frames := []image.Image{copyRGBA(canvas)}
	for _, block := range blocks {
		draw.Draw(canvas, block.area, &image.Uniform{block.fill}, image.Point{}, draw.Src)
		frames = append(frames, copyRGBA(canvas))
	}
	return frames
}

func encodeTestGIF(t *testing.T, frames []image.Image, delay, lastDelay int) *gif.GIF {
	t.Helper()
	var encoded bytes.Buffer
	if err := encodeGIF(&encoded, frames, delay, lastDelay); err != nil {
		t.Fatal(err)
	}
	decoded, err := gif.DecodeAll(&encoded)
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

// compositeGIFFrames plays the animation the way a viewer does when each frame
// stays on screen: the result of every step, not the stored regions.
func compositeGIFFrames(animation *gif.GIF) []*image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, animation.Config.Width, animation.Config.Height))
	var shown []*image.RGBA
	for _, frame := range animation.Image {
		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		shown = append(shown, copyRGBA(canvas))
	}
	return shown
}

// TestSaveGIFWritesPlayableFile pins the file format contract: a standard decoder
// reads it, with one frame per input, an endless loop, the delay on every frame
// and the longer pause on the last one.
func TestSaveGIFWritesPlayableFile(t *testing.T) {
	model := NewModel(buildNoisyTarget(16, 16), Color{0, 0, 0, 255}, 32, 1, 3)
	for i := 0; i < 5; i++ {
		model.Step(ShapeTypeTriangle, 128, 0, 0.5)
	}
	frames := model.Frames(0)
	path := filepath.Join(t.TempDir(), "output.gif")
	if err := SaveGIF(path, frames, 50, 250); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	animation, err := gif.DecodeAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 6 || len(animation.Image) != len(frames) {
		t.Fatalf("got %d frames from %d inputs, want 6", len(animation.Image), len(frames))
	}
	if animation.LoopCount != 0 {
		t.Errorf("loop count %d, want 0 for an endless loop", animation.LoopCount)
	}
	if animation.Config.Width != 32 || animation.Config.Height != 32 {
		t.Errorf("logical screen %dx%d, want 32x32", animation.Config.Width, animation.Config.Height)
	}
	for i, delay := range animation.Delay {
		want := 50
		if i == len(animation.Delay)-1 {
			want = 250
		}
		if delay != want {
			t.Errorf("frame %d: delay %d, want %d", i, delay, want)
		}
	}
}

// TestGIFFramesRepeatInput pins the frame differences: playing the stored regions
// has to give each input frame, which fails when a region loses a pixel or a
// disposal method clears the frame before.
func TestGIFFramesRepeatInput(t *testing.T) {
	frames := buildBlockFrames(16, 12)
	animation := encodeTestGIF(t, frames, 10, 20)
	for i, shown := range compositeGIFFrames(animation) {
		if want := frames[i].(*image.RGBA); !bytes.Equal(shown.Pix, want.Pix) {
			t.Errorf("frame %d differs from the input", i)
		}
	}
	for i, disposal := range animation.Disposal {
		if disposal != gif.DisposalNone {
			t.Errorf("frame %d: disposal %d, want the frame to stay", i, disposal)
		}
	}
}

// TestGIFStoresOnlyChangedRegions pins the size gain: a region of the full frame
// means LZW compresses unchanged pixels of every frame again.
func TestGIFStoresOnlyChangedRegions(t *testing.T) {
	animation := encodeTestGIF(t, buildBlockFrames(16, 12), 10, 20)
	wants := []image.Rectangle{
		image.Rect(0, 0, 16, 12),
		image.Rect(2, 2, 6, 6),
		image.Rect(8, 3, 12, 9),
		image.Rect(1, 9, 5, 11),
	}
	for i, want := range wants {
		if got := animation.Image[i].Bounds(); got != want {
			t.Errorf("frame %d: bounds %v, want %v", i, got, want)
		}
	}
}

// TestGIFKeepsFrameWithoutChanges pins equal frames, such as a shape too faint to
// change an index: GIF has no empty image, and a zero-sized one fails to encode.
func TestGIFKeepsFrameWithoutChanges(t *testing.T) {
	frame := uniformRGBA(image.Rect(0, 0, 8, 8), color.RGBA{R: 40, G: 50, B: 60, A: 255})
	animation := encodeTestGIF(t, []image.Image{frame, frame}, 10, 20)
	if got := animation.Image[1].Bounds(); got.Dx() != 1 || got.Dy() != 1 {
		t.Fatalf("second frame bounds %v, want one pixel", got)
	}
	shown := compositeGIFFrames(animation)
	if !bytes.Equal(shown[1].Pix, frame.Pix) {
		t.Error("the frame without changes altered the picture")
	}
}

// TestGIFKeepsTransparentBackground pins -bg with zero alpha: without a transparent
// palette entry the background turns into the nearest opaque color.
func TestGIFKeepsTransparentBackground(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 8, 8))
	first := copyRGBA(canvas)
	draw.Draw(canvas, image.Rect(2, 2, 6, 6), &image.Uniform{color.RGBA{R: 255, A: 255}}, image.Point{}, draw.Src)
	animation := encodeTestGIF(t, []image.Image{first, canvas}, 10, 20)
	shown := compositeGIFFrames(animation)
	if got := shown[1].RGBAAt(0, 0); got.A != 0 {
		t.Errorf("background pixel %v, want transparent", got)
	}
	if got := shown[1].RGBAAt(3, 3); got != (color.RGBA{R: 255, A: 255}) {
		t.Errorf("shape pixel %v", got)
	}
}

// TestGIFRepeatsForOneInput pins a fixed palette order: the color map iterates in
// random order, and -seed promises equal output.
func TestGIFRepeatsForOneInput(t *testing.T) {
	frames := []image.Image{buildNoisyTarget(64, 64)}
	var first, second bytes.Buffer
	if err := encodeGIF(&first, frames, 10, 20); err != nil {
		t.Fatal(err)
	}
	if err := encodeGIF(&second, frames, 10, 20); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Error("two encodings of one frame differ")
	}
}

// TestEncodeGIFRejectsNoFrames pins the error: the palette comes from the last
// frame, and an empty list would panic on that index.
func TestEncodeGIFRejectsNoFrames(t *testing.T) {
	var encoded bytes.Buffer
	err := encodeGIF(&encoded, nil, 10, 20)
	if err == nil || !strings.Contains(err.Error(), "at least one frame") {
		t.Errorf("got error %v", err)
	}
}

// TestSaveGIFReportsCreateError pins that a failed write reaches the caller, which
// turns it into exit code 1.
func TestSaveGIFReportsCreateError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent", "output.gif")
	if err := SaveGIF(path, buildBlockFrames(16, 12), 10, 20); err == nil {
		t.Error("got no error for a path in a missing directory")
	}
}

// TestMedianCutKeepsFewColors pins that a picture within the limit keeps its exact
// colors: a mean over a box with one color must not round them away.
func TestMedianCutKeepsFewColors(t *testing.T) {
	last := imageToRGBA(buildBlockFrames(16, 12)[3])
	palette := medianCutPalette(last, 8)
	if len(palette) != 4 {
		t.Fatalf("got %d entries, want 4", len(palette))
	}
	for i := 0; i < len(last.Pix); i += 4 {
		want := last.RGBAAt(i/4%16, i/4/16)
		if got := palette.Convert(want); got != color.Color(want) {
			t.Fatalf("pixel %d: %v becomes %v", i/4, want, got)
		}
	}
}

// TestMedianCutRespectsLimit pins the size limit, and spends entries where the
// error is: a uniform box of mid gray gets one entry, a gradient the others.
func TestMedianCutRespectsLimit(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 512, 2))
	for x := 0; x < 256; x++ {
		picture.SetRGBA(x, 0, color.RGBA{R: uint8(x), G: uint8(x), B: uint8(x), A: 255})
		picture.SetRGBA(x, 1, color.RGBA{R: uint8(x), G: uint8(x), B: uint8(x), A: 255})
	}
	for x := 256; x < 512; x++ {
		picture.SetRGBA(x, 0, color.RGBA{R: 128, G: 128, B: 128, A: 255})
		picture.SetRGBA(x, 1, color.RGBA{R: 128, G: 128, B: 128, A: 255})
	}
	palette := medianCutPalette(picture, 16)
	if len(palette) != 16 {
		t.Fatalf("got %d entries, want 16", len(palette))
	}
	worst := 0
	for x := 0; x < 256; x++ {
		original := picture.RGBAAt(x, 0)
		nearest := palette.Convert(original).(color.RGBA)
		worst = max(worst, max(int(original.R)-int(nearest.R), int(nearest.R)-int(original.R)))
	}
	if worst > 32 {
		t.Errorf("worst channel error %d, want at most 32 for 16 entries over 256 levels", worst)
	}
}

// TestMedianCutSkipsTransparentPixels pins that transparent pixels spend no palette
// entries: they are stored as the transparent index.
func TestMedianCutSkipsTransparentPixels(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 4, 1))
	picture.SetRGBA(0, 0, color.RGBA{R: 200, A: 255})
	picture.SetRGBA(1, 0, color.RGBA{R: 50, A: 50})
	palette := medianCutPalette(picture, 8)
	if len(palette) != 1 || palette[0] != color.Color(color.RGBA{R: 200, A: 255}) {
		t.Errorf("got %v", palette)
	}
}

// TestMedianCutFullyTransparent pins the empty histogram: the GIF palette must
// not be empty, and a mean over no pixels divides by zero.
func TestMedianCutFullyTransparent(t *testing.T) {
	palette := medianCutPalette(image.NewRGBA(image.Rect(0, 0, 4, 4)), 8)
	if len(palette) != 1 {
		t.Errorf("got %d entries, want 1", len(palette))
	}
}

// TestGIFAlphaThresholdIsFixed pins the documented limit of half opacity. Nearest
// color over all entries, the transparent one included, moves the limit with the
// palette: alpha 100 next to an opaque color of equal brightness turns opaque.
func TestGIFAlphaThresholdIsFixed(t *testing.T) {
	cases := []struct {
		alpha         uint8
		isTransparent bool
	}{{1, true}, {100, true}, {127, true}, {128, false}, {255, false}}
	for _, testCase := range cases {
		frame := image.NewRGBA(image.Rect(0, 0, 4, 4))
		for i := 0; i < len(frame.Pix); i += 4 {
			frame.Pix[i], frame.Pix[i+1], frame.Pix[i+2], frame.Pix[i+3] = 100, 100, 100, testCase.alpha
		}
		frame.SetRGBA(0, 0, color.RGBA{R: 100, G: 100, B: 100, A: 255})
		shown := compositeGIFFrames(encodeTestGIF(t, []image.Image{frame}, 10, 20))[0]
		if got := shown.RGBAAt(1, 1).A == 0; got != testCase.isTransparent {
			t.Errorf("alpha %d: pixel %v, want transparent %v", testCase.alpha, shown.RGBAAt(1, 1), testCase.isTransparent)
		}
	}
}

// TestGIFShowsPixelThatGrowsOpaque pins the monotone index: a pixel crossing the
// threshold moves from the transparent index to a color, and the frame difference
// has to store it.
func TestGIFShowsPixelThatGrowsOpaque(t *testing.T) {
	faint := uniformRGBA(image.Rect(0, 0, 4, 4), color.RGBA{R: 90, A: 120})
	strong := uniformRGBA(image.Rect(0, 0, 4, 4), color.RGBA{R: 90, A: 130})
	shown := compositeGIFFrames(encodeTestGIF(t, []image.Image{faint, strong}, 10, 20))
	if got := shown[0].RGBAAt(1, 1); got.A != 0 {
		t.Errorf("first frame pixel %v, want transparent", got)
	}
	if got := shown[1].RGBAAt(1, 1); got.A != 255 {
		t.Errorf("second frame pixel %v, want opaque", got)
	}
}

// TestEncodeGIFRejectsMixedSizes pins the error for public SaveGIF callers: a larger
// frame after a smaller one panics in changedRegion with index out of range.
func TestEncodeGIFRejectsMixedSizes(t *testing.T) {
	frames := []image.Image{image.NewRGBA(image.Rect(0, 0, 4, 4)), image.NewRGBA(image.Rect(0, 0, 8, 8))}
	var encoded bytes.Buffer
	err := encodeGIF(&encoded, frames, 10, 20)
	if err == nil || !strings.Contains(err.Error(), "has size") {
		t.Errorf("got error %v", err)
	}
}

// TestEncodeGIFRejectsDelayOutOfRange pins the 16-bit field: image/gif truncates
// 70000 to 4464 without an error.
func TestEncodeGIFRejectsDelayOutOfRange(t *testing.T) {
	frames := buildBlockFrames(16, 12)
	for _, delays := range [][2]int{{65536, 20}, {10, 65536}, {-1, 20}, {10, -1}} {
		var encoded bytes.Buffer
		err := encodeGIF(&encoded, frames, delays[0], delays[1])
		if err == nil || !strings.Contains(err.Error(), "outside") {
			t.Errorf("delays %v: got error %v", delays, err)
		}
	}
	if decoded := encodeTestGIF(t, frames, 65535, 65535); decoded.Delay[0] != 65535 {
		t.Errorf("delay %d, want 65535", decoded.Delay[0])
	}
}

// TestSaveGIFKeepsExistingFileOnEncodeError pins the encode step before os.Create:
// a run that fails at the end must not leave the previous result empty.
func TestSaveGIFKeepsExistingFileOnEncodeError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "output.gif")
	if err := os.WriteFile(path, []byte("previous"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveGIF(path, nil, 10, 20); err == nil {
		t.Fatal("got no error")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "previous" {
		t.Errorf("file holds %q", contents)
	}
}

// TestFramesEndWithFinishedPicture pins the tail of the shapes: a frame came only
// from a gain of scoreDelta, so the shapes after the last one never reached the
// animation.
func TestFramesEndWithFinishedPicture(t *testing.T) {
	target := buildNoisyTarget(16, 16)
	model := NewModel(target, MakeColor(AverageImageColor(target)), 32, 1, 3)
	if got := len(model.Frames(0)); got != 1 {
		t.Fatalf("model without shapes: got %d frames, want the background", got)
	}
	for step := 0; step < 6; step++ {
		model.Step(ShapeTypeTriangle, 128, 0, 0.5)
	}
	frames := model.Frames(100)
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want the background and the finished picture", len(frames))
	}
	last := imageToRGBA(frames[len(frames)-1])
	if !bytes.Equal(last.Pix, imageToRGBA(model.Context.Image()).Pix) {
		t.Error("last frame differs from the finished picture")
	}
}

// TestGIFShowsStraightColors pins translucent pixels: RGBA pixels are premultiplied
// and a GIF shows them opaque, so half opaque red came out dark red, even with an
// exact dark red in the palette next to the true color.
func TestGIFShowsStraightColors(t *testing.T) {
	frame := uniformRGBA(image.Rect(0, 0, 4, 4), color.NRGBA{R: 255, A: 128})
	draw.Draw(frame, image.Rect(0, 0, 2, 4), &image.Uniform{color.RGBA{R: 128, A: 255}}, image.Point{}, draw.Src)
	shown := compositeGIFFrames(encodeTestGIF(t, []image.Image{frame}, 10, 10))[0]
	if got := shown.RGBAAt(3, 0); got != (color.RGBA{R: 255, A: 255}) {
		t.Errorf("translucent red shows as %v", got)
	}
	if got := shown.RGBAAt(0, 0); got != (color.RGBA{R: 128, A: 255}) {
		t.Errorf("opaque dark red shows as %v", got)
	}
}

// TestEncodeGIFAcceptsZeroDelay pins the lower end of the delay range: 0 is a
// valid delay, and a check that rejects it refuses GIFs the format allows.
func TestEncodeGIFAcceptsZeroDelay(t *testing.T) {
	decoded := encodeTestGIF(t, buildBlockFrames(16, 12), 0, 0)
	if decoded.Delay[0] != 0 {
		t.Errorf("delay %d, want 0", decoded.Delay[0])
	}
}

// TestCountColorsIncludesLastPixel pins the end of the pixel loop: a color that
// only the last pixel has was missing from the palette and drawn as a neighbor.
func TestCountColorsIncludesLastPixel(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 2, 1))
	picture.SetRGBA(0, 0, color.RGBA{R: 10, A: 255})
	picture.SetRGBA(1, 0, color.RGBA{R: 20, A: 255})
	want := []colorCount{{[3]uint8{10, 0, 0}, 1}, {[3]uint8{20, 0, 0}, 1}}
	if got := countColors(picture); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestColorBoxSplitsAtMedianPixel pins the cut with an even number of equal
// colors: each side gets half of the pixels, not three quarters on one side.
func TestColorBoxSplitsAtMedianPixel(t *testing.T) {
	colors := []colorCount{
		{[3]uint8{10, 10, 10}, 1}, {[3]uint8{20, 20, 20}, 1},
		{[3]uint8{30, 30, 30}, 1}, {[3]uint8{40, 40, 40}, 1},
	}
	first, second := newColorBox(colors).split()
	if len(first.colors) != 2 || len(second.colors) != 2 {
		t.Errorf("got %d and %d colors, want 2 and 2", len(first.colors), len(second.colors))
	}
}

// TestColorBoxSplitsAlongWidestChannel pins that the widest spread picks the sort
// channel: colors that vary in blue only must be cut by blue, not by the packed
// value, which orders by red first.
func TestColorBoxSplitsAlongWidestChannel(t *testing.T) {
	low, middle, high := colorCount{[3]uint8{9, 0, 0}, 1}, colorCount{[3]uint8{5, 0, 50}, 1}, colorCount{[3]uint8{0, 0, 100}, 1}
	first, second := newColorBox([]colorCount{low, middle, high}).split()
	if !slices.Equal(first.colors, []colorCount{low, middle}) || !slices.Equal(second.colors, []colorCount{high}) {
		t.Errorf("got %v and %v, want the two lowest blues first", first.colors, second.colors)
	}
}

// TestColorBoxTiedSpreadsSplitAlongFirstChannel pins that equal spreads resolve to
// the lowest channel: the palette order, and so the bytes of the GIF for a given
// seed, must not depend on which equal channel wins.
func TestColorBoxTiedSpreadsSplitAlongFirstChannel(t *testing.T) {
	greenish, reddish := colorCount{[3]uint8{0, 10, 0}, 1}, colorCount{[3]uint8{10, 0, 0}, 1}
	first, _ := newColorBox([]colorCount{reddish, greenish}).split()
	if !slices.Equal(first.colors, []colorCount{greenish}) {
		t.Errorf("first box %v, want the lowest red", first.colors)
	}
}

// TestMedianCutSplitsFirstOfEqualErrors pins the choice between boxes of equal
// squared error: the earlier one splits, so that the palette of a picture with
// symmetric halves is the same on every run and every version.
func TestMedianCutSplitsFirstOfEqualErrors(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 4, 1))
	for x, level := range []uint8{0, 10, 100, 110} {
		picture.SetRGBA(x, 0, color.RGBA{R: level, G: level, B: level, A: 255})
	}
	want := color.Palette{color.RGBA{A: 255}, color.RGBA{R: 105, G: 105, B: 105, A: 255}, color.RGBA{R: 10, G: 10, B: 10, A: 255}}
	if got := medianCutPalette(picture, 3); !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestFramesKeepShapeThatGainsExactlyDelta pins the threshold as inclusive: the
// shape that lowers the score by exactly scoreDelta is a frame, as the doc says.
func TestFramesKeepShapeThatGainsExactlyDelta(t *testing.T) {
	target := buildNoisyTarget(16, 16)
	model := NewModel(target, MakeColor(AverageImageColor(target)), 32, 1, 3)
	for step := 0; step < 3; step++ {
		model.Step(ShapeTypeTriangle, 128, 0, 0.5)
	}
	model.Scores = []float64{9, 8, 7}
	if got := len(model.Frames(1)); got != 4 {
		t.Errorf("got %d frames, want the background and 3 shapes", got)
	}
}
