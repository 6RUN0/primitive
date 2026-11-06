package primitive

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// Color is a non-premultiplied color with channels from 0 to 255.
type Color struct {
	R, G, B, A int
}

// MakeColor converts c to 8-bit channels; a translucent c stays premultiplied.
func MakeColor(c color.Color) Color {
	r, g, b, a := c.RGBA()
	return Color{int(r / 257), int(g / 257), int(b / 257), int(a / 257)}
}

// MakeHexColor parses "rgb", "rgba", "rrggbb" or "rrggbbaa" hex digits with an
// optional leading "#". Alpha defaults to 255. Any other value is an error.
func MakeHexColor(x string) (Color, error) {
	digits := strings.TrimPrefix(x, "#")
	// strconv accepts neither a sign nor a 0x prefix for an explicit base 16.
	value, err := strconv.ParseUint(digits, 16, 32)
	if err != nil {
		return Color{}, fmt.Errorf("invalid hex color %q", x)
	}
	switch len(digits) {
	case 3:
		value = value<<4 | 0xf
		fallthrough
	case 4:
		return Color{
			int(value>>12&0xf) * 0x11,
			int(value>>8&0xf) * 0x11,
			int(value>>4&0xf) * 0x11,
			int(value&0xf) * 0x11,
		}, nil
	case 6:
		value = value<<8 | 0xff
		fallthrough
	case 8:
		return Color{
			int(value >> 24 & 0xff),
			int(value >> 16 & 0xff),
			int(value >> 8 & 0xff),
			int(value & 0xff),
		}, nil
	}
	return Color{}, fmt.Errorf("invalid hex color %q", x)
}

// NRGBA returns c as a standard library color.
func (c *Color) NRGBA() color.NRGBA {
	return color.NRGBA{uint8(c.R), uint8(c.G), uint8(c.B), uint8(c.A)}
}
