package decker

import "math"

// Animations for block text (Block.FX). Each takes the time since the
// effect starts, in seconds.

// BlockDecrypt: cells flicker through shade characters in the noise color
// (a faint one reads best), then lock in with a bright flash, sweeping left
// to right.
func BlockDecrypt(t, dur float64, noise RGB) func(BlockCell) BlockFX {
	const shades = "░▒▓█▚▞▙▟"
	return func(c BlockCell) BlockFX {
		lock := dur * (0.15 + 0.85*c.U) * (0.85 + 0.3*Hash01(c.Col, c.Row, 3))
		switch {
		case t < lock-0.5:
			return BlockFX{}
		case t < lock:
			frame := int(t * 20)
			r := []rune(shades)[int(Hash01(c.Col, c.Row+frame, 7)*float64(len([]rune(shades))))]
			return BlockFX{Alpha: 0.6, Rune: r, Color: &noise}
		default:
			return BlockFX{Alpha: 1, Bright: 0.7 * (1 - Progress(t, lock, 0.35))}
		}
	}
}

// BlockRain: each column drops in from above with a little bounce,
// staggered left to right.
func BlockRain(t float64) func(BlockCell) BlockFX {
	return func(c BlockCell) BlockFX {
		lt := t - 0.5*c.U - 0.05*Hash01(c.Col, 0, 5)
		if lt <= 0 {
			return BlockFX{}
		}
		fall := Spring(-float64(c.H+4), 0, lt, 7, 0.55)
		return BlockFX{DY: fall, Alpha: Ease(lt, 0.15)}
	}
}

// BlockSlide: alternate rows glide in from opposite sides.
func BlockSlide(t float64, width int) func(BlockCell) BlockFX {
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

// BlockType: characters appear one at a time.
func BlockType(t, cps float64) func(BlockCell) BlockFX {
	return func(c BlockCell) BlockFX {
		if float64(c.Char) < t*cps {
			return BlockFX{Alpha: 1}
		}
		return BlockFX{}
	}
}

// BlockBeam: a bright band sweeps across once (layer it on settled text).
func BlockBeam(t, dur float64) func(BlockCell) BlockFX {
	pos := Lerp(-0.3, 1.3, Progress(t, 0, dur))
	return func(c BlockCell) BlockFX {
		d := math.Abs(c.U-pos+0.08*float64(c.Row)/math.Max(float64(c.H), 1)) / 0.12
		b := 0.0
		if d < 1 {
			b = 0.8 * (1 - d*d)
		}
		return BlockFX{Alpha: 1, Bright: b}
	}
}

// BlockGlitch: rows jump sideways and flicker now and then (for "this is
// broken" moments). amount is 0..1.
func BlockGlitch(t, amount float64) func(BlockCell) BlockFX {
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

// BlockFade: the whole block fades in.
func BlockFade(t, dur float64) func(BlockCell) BlockFX {
	a := Ease(t, dur)
	return func(BlockCell) BlockFX { return BlockFX{Alpha: a} }
}

// BlockChain combines effects: offsets add, alpha multiplies, the last rune
// or color set wins, brightness takes the max.
func BlockChain(fxs ...func(BlockCell) BlockFX) func(BlockCell) BlockFX {
	return func(c BlockCell) BlockFX {
		out := BlockFX{Alpha: 1}
		for _, f := range fxs {
			g := f(c)
			out.DX += g.DX
			out.DY += g.DY
			out.Alpha *= g.Alpha
			if g.Rune != 0 {
				out.Rune = g.Rune
			}
			if g.Color != nil {
				out.Color = g.Color
			}
			out.Bright = math.Max(out.Bright, g.Bright)
		}
		return out
	}
}
