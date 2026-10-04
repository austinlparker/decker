package decker

import "math"

// BlockCell describes one non-blank cell of rendered block text, for effects.
type BlockCell struct {
	Char        int     // index of the source character (across all lines)
	Col, Row    int     // position within the whole block, in font cells
	W, H        int     // block size in font cells
	U           float64 // Col / (W-1): 0 at the left edge, 1 at the right
	Rune        rune    // the font's character for this cell
	Solid       bool    // a filled block character, as opposed to a box-drawing edge
	Line, Line0 int     // which line, and the Char index where it starts
}

// BlockFX is an effect's instruction for one cell of block text.
type BlockFX struct {
	DX, DY float64 // offset in font cells (fractions allowed)
	Alpha  float64 // 0 hides the cell, 1 is fully visible; Draw skips cells at 0.02 or below
	Rune   rune    // 0 keeps the real character
	Bright float64 // 0..1 pushes the color toward white
	Color  *RGB    // replaces the color entirely
}

func (a BlockFX) then(b BlockFX) BlockFX {
	a.DX += b.DX
	a.DY += b.DY
	a.Alpha *= b.Alpha
	if b.Rune != 0 {
		a.Rune = b.Rune
	}
	if b.Color != nil {
		a.Color = b.Color
	}
	a.Bright = math.Max(a.Bright, b.Bright)
	return a
}

// BlockEffect animates the cells of a Block (Block.FX); Draw calls it for each
// non-blank cell. It follows GlyphEffect's contract: a pure function of t
// (seconds since start) and the cell, where the zero BlockFX hides the cell and
// BlockFX{Alpha: 1} leaves it alone. Combine with BlockChain.
type BlockEffect func(BlockCell) BlockFX

var decryptShades = []rune("░▒▓█▚▞▙▟")

// BlockDecrypt flickers cells through shade characters in the noise color
// (faint reads best), then locks them in with a bright flash, left to right
// over dur.
func BlockDecrypt(t, dur float64, noise RGB) BlockEffect {
	return func(c BlockCell) BlockFX {
		lock := dur * (0.15 + 0.85*c.U) * (0.85 + 0.3*Hash01(c.Col, c.Row, 3))
		switch {
		case t < lock-0.5:
			return BlockFX{}
		case t < lock:
			frame := int(t * 20)
			r := decryptShades[int(Hash01(c.Col, c.Row+frame, 7)*float64(len(decryptShades)))]
			return BlockFX{Alpha: 0.6, Rune: r, Color: &noise}
		default:
			return BlockFX{Alpha: 1, Bright: 0.7 * (1 - Progress(t, lock, 0.35))}
		}
	}
}

// BlockRain drops each column in from above with a bounce, staggered left to
// right.
func BlockRain(t float64) BlockEffect {
	return func(c BlockCell) BlockFX {
		lt := t - 0.5*c.U - 0.05*Hash01(c.Col, 0, 5)
		if lt <= 0 {
			return BlockFX{}
		}
		fall := Spring(-float64(c.H+4), 0, lt, 7, 0.55)
		return BlockFX{DY: fall, Alpha: Ease(lt, 0.15)}
	}
}

// BlockSlide glides alternate rows in from opposite sides; width is the
// block's width in cells (Block.Cells).
func BlockSlide(t float64, width int) BlockEffect {
	return func(c BlockCell) BlockFX {
		dir := 1.0
		if c.Row%2 == 1 {
			dir = -1
		}
		lt := t - 0.04*float64(c.Row)
		off := dir * float64(width) * (1 - EaseOutCubic(Progress(lt, 0, 0.6)))
		return BlockFX{DX: off, Alpha: Ease(lt, 0.3)}
	}
}

// BlockType shows characters one at a time at cps characters per second.
func BlockType(t, cps float64) BlockEffect {
	return func(c BlockCell) BlockFX {
		if float64(c.Char) < t*cps {
			return BlockFX{Alpha: 1}
		}
		return BlockFX{}
	}
}

// BlockBeam sweeps a bright band across once in dur seconds; layer it on
// settled text.
func BlockBeam(t, dur float64) BlockEffect {
	pos := sweepPos(t, dur)
	return func(c BlockCell) BlockFX {
		return BlockFX{Alpha: 1, Bright: 0.8 * bandFalloff(c.U-pos+0.08*float64(c.Row)/math.Max(float64(c.H), 1))}
	}
}

// BlockGlitch jumps rows sideways and flickers cells; amount is 0..1.
func BlockGlitch(t, amount float64) BlockEffect {
	frame := int(t * 12)
	return func(c BlockCell) BlockFX {
		fx := BlockFX{Alpha: 1}
		if Hash01(c.Row, frame, 31) < amount*0.5 {
			fx.DX = math.Round((Hash01(c.Row, frame, 32) - 0.5) * 6)
		}
		if Hash01(c.Col, frame, 33) < amount*0.08 {
			fx.Alpha = 0.3
		}
		return fx
	}
}

// BlockFade fades the whole block in over dur seconds.
func BlockFade(t, dur float64) BlockEffect {
	a := Ease(t, dur)
	return func(BlockCell) BlockFX { return BlockFX{Alpha: a} }
}

// BlockChain runs effects together: offsets add, alpha multiplies, the last
// rune or color set wins, brightness takes the max.
func BlockChain(fxs ...BlockEffect) BlockEffect {
	return func(c BlockCell) BlockFX {
		out := BlockFX{Alpha: 1}
		for _, f := range fxs {
			out = out.then(f(c))
		}
		return out
	}
}
