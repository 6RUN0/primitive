package primitive

import (
	"math"
	"strconv"
	"strings"
)

// Decimals of the numbers in SVG output. Coordinates are in target pixels, so the
// rounding error in output pixels is 0.005 x Model.Scale.
const (
	svgCoordinateDecimals = 2
	svgOpacityDecimals    = 3
	svgCoordinateUnits    = 100
)

// formatSVGNumber returns x with at most decimals digits after the point and no
// redundant characters: 0.50 is ".5", -0.001 is "0". A non-finite x gives "0": a
// parser that meets NaN drops the element or the rest of the document.
func formatSVGNumber(x float64, decimals int) string {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return "0"
	}
	text := strconv.FormatFloat(x, 'f', decimals, 64)
	if strings.Contains(text, ".") {
		text = strings.TrimRight(strings.TrimRight(text, "0"), ".")
	}
	switch {
	case text == "-0":
		return "0"
	case strings.HasPrefix(text, "0."):
		return text[1:]
	case strings.HasPrefix(text, "-0."):
		return "-" + text[2:]
	}
	return text
}

func formatSVGCoordinate(x float64) string {
	return formatSVGNumber(x, svgCoordinateDecimals)
}

func formatSVGOpacity(alpha int) string {
	return formatSVGNumber(float64(alpha)/255, svgOpacityDecimals)
}

// roundSVGCoordinate returns x as a whole number of output units. Relative path
// commands are computed on these: a delta of two rounded points adds no error,
// while rounding each delta of exact points would let the error grow along a path.
func roundSVGCoordinate(x float64) int64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return 0
	}
	return int64(math.Round(x * svgCoordinateUnits))
}

// joinSVGPathNumbers writes coordinates from roundSVGCoordinate in the shortest
// form of the path grammar: a number ends at a sign or at a second decimal point,
// so only the other cases need a space. The SVG 1.1 grammar of transform lists
// requires a separator inside and between functions, so they do not use this.
func joinSVGPathNumbers(values ...int64) string {
	var path strings.Builder
	previous := ""
	for _, value := range values {
		text := formatSVGCoordinate(float64(value) / svgCoordinateUnits)
		isSelfDelimited := text[0] == '-' || (text[0] == '.' && strings.Contains(previous, "."))
		if previous != "" && !isSelfDelimited {
			path.WriteByte(' ')
		}
		path.WriteString(text)
		previous = text
	}
	return path.String()
}

// closedSVGPath returns path data of the polygon through the points.
func closedSVGPath(xs, ys []float64) string {
	startX, startY := roundSVGCoordinate(xs[0]), roundSVGCoordinate(ys[0])
	previousX, previousY := startX, startY
	deltas := make([]int64, 0, 2*len(xs))
	for i := 1; i < len(xs); i++ {
		x, y := roundSVGCoordinate(xs[i]), roundSVGCoordinate(ys[i])
		deltas = append(deltas, x-previousX, y-previousY)
		previousX, previousY = x, y
	}
	return "M" + joinSVGPathNumbers(startX, startY) + "l" + joinSVGPathNumbers(deltas...) + "z"
}

// buildSVGElement returns an empty element. Attribute order is fixed by the
// callers, attrs first: a shared prefix is what gzip compresses best.
func buildSVGElement(name, attrs, geometry string) string {
	if attrs == "" {
		return "<" + name + " " + geometry + "/>"
	}
	return "<" + name + " " + attrs + " " + geometry + "/>"
}

// findCommonAlpha returns the most frequent alpha, the lowest of equally frequent
// ones and 255 for no colors: the group carries it, shapes only their exceptions.
func findCommonAlpha(colors []Color) int {
	counts := make(map[int]int)
	common, commonCount := 255, 0
	for _, c := range colors {
		counts[c.A]++
		count := counts[c.A]
		if count > commonCount || (count == commonCount && c.A < common) {
			common, commonCount = c.A, count
		}
	}
	return common
}

// surveySVGCurves returns the number of curves among shapes and the stroke width
// they share in output form, "" if there are none or their widths differ.
func surveySVGCurves(shapes []Shape) (curveCount int, sharedWidth string) {
	hasOneWidth := true
	for _, shape := range shapes {
		curve, isCurve := shape.(*Quadratic)
		if !isCurve {
			continue
		}
		width := formatSVGCoordinate(curve.Width)
		hasOneWidth = hasOneWidth && (curveCount == 0 || width == sharedWidth)
		curveCount++
		sharedWidth = width
	}
	if !hasOneWidth {
		return curveCount, ""
	}
	return curveCount, sharedWidth
}
