package decker

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// RGB is a color with 0..255 float channels, used for fast blending.
type RGB struct{ R, G, B float32 }

// Hex parses a CSS-style hex color such as "#FFB000" or "#fb0"; unparseable
// input is black.
func Hex(s string) RGB { return toRGB(lipgloss.Color(s)) }

func toRGB(c color.Color) RGB {
	r, g, b, _ := c.RGBA()
	return RGB{float32(r >> 8), float32(g >> 8), float32(b >> 8)}
}

// toRGBA splits c into straight (not premultiplied) color and alpha in 0..1.
// color.Color reports premultiplied channels, so a translucent pixel has to be
// divided back out or it would draw darker than it is. Opaque pixels take the
// toRGB path unchanged.
func toRGBA(c color.Color) (RGB, float32) {
	r, g, b, a := c.RGBA()
	if a == 0xffff {
		return RGB{float32(r >> 8), float32(g >> 8), float32(b >> 8)}, 1
	}
	if a == 0 {
		return RGB{}, 0
	}
	k := 255 / float32(a)
	return RGB{float32(r) * k, float32(g) * k, float32(b) * k}, float32(a) / 0xffff
}

// Mix blends two colors: p=0 is a, p=1 is b.
func Mix(a, b RGB, p float64) RGB {
	q := float32(Clamp01(p))
	return RGB{a.R + (b.R-a.R)*q, a.G + (b.G-a.G)*q, a.B + (b.B-a.B)*q}
}

// Scale multiplies brightness, clamped to 255.
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
func (c RGB) q() [3]uint8 { return [3]uint8{q8(c.R), q8(c.G), q8(c.B)} }

// q8 uses plain comparisons: builtin min/max's NaN handling costs ~30% on
// this per-cell path.
func q8(v float32) uint8 {
	if v += 0.5; v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}
