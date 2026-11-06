package main

import (
	"flag"
	"image"
	"image/color"
	"runtime/debug"
	"strings"
	"testing"
)

// TestResolveVersion pins the order of the sources: go install passes no ldflags,
// so the module version is the only one its binary has, and a linked value wins.
func TestResolveVersion(t *testing.T) {
	recorded := func(version string) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: version}}
	}
	cases := []struct {
		name    string
		linked  string
		info    *debug.BuildInfo
		hasInfo bool
		want    string
	}{
		{"linked", "1.2.3", recorded("v9.9.9"), true, "1.2.3"},
		{"go install", "devel", recorded("v1.4.0"), true, "v1.4.0"},
		{"no build info", "devel", nil, false, "devel"},
		{"test binary", "devel", recorded(""), true, "devel"},
		{"no module version", "devel", recorded("(devel)"), true, "devel"},
	}
	for _, testCase := range cases {
		if got := resolveVersion(testCase.linked, testCase.info, testCase.hasInfo); got != testCase.want {
			t.Errorf("%s: got %q, want %q", testCase.name, got, testCase.want)
		}
	}
}

// TestUsageRepeatsNoDefault pins the flag texts: the flag package prints a non-zero
// default itself, so a text with one reads "(default 100) (default 100)".
func TestUsageRepeatsNoDefault(t *testing.T) {
	flag.VisitAll(func(entry *flag.Flag) {
		if entry.DefValue != "0" && strings.Contains(entry.Usage, "(default "+entry.DefValue+")") {
			t.Errorf("-%s: %q", entry.Name, entry.Usage)
		}
	})
}

// TestThumbnailKeepsFittingImage pins the "never upscale" guarantee: an image
// inside the limit, or with no pixels at all, is passed through untouched.
func TestThumbnailKeepsFittingImage(t *testing.T) {
	cases := []struct {
		name   string
		bounds image.Rectangle
	}{
		{"smaller", image.Rect(0, 0, 100, 50)},
		{"exact limit", image.Rect(0, 0, 256, 256)},
		{"zero width", image.Rect(0, 0, 0, 1000)},
		{"zero height", image.Rect(0, 0, 1000, 0)},
		{"zero both", image.Rect(0, 0, 0, 0)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			source := image.NewRGBA(testCase.bounds)
			if got := makeThumbnail(source, 256); got != image.Image(source) {
				t.Errorf("got a new %v image, want the source returned", got.Bounds())
			}
		})
	}
}

// TestThumbnailDownscaledSize pins the output size, including the clamp to one
// pixel: the integer aspect ratio of an elongated image rounds to zero.
func TestThumbnailDownscaledSize(t *testing.T) {
	cases := []struct {
		name          string
		bounds        image.Rectangle
		maxSize       int
		width, height int
	}{
		{"landscape", image.Rect(0, 0, 512, 256), 256, 256, 128},
		{"portrait", image.Rect(0, 0, 256, 512), 256, 128, 256},
		{"square", image.Rect(0, 0, 512, 512), 256, 256, 256},
		{"portrait within width limit", image.Rect(0, 0, 100, 1000), 256, 25, 256},
		{"one side over the limit", image.Rect(0, 0, 257, 10), 256, 256, 9},
		{"elongated landscape", image.Rect(0, 0, 10000, 1), 256, 256, 1},
		{"elongated portrait", image.Rect(0, 0, 1, 10000), 256, 1, 256},
		{"portrait over the limit on both sides", image.Rect(0, 0, 257, 258), 256, 255, 256},
		{"non-zero origin", image.Rect(100, 200, 612, 456), 256, 256, 128},
		{"limit of one", image.Rect(0, 0, 3, 2), 1, 1, 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := makeThumbnail(image.NewRGBA(testCase.bounds), testCase.maxSize).Bounds()
			want := image.Rect(0, 0, testCase.width, testCase.height)
			if got != want {
				t.Errorf("got bounds %v, want %v", got, want)
			}
		})
	}
}

// TestThumbnailSamplesSourceBounds pins that the sampled area follows the
// source bounds: a non-zero origin must not shift the picture out of the frame.
func TestThumbnailSamplesSourceBounds(t *testing.T) {
	fill := color.RGBA{R: 200, G: 100, B: 50, A: 255}
	source := image.NewRGBA(image.Rect(100, 200, 612, 456))
	for y := source.Bounds().Min.Y; y < source.Bounds().Max.Y; y++ {
		for x := source.Bounds().Min.X; x < source.Bounds().Max.X; x++ {
			source.SetRGBA(x, y, fill)
		}
	}

	thumbnail := makeThumbnail(source, 256)

	for _, point := range []image.Point{{0, 0}, {128, 64}, {255, 127}} {
		if got := thumbnail.At(point.X, point.Y); got != color.Color(fill) {
			t.Errorf("pixel %v: got %v, want %v", point, got, fill)
		}
	}
}

// TestThumbnailAveragesSparseDetail pins the choice of draw.BiLinear: an
// interpolator that samples only a few source pixels loses or inflates isolated
// detail. One white pixel per 8x8 cell averages to 255/64, about 4.
func TestThumbnailAveragesSparseDetail(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 800, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 800; x++ {
			shade := uint8(0)
			if x%8 == 3 && y%8 == 3 {
				shade = 255
			}
			source.SetRGBA(x, y, color.RGBA{R: shade, G: shade, B: shade, A: 255})
		}
	}

	thumbnail := makeThumbnail(source, 100)

	red, _, _, _ := thumbnail.At(50, 50).RGBA()
	if shade := red >> 8; shade < 2 || shade > 6 {
		t.Errorf("center pixel: got shade %d, want 2-6", shade)
	}
}

// TestThumbnailCoversWholeSource pins that the scaled area spans every source
// column on an image with a non-zero origin: a cropped source rectangle keeps
// the edge pixels away from the ends of the gradient.
func TestThumbnailCoversWholeSource(t *testing.T) {
	bounds := image.Rect(100, 200, 1124, 712)
	source := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			shade := uint8((x - bounds.Min.X) * 255 / (bounds.Dx() - 1))
			source.SetRGBA(x, y, color.RGBA{R: shade, G: shade, B: shade, A: 255})
		}
	}

	thumbnail := makeThumbnail(source, 256)

	left, _, _, _ := thumbnail.At(0, 64).RGBA()
	right, _, _, _ := thumbnail.At(255, 64).RGBA()
	if left>>8 > 5 {
		t.Errorf("left edge: got shade %d, want at most 5", left>>8)
	}
	if right>>8 < 250 {
		t.Errorf("right edge: got shade %d, want at least 250", right>>8)
	}
}

// TestThumbnailPremultipliesTransparency pins alpha handling: an NRGBA source
// is scaled in premultiplied RGBA, so half-transparent white becomes shade 128
// with alpha 128 instead of keeping full-intensity color channels.
func TestThumbnailPremultipliesTransparency(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			source.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 128})
		}
	}

	thumbnail := makeThumbnail(source, 256)

	got := color.RGBAModel.Convert(thumbnail.At(128, 128)).(color.RGBA)
	want := color.RGBA{R: 128, G: 128, B: 128, A: 128}
	if got != want {
		t.Errorf("center pixel: got %v, want %v", got, want)
	}
}
