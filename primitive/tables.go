package primitive

import (
	"image"
	"math"
)

// maxTablePixels bounds pixelTables: 56 bytes x 2^26 pixels = 3.5 GiB. A larger image
// goes through the pixel kernels and the exact search, where a shape costs O(area)
// and the climbs need more steps: at 8192 x 8192 three shapes take 39 s of CPU, and
// at 8193 x 8192 the first one is not done after 165 s. Peak memory at the limit is
// 5.3 GiB. Re-measure with -r 0 -j 1 on images just below and just above the limit.
const maxTablePixels = 1 << 26

// maxTableWidth keeps the sums of squares of a row below 2^32, 255 x 255 per pixel.
const maxTableWidth = (1<<32 - 1) / (255 * 255)

// rowTotal holds the sums over the pixels [0, x) of one row. Colors are RGB only,
// because the color of a shape does not depend on alpha. maxTableWidth keeps every
// sum within uint32.
type rowTotal struct {
	target       [3]uint32
	current      [3]uint32
	squaredError uint64
	// currentSquared and targetCurrent hold the sums of c*c and t*c per channel.
	currentSquared [3]uint32
	targetCurrent  [3]uint32
}

// pixelTables answers sums over a scanline in O(1), so that the part of Energy
// that only reads target and current does not depend on the shape area. Workers
// read it during a search, and Model.Add updates it between searches.
type pixelTables struct {
	width, height int
	totals        []rowTotal
}

// newPixelTables returns tables of target and current, or nil for an image above
// maxTablePixels or maxTableWidth. Both images must start at the origin.
func newPixelTables(target, current *image.RGBA) *pixelTables {
	size := target.Bounds().Size()
	if size.X*size.Y > maxTablePixels || size.X > maxTableWidth {
		return nil
	}
	tables := &pixelTables{size.X, size.Y, make([]rowTotal, (size.X+1)*size.Y)}
	for y := 0; y < size.Y; y++ {
		tables.fillRow(y, 0, target, current)
	}
	return tables
}

// fillRow recomputes the sums of row y from column fromX on. The sums up to
// fromX stay valid.
func (tables *pixelTables) fillRow(y, fromX int, target, current *image.RGBA) {
	stride := tables.width + 1
	row := tables.totals[y*stride : (y+1)*stride]
	total := row[fromX]
	i := target.PixOffset(fromX, y)
	for x := fromX; x < tables.width; x++ {
		tr, tg, tb, ta := int(target.Pix[i]), int(target.Pix[i+1]), int(target.Pix[i+2]), int(target.Pix[i+3])
		cr, cg, cb, ca := int(current.Pix[i]), int(current.Pix[i+1]), int(current.Pix[i+2]), int(current.Pix[i+3])
		i += 4
		total.target[0] += uint32(tr)
		total.target[1] += uint32(tg)
		total.target[2] += uint32(tb)
		total.current[0] += uint32(cr)
		total.current[1] += uint32(cg)
		total.current[2] += uint32(cb)
		dr, dg, db, da := tr-cr, tg-cg, tb-cb, ta-ca
		total.squaredError += uint64(dr*dr + dg*dg + db*db + da*da)
		total.currentSquared[0] += uint32(cr * cr)
		total.currentSquared[1] += uint32(cg * cg)
		total.currentSquared[2] += uint32(cb * cb)
		total.targetCurrent[0] += uint32(tr * cr)
		total.targetCurrent[1] += uint32(tg * cg)
		total.targetCurrent[2] += uint32(tb * cb)
		row[x+1] = total
	}
}

// updateRows brings the tables up to date with current after lines changed it.
// A row holds cumulative sums, so everything right of the first changed column
// moves, and the rows of the other shapes stay as they are.
func (tables *pixelTables) updateRows(target, current *image.RGBA, lines []Scanline) {
	if len(lines) == 0 {
		return
	}
	minX, minY, maxY := lines[0].X1, lines[0].Y, lines[0].Y
	for _, line := range lines {
		minX = min(minX, line.X1)
		minY = min(minY, line.Y)
		maxY = max(maxY, line.Y)
	}
	for y := minY; y <= maxY; y++ {
		tables.fillRow(y, minX, target, current)
	}
}

// colorAndError returns the same result as the function colorAndError in
// O(len(lines)).
func (tables *pixelTables) colorAndError(lines []Scanline, alpha int) (Color, uint64) {
	var rsum, gsum, bsum, count int64
	var squaredError uint64
	a := int64(0x101 * 255 / alpha)
	stride := tables.width + 1
	for _, line := range lines {
		row := tables.totals[line.Y*stride : (line.Y+1)*stride]
		first, last := &row[line.X1], &row[line.X2+1]
		tr := int64(last.target[0] - first.target[0])
		tg := int64(last.target[1] - first.target[1])
		tb := int64(last.target[2] - first.target[2])
		cr := int64(last.current[0] - first.current[0])
		cg := int64(last.current[1] - first.current[1])
		cb := int64(last.current[2] - first.current[2])
		rsum += (tr-cr)*a + cr*0x101
		gsum += (tg-cg)*a + cg*0x101
		bsum += (tb-cb)*a + cb*0x101
		squaredError += last.squaredError - first.squaredError
		count += int64(line.X2 - line.X1 + 1)
	}
	return meanColor(rsum, gsum, bsum, count, alpha), squaredError
}

// estimateEnergy returns the score of current with a shape of the given lines
// added, in O(len(lines)), without reading a pixel. drawLines gives
// new = floor(k*c + q) for constant k and q on a line, and the squared error of
// that expands to the sums in rowTotal except for the floor. The estimate replaces
// it by its mean effect on a uniform fraction f: E[f] = 1/2, E[f*f] = 1/3. The
// color is exact. Only for an opaque target and current: alpha adds nothing there.
func (worker *Worker) estimateEnergy(lines []Scanline, alpha int) float64 {
	const m = 0xffff
	tables := worker.tables
	color, _ := tables.colorAndError(lines, alpha)
	sr, sg, sb, sa := color.NRGBA().RGBA()
	stride := tables.width + 1
	var delta float64
	for _, line := range lines {
		coverage := line.Alpha
		a := (m - sa*coverage/m) * 0x101
		k := float64(a) / (m << 8)
		q := [3]float64{
			float64(sr*coverage) / (m << 8),
			float64(sg*coverage) / (m << 8),
			float64(sb*coverage) / (m << 8),
		}
		pixels := float64(line.X2 - line.X1 + 1)
		row := tables.totals[line.Y*stride : (line.Y+1)*stride]
		first, last := &row[line.X1], &row[line.X2+1]
		for channel := range q {
			t := float64(last.target[channel] - first.target[channel])
			c := float64(last.current[channel] - first.current[channel])
			cc := float64(last.currentSquared[channel] - first.currentSquared[channel])
			tc := float64(last.targetCurrent[channel] - first.targetCurrent[channel])
			delta += 2*(1-k)*tc - 2*q[channel]*t + (k*k-1)*cc + 2*k*q[channel]*c + pixels*q[channel]*q[channel] +
				t - k*c - pixels*q[channel] + pixels/3
		}
	}
	scaledScore := worker.Score * 255
	pixelChannels := float64(worker.W * worker.H * 4)
	// The estimate of a pixel is positive, so only float rounding of a score near 0
	// gets below zero, and the root of that is NaN, which passes energy >= bestEnergy.
	total := max(scaledScore*scaledScore*pixelChannels+delta, 0)
	return math.Sqrt(total/pixelChannels) / 255
}
