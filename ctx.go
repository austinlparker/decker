package decker

// Ctx is everything a slide needs to draw a frame. Its layout methods take
// fractions of the pixel canvas (W wide, 2H tall), so a slide looks the same at
// any size: c.X(0.5) is the center, c.Size(0.1) a font a tenth of the screen
// tall.
type Ctx struct {
	W, H  int     // drawable area in cells
	T     float64 // seconds since this slide appeared
	Step  int     // current build step, 0-based
	StepT float64 // seconds since the current step began

	// Index is this slide's 0-based position in the deck and Count the number
	// of slides, so an overlay can draw "12 / 40" or a progress bar. Section
	// is the slide's resolved Slide.Section: its own, or the nearest one
	// before it, empty before the first. All three are zero in a Ctx built by
	// hand and not passed through the engine (Deck.Render and Deck.Draw fill
	// them in from the slide index).
	Index, Count int
	Section      string

	// Theme is the deck's theme; never nil inside View.
	Theme *Theme

	// review collects what this frame reports under Deck.Review; nil while
	// presenting.
	review *reviewLog
}

// at returns c positioned at slide i of slides.
func (c Ctx) at(slides []Slide, i int) Ctx {
	c.Index, c.Count, c.Section = i, len(slides), sectionAt(slides, i)
	return c
}

// Reached reports whether the slide is at or past step.
func (c Ctx) Reached(step int) bool { return c.Step >= step }

// Since returns seconds since step began: negative before it, Settled after
// it. Animate a step's element with Ease(Since(2), 0.5).
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

// Settled is a time past every entrance animation; slides entered backwards
// or past a step use it to appear fully built.
const Settled = 1000.0

// PW returns the pixel canvas width.
func (c Ctx) PW() float64 { return float64(c.W) }

// PH returns the pixel canvas height, twice the height in cells.
func (c Ctx) PH() float64 { return float64(2 * c.H) }

// X converts a fraction of the canvas width to a pixel x coordinate.
func (c Ctx) X(f float64) float64 { return f * c.PW() }

// Y converts a fraction of the canvas height to a pixel y coordinate.
func (c Ctx) Y(f float64) float64 { return f * c.PH() }

// Size returns a font size in pixels, f of the canvas height, at least 6.
func (c Ctx) Size(f float64) int { return max(int(f*c.PH()), 6) }

// MinText is the smallest text size, as a canvas-height fraction, readable at
// projector resolutions.
const MinText = 0.068

// Unit scales a stroke, gap or radius with the canvas: f of its height.
// Same as Y; use it when the number isn't a position.
func (c Ctx) Unit(f float64) float64 { return f * c.PH() }

// SmallText is the smallest readable size for f on this screen, as drawn.
func (c Ctx) SmallText(f *Font) int { return f.Drawn(c.Size(MinText)) }
