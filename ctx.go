package decker

// Ctx is everything a slide needs to draw a frame.
//
// Its layout methods take fractions of the pixel canvas, so a slide looks the
// same at any terminal size: c.X(0.5) is the horizontal center, c.Size(0.1) a
// font one tenth of the screen tall.
type Ctx struct {
	W, H  int     // drawable area in cells
	T     float64 // seconds since this slide appeared
	Step  int     // current build step, 0-based
	StepT float64 // seconds since the current step began

	// Theme is the deck's theme; never nil inside View.
	Theme *Theme
}

// Scene returns an empty Scene the size of the drawable area, on the
// theme's background. Rendering it draws the theme's Overlay on top.
func (c Ctx) Scene() *Scene {
	sc := NewScene(c.W, c.H, c.Theme)
	if o := c.Theme.Overlay; o != nil {
		sc.overlay = func(p *Pixels) { o(c, p) }
	}
	return sc
}

// Reached reports whether the slide is at or past the given step.
func (c Ctx) Reached(step int) bool { return c.Step >= step }

// Since returns seconds elapsed since the given step began: negative
// before the step, and a large "settled" value if we are past it. Use it to
// animate the element a step introduces, e.g. Ease(Since(2), 0.5).
func (c Ctx) Since(step int) float64 {
	switch {
	case c.Step < step:
		return -1
	case c.Step == step:
		return c.StepT
	default:
		return Settled
	}
}

// Settled is a time far enough in the future that every entrance animation
// has finished. Slides entered "backwards" (or past steps) use it so they
// appear fully built rather than replaying their animations.
const Settled = 1000.0

// PW returns the pixel canvas width.
func (c Ctx) PW() float64 { return float64(c.W) }

// PH returns the pixel canvas height, twice the height in cells.
func (c Ctx) PH() float64 { return float64(2 * c.H) }

// X converts a fraction of the canvas width to a pixel x coordinate.
func (c Ctx) X(f float64) float64 { return f * c.PW() }

// Y converts a fraction of the canvas height to a pixel y coordinate.
func (c Ctx) Y(f float64) float64 { return f * c.PH() }

// Size is a font size in pixels as a fraction of the canvas height, never
// below 6.
func (c Ctx) Size(f float64) int { return max(int(f*c.PH()), 6) }

// MinText is the smallest text size (as a fraction of the canvas height)
// that stays readable at typical projector resolutions. Don't go below it.
const MinText = 0.068

// Unit is a length that scales with the canvas (a stroke, gap or radius): a
// fraction of its height. It equals Y; use it when the number is not a
// position.
func (c Ctx) Unit(f float64) float64 { return f * c.PH() }

// SmallText is the smallest readable text size for f on this screen, as it
// will actually be drawn.
func (c Ctx) SmallText(f *Font) int { return f.Drawn(c.Size(MinText)) }
