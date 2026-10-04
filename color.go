package decker

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// RGB is a color with 0..255 float channels, used for fast blending.
type RGB struct{ R, G, B float32 }

// Hex parses a CSS-style hex color such as "#FFB000" or "#fb0". Anything
// unparseable yields black.
func Hex(s string) RGB { return toRGB(lipgloss.Color(s)) }

func toRGB(c color.Color) RGB {
	r, g, b, _ := c.RGBA()
	return RGB{float32(r >> 8), float32(g >> 8), float32(b >> 8)}
}

// Mix blends two colors: p=0 is a, p=1 is b.
func Mix(a, b RGB, p float64) RGB {
	q := float32(Clamp01(p))
	return RGB{a.R + (b.R-a.R)*q, a.G + (b.G-a.G)*q, a.B + (b.B-a.B)*q}
}

// Scale multiplies a color's brightness.
func (c RGB) Scale(k float64) RGB {
	f := float32(k)
	return RGB{min(c.R*f, 255), min(c.G*f, 255), min(c.B*f, 255)}
}

// Color returns c as an opaque color.Color, for Lip Gloss.
func (c RGB) Color() color.Color {
	q := c.q()
	return color.RGBA{q[0], q[1], q[2], 255}
}

// q rounds c to 0-255 bytes.
func (c RGB) q() [3]uint8 {
	cl := func(v float32) uint8 { return uint8(max(0, min(255, v+0.5))) }
	return [3]uint8{cl(c.R), cl(c.G), cl(c.B)}
}
