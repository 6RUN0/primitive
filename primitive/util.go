package primitive

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
)

// LoadImage decodes the image at path, "-" reads standard input.
func LoadImage(path string) (image.Image, error) {
	if path == "-" {
		im, _, err := image.Decode(os.Stdin)
		return im, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// A read error surfaces through Decode, Close has nothing to add.
	defer func() { _ = file.Close() }()
	im, _, err := image.Decode(file)
	return im, err
}

// saveToFile encodes to memory and then replaces path, so that a failed encoder
// leaves an existing file as it was. A failed write can still leave part of the
// file. The error of Close counts: a full disk or a network mount can report a
// failed write only there.
func saveToFile(path string, encode func(io.Writer) error) (err error) {
	var encoded bytes.Buffer
	if err := encode(&encoded); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	_, err = file.Write(encoded.Bytes())
	return err
}

// SaveFile writes contents to path, "-" writes to standard output.
func SaveFile(path, contents string) error {
	if path == "-" {
		_, err := fmt.Fprint(os.Stdout, contents)
		return err
	}
	return saveToFile(path, func(w io.Writer) error {
		_, err := io.WriteString(w, contents)
		return err
	})
}

// SavePNG writes im to path as a PNG file.
func SavePNG(path string, im image.Image) error {
	return saveToFile(path, func(w io.Writer) error {
		return png.Encode(w, im)
	})
}

// SaveJPG writes im to path as a JPEG file of the given quality, from 1 to 100.
func SaveJPG(path string, im image.Image, quality int) error {
	return saveToFile(path, func(w io.Writer) error {
		return jpeg.Encode(w, im, &jpeg.Options{Quality: quality})
	})
}

// SaveGIF writes frames as a looping animation with a palette taken from the last
// frame. Frames must have one size. delay and lastDelay are in hundredths of a
// second from 0 to 65535, the last frame gets lastDelay.
func SaveGIF(path string, frames []image.Image, delay, lastDelay int) error {
	return saveToFile(path, func(w io.Writer) error {
		return encodeGIF(w, frames, delay, lastDelay)
	})
}

// NumberString formats x with one decimal and a k, M, G or T suffix.
func NumberString(x float64) string {
	suffixes := []string{"", "k", "M", "G"}
	for _, suffix := range suffixes {
		if x < 1000 {
			return fmt.Sprintf("%.1f%s", x, suffix)
		}
		x /= 1000
	}
	return fmt.Sprintf("%.1f%s", x, "T")
}

func radians(degrees float64) float64 {
	return degrees * math.Pi / 180
}

func degrees(radians float64) float64 {
	return radians * 180 / math.Pi
}

func clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func clampInt(x, lo, hi int) int {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func rotate(x, y, theta float64) (rx, ry float64) {
	rx = x*math.Cos(theta) - y*math.Sin(theta)
	ry = x*math.Sin(theta) + y*math.Cos(theta)
	return
}

// imageToRGBA copies src to an image with its origin at zero: the pixel kernels
// use zero-based coordinates and a shared Pix layout.
func imageToRGBA(src image.Image) *image.RGBA {
	bounds := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(dst, dst.Rect, src, bounds.Min, draw.Src)
	return dst
}

func copyRGBA(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}

func uniformRGBA(r image.Rectangle, c color.Color) *image.RGBA {
	im := image.NewRGBA(r)
	draw.Draw(im, im.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	return im
}

// AverageImageColor returns the opaque mean color of im, black for an image
// without pixels.
func AverageImageColor(im image.Image) color.NRGBA {
	rgba := imageToRGBA(im)
	size := rgba.Bounds().Size()
	w, h := size.X, size.Y
	if w*h == 0 {
		return color.NRGBA{A: 255}
	}
	var r, g, b int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := rgba.RGBAAt(x, y)
			r += int(c.R)
			g += int(c.G)
			b += int(c.B)
		}
	}
	r /= w * h
	g /= w * h
	b /= w * h
	return color.NRGBA{uint8(r), uint8(g), uint8(b), 255}
}
