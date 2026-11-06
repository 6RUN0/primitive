package primitive

import (
	"cmp"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"io"
	"slices"
)

const (
	// gifPaletteSize is the limit of the GIF format.
	gifPaletteSize = 256
	// gifMaxDelay is the limit of the 16-bit delay field; image/gif truncates a
	// larger value without an error.
	gifMaxDelay = 65535
	// gifMinOpaqueAlpha is the lowest alpha a GIF shows as an opaque color.
	gifMinOpaqueAlpha = 128
)

// colorCount is one distinct opaque color of an image and its pixel count.
type colorCount struct {
	channels [3]uint8
	pixels   int
}

// colorBox is one cell of the median cut: colors sorted along splitChannel and
// the squared error of replacing them with their mean.
type colorBox struct {
	colors       []colorCount
	mean         [3]float64
	splitChannel int
	squaredError float64
}

// newColorBox measures colors and sorts them along the channel of the widest
// spread. The packed value breaks ties, because the sort is not stable and the
// palette has to repeat for one image.
func newColorBox(colors []colorCount) colorBox {
	box := colorBox{colors: colors}
	var pixels float64
	for _, entry := range colors {
		pixels += float64(entry.pixels)
		for channel, value := range entry.channels {
			box.mean[channel] += float64(entry.pixels) * float64(value)
		}
	}
	for channel := range box.mean {
		box.mean[channel] /= pixels
	}
	var spread [3]float64
	for _, entry := range colors {
		for channel, value := range entry.channels {
			distance := float64(value) - box.mean[channel]
			spread[channel] += float64(entry.pixels) * distance * distance
		}
	}
	for channel, value := range spread {
		box.squaredError += value
		if value > spread[box.splitChannel] {
			box.splitChannel = channel
		}
	}
	slices.SortFunc(box.colors, func(a, b colorCount) int {
		return cmp.Or(
			cmp.Compare(a.channels[box.splitChannel], b.channels[box.splitChannel]),
			cmp.Compare(packChannels(a.channels), packChannels(b.channels)),
		)
	})
	return box
}

func packChannels(channels [3]uint8) uint32 {
	return uint32(channels[0])<<16 | uint32(channels[1])<<8 | uint32(channels[2])
}

// split divides the box at the median pixel, keeping one color on each side.
func (box colorBox) split() (colorBox, colorBox) {
	total := 0
	for _, entry := range box.colors {
		total += entry.pixels
	}
	seen, cut := 0, 1
	for i, entry := range box.colors[:len(box.colors)-1] {
		seen += entry.pixels
		cut = i + 1
		if seen*2 >= total {
			break
		}
	}
	return newColorBox(box.colors[:cut]), newColorBox(box.colors[cut:])
}

// straightChannels returns the color of a premultiplied pixel of at least
// gifMinOpaqueAlpha: a GIF shows it opaque, so the premultiplied channels would
// darken it. Most pictures are opaque, and only that case avoids the conversion.
func straightChannels(pixel []uint8) [3]uint8 {
	if pixel[3] == 255 {
		return [3]uint8(pixel[:3])
	}
	straight := color.NRGBAModel.Convert(color.RGBA{pixel[0], pixel[1], pixel[2], pixel[3]}).(color.NRGBA)
	return [3]uint8{straight.R, straight.G, straight.B}
}

// countColors returns the distinct colors of the pixels a GIF shows as opaque,
// sorted, because the sums of a colorBox must not depend on map order.
func countColors(im *image.RGBA) []colorCount {
	counts := map[uint32]int{}
	for i := 0; i+4 <= len(im.Pix); i += 4 {
		if im.Pix[i+3] >= gifMinOpaqueAlpha {
			counts[packChannels(straightChannels(im.Pix[i:i+4]))]++
		}
	}
	colors := make([]colorCount, 0, len(counts))
	for packed, pixels := range counts {
		channels := [3]uint8{uint8(packed >> 16), uint8(packed >> 8), uint8(packed)}
		colors = append(colors, colorCount{channels, pixels})
	}
	slices.SortFunc(colors, func(a, b colorCount) int {
		return cmp.Compare(packChannels(a.channels), packChannels(b.channels))
	})
	return colors
}

// medianCutPalette returns at most size opaque colors for im. The box with the
// largest squared error splits first, so colors go where they lower the error
// most, not to the most populated range.
func medianCutPalette(im *image.RGBA, size int) color.Palette {
	colors := countColors(im)
	if len(colors) == 0 {
		return color.Palette{color.RGBA{A: 255}}
	}
	boxes := []colorBox{newColorBox(colors)}
	for len(boxes) < size {
		worst := 0
		for i, box := range boxes {
			if box.squaredError > boxes[worst].squaredError {
				worst = i
			}
		}
		if len(boxes[worst].colors) < 2 {
			break
		}
		first, second := boxes[worst].split()
		boxes[worst] = first
		boxes = append(boxes, second)
	}
	palette := make(color.Palette, 0, len(boxes))
	for _, box := range boxes {
		palette = append(palette, color.RGBA{
			R: uint8(box.mean[0] + 0.5),
			G: uint8(box.mean[1] + 0.5),
			B: uint8(box.mean[2] + 0.5),
			A: 255,
		})
	}
	return palette
}

// gifQuantizer maps pixels to palette entries. Palette.Index scans the whole
// palette, the cache of seen pixels saves that for the many repeated ones.
type gifQuantizer struct {
	palette     color.Palette
	transparent uint8
	indexes     map[uint32]uint8
}

// indexOf returns the transparent index for a pixel below gifMinOpaqueAlpha and the
// nearest opaque entry otherwise. Choosing among all entries would make the
// threshold depend on the palette, and a growing alpha could then leave the
// transparent index and return to it, which changedRegion cannot show.
func (quantizer *gifQuantizer) indexOf(pixel []uint8) uint8 {
	if pixel[3] < gifMinOpaqueAlpha {
		return quantizer.transparent
	}
	opaque := quantizer.palette[:quantizer.transparent]
	channels := straightChannels(pixel)
	return uint8(opaque.Index(color.RGBA{R: channels[0], G: channels[1], B: channels[2], A: 255}))
}

// quantize converts frame to the nearest colors. Floyd-Steinberg dithering gave 6.8
// times the file and 2.4 times the RMSE on examples/lenna.png with -n 100: flat
// shapes need no error diffusion, and diffused noise defeats the frame differences.
func (quantizer *gifQuantizer) quantize(frame *image.RGBA) *image.Paletted {
	quantized := image.NewPaletted(frame.Rect, quantizer.palette)
	var previous uint32
	var previousIndex uint8
	for i := range quantized.Pix {
		pixel := frame.Pix[i*4 : i*4+4 : i*4+4]
		key := uint32(pixel[0])<<24 | uint32(pixel[1])<<16 | uint32(pixel[2])<<8 | uint32(pixel[3])
		if i == 0 || key != previous {
			index, isCached := quantizer.indexes[key]
			if !isCached {
				index = quantizer.indexOf(pixel)
				quantizer.indexes[key] = index
			}
			previous, previousIndex = key, index
		}
		quantized.Pix[i] = previousIndex
	}
	return quantized
}

// changedRegion returns the bounding box of the pixels that differ between two
// frames of one size, as transparent pixels over the frame shown before. A frame
// without changes gives one transparent pixel: GIF has no empty frame.
func changedRegion(shown, next *image.Paletted, transparent uint8) *image.Paletted {
	var box image.Rectangle
	width := next.Rect.Dx()
	for i, index := range next.Pix {
		if index != shown.Pix[i] {
			x, y := i%width, i/width
			box = box.Union(image.Rect(x, y, x+1, y+1))
		}
	}
	if box.Empty() {
		box = image.Rect(0, 0, 1, 1)
	}
	region := image.NewPaletted(box, next.Palette)
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			index := next.ColorIndexAt(x, y)
			if index == shown.ColorIndexAt(x, y) {
				index = transparent
			}
			region.SetColorIndex(x, y, index)
		}
	}
	return region
}

// encodeGIF writes frames as a looping GIF with one palette from the last frame,
// each frame after the first holding only the changed pixels. delay and lastDelay
// are in hundredths of a second, lastDelay belongs to the last frame.
//
// Frames must have one size and delays must be from 0 to gifMaxDelay, otherwise it
// returns an error. No pixel may turn transparent after it was shown, which holds
// for Model.Frames: shapes only add coverage.
func encodeGIF(w io.Writer, frames []image.Image, delay, lastDelay int) error {
	if len(frames) == 0 {
		return errors.New("a GIF needs at least one frame")
	}
	if min(delay, lastDelay) < 0 || max(delay, lastDelay) > gifMaxDelay {
		return fmt.Errorf("GIF delays %d and %d are outside 0-%d", delay, lastDelay, gifMaxDelay)
	}
	last := imageToRGBA(frames[len(frames)-1])
	for i, frame := range frames {
		if got, want := frame.Bounds().Size(), last.Rect.Size(); got != want {
			return fmt.Errorf("GIF frame %d has size %v, want %v", i, got, want)
		}
	}
	// The last entry marks unchanged pixels and a transparent background; the
	// encoder takes the first entry with zero alpha as the transparent one.
	palette := append(medianCutPalette(last, gifPaletteSize-1), color.RGBA{})
	transparent := uint8(len(palette) - 1)
	quantizer := gifQuantizer{palette, transparent, map[uint32]uint8{}}
	animation := gif.GIF{
		Config: image.Config{ColorModel: palette, Width: last.Rect.Dx(), Height: last.Rect.Dy()},
	}
	var shown *image.Paletted
	for i, frame := range frames {
		quantized := quantizer.quantize(imageToRGBA(frame))
		if shown == nil {
			animation.Image = append(animation.Image, quantized)
		} else {
			animation.Image = append(animation.Image, changedRegion(shown, quantized, transparent))
		}
		shown = quantized
		animation.Disposal = append(animation.Disposal, gif.DisposalNone)
		if i == len(frames)-1 {
			animation.Delay = append(animation.Delay, lastDelay)
		} else {
			animation.Delay = append(animation.Delay, delay)
		}
	}
	return gif.EncodeAll(w, &animation)
}
