package primitive

import (
	"image"
	"math"
)

// colorAndError returns the color that brings the covered pixels closest to the
// target at the given alpha, and their squared error before the shape, over all
// four channels. One pass over the pixels serves both results.
func colorAndError(target, current *image.RGBA, lines []Scanline, alpha int) (Color, uint64) {
	var rsum, gsum, bsum, count int64
	var squaredError uint64
	a := 0x101 * 255 / alpha
	for _, line := range lines {
		i := target.PixOffset(line.X1, line.Y)
		for x := line.X1; x <= line.X2; x++ {
			tr := int(target.Pix[i])
			tg := int(target.Pix[i+1])
			tb := int(target.Pix[i+2])
			ta := int(target.Pix[i+3])
			cr := int(current.Pix[i])
			cg := int(current.Pix[i+1])
			cb := int(current.Pix[i+2])
			ca := int(current.Pix[i+3])
			i += 4
			rsum += int64((tr-cr)*a + cr*0x101)
			gsum += int64((tg-cg)*a + cg*0x101)
			bsum += int64((tb-cb)*a + cb*0x101)
			dr := tr - cr
			dg := tg - cg
			db := tb - cb
			da := ta - ca
			squaredError += uint64(dr*dr + dg*dg + db*db + da*da)
			count++
		}
	}
	return meanColor(rsum, gsum, bsum, count, alpha), squaredError
}

// meanColor turns the sums of 0x101-scaled channel values over count pixels into
// a color. No pixels give the zero color.
func meanColor(rsum, gsum, bsum, count int64, alpha int) Color {
	if count == 0 {
		return Color{}
	}
	r := clampInt(int(rsum/count)>>8, 0, 255)
	g := clampInt(int(gsum/count)>>8, 0, 255)
	b := clampInt(int(bsum/count)>>8, 0, 255)
	return Color{r, g, b, alpha}
}

func drawLines(im *image.RGBA, c Color, lines []Scanline) {
	const m = 0xffff
	sr, sg, sb, sa := c.NRGBA().RGBA()
	for _, line := range lines {
		ma := line.Alpha
		sra := sr * ma
		sga := sg * ma
		sba := sb * ma
		saa := sa * ma

		a := (m - saa/m) * 0x101
		i := im.PixOffset(line.X1, line.Y)
		for x := line.X1; x <= line.X2; x++ {
			dr := uint32(im.Pix[i+0])
			dg := uint32(im.Pix[i+1])
			db := uint32(im.Pix[i+2])
			da := uint32(im.Pix[i+3])
			im.Pix[i+0] = uint8((dr*a + sra) / m >> 8)
			im.Pix[i+1] = uint8((dg*a + sga) / m >> 8)
			im.Pix[i+2] = uint8((db*a + sba) / m >> 8)
			im.Pix[i+3] = uint8((da*a + saa) / m >> 8)
			i += 4
		}
	}
}

func differenceFull(a, b *image.RGBA) float64 {
	size := a.Bounds().Size()
	w, h := size.X, size.Y
	var total uint64
	for y := 0; y < h; y++ {
		i := a.PixOffset(0, y)
		for x := 0; x < w; x++ {
			ar := int(a.Pix[i])
			ag := int(a.Pix[i+1])
			ab := int(a.Pix[i+2])
			aa := int(a.Pix[i+3])
			br := int(b.Pix[i])
			bg := int(b.Pix[i+1])
			bb := int(b.Pix[i+2])
			ba := int(b.Pix[i+3])
			i += 4
			dr := ar - br
			dg := ag - bg
			db := ab - bb
			da := aa - ba
			total += uint64(dr*dr + dg*dg + db*db + da*da)
		}
	}
	return math.Sqrt(float64(total)/float64(w*h*4)) / 255
}

// errorAfterBlend returns the squared error of the covered pixels with c blended
// over current, over all four channels. It uses the arithmetic of drawLines and
// writes nothing, so workers can share current. It counts every pixel of lines
// once: shapes never repeat a pixel in their scanlines, which
// TestScanlinesDoNotOverlap pins, and a repeated pixel would be blended twice by
// drawLines.
func errorAfterBlend(target, current *image.RGBA, c Color, lines []Scanline) uint64 {
	const m = 0xffff
	sr, sg, sb, sa := c.NRGBA().RGBA()
	var squaredError uint64
	for _, line := range lines {
		ma := line.Alpha
		sra := sr * ma
		sga := sg * ma
		sba := sb * ma
		saa := sa * ma
		a := (m - saa/m) * 0x101
		i := target.PixOffset(line.X1, line.Y)
		for x := line.X1; x <= line.X2; x++ {
			dr := int(target.Pix[i]) - int(uint8((uint32(current.Pix[i])*a+sra)/m>>8))
			dg := int(target.Pix[i+1]) - int(uint8((uint32(current.Pix[i+1])*a+sga)/m>>8))
			db := int(target.Pix[i+2]) - int(uint8((uint32(current.Pix[i+2])*a+sba)/m>>8))
			da := int(target.Pix[i+3]) - int(uint8((uint32(current.Pix[i+3])*a+saa)/m>>8))
			i += 4
			squaredError += uint64(dr*dr + dg*dg + db*db + da*da)
		}
	}
	return squaredError
}

// errorAfterBlendOpaque equals errorAfterBlend when target and current are opaque:
// blending keeps alpha at 255 for any c and coverage, so alpha adds no error and
// the kernel skips it. On a transparent pixel the result is wrong.
func errorAfterBlendOpaque(target, current *image.RGBA, c Color, lines []Scanline) uint64 {
	const m = 0xffff
	sr, sg, sb, sa := c.NRGBA().RGBA()
	var squaredError uint64
	for _, line := range lines {
		ma := line.Alpha
		sra := sr * ma
		sga := sg * ma
		sba := sb * ma
		saa := sa * ma
		a := (m - saa/m) * 0x101
		i := target.PixOffset(line.X1, line.Y)
		for x := line.X1; x <= line.X2; x++ {
			dr := int(target.Pix[i]) - int(uint8((uint32(current.Pix[i])*a+sra)/m>>8))
			dg := int(target.Pix[i+1]) - int(uint8((uint32(current.Pix[i+1])*a+sga)/m>>8))
			db := int(target.Pix[i+2]) - int(uint8((uint32(current.Pix[i+2])*a+sba)/m>>8))
			i += 4
			squaredError += uint64(dr*dr + dg*dg + db*db)
		}
	}
	return squaredError
}

// scoreAfterChange returns score with the squared error of some pixels changed
// from before to after, for a width x height picture. The integer total comes back
// from the float score by rounding: truncation loses one on most values, the loss
// drifts over the steps, and a shape that removes all the error then wraps uint64.
func scoreAfterChange(width, height int, score float64, before, after uint64) float64 {
	scaledScore := score * 255
	total := uint64(math.Round(scaledScore * scaledScore * float64(width*height*4)))
	total = total - before + after
	return math.Sqrt(float64(total)/float64(width*height*4)) / 255
}

// isOpaque reports whether every pixel of im has alpha 255.
func isOpaque(im *image.RGBA) bool {
	for i := 3; i < len(im.Pix); i += 4 {
		if im.Pix[i] != 255 {
			return false
		}
	}
	return true
}
