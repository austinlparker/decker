package decker

// Stage layout helpers. On a projector everything should be sized relative
// to the screen, not in terminal cells, so slides think in fractions of the
// pixel canvas: c.X(0.5) is the horizontal center, c.Size(0.1) is a font
// one tenth of the screen tall.

// PW and PH are the pixel canvas size (PH is twice the height in cells).
func (c Ctx) PW() float64 { return float64(c.W) }
func (c Ctx) PH() float64 { return float64(2 * c.H) }

// X and Y convert fractions of the canvas to pixel coordinates.
func (c Ctx) X(f float64) float64 { return f * c.PW() }
func (c Ctx) Y(f float64) float64 { return f * c.PH() }

// Size is a font size (or length) as a fraction of the canvas height.
func (c Ctx) Size(f float64) int { return max(int(f*c.PH()), 6) }

// MinText is the smallest text size (as a fraction of the canvas height)
// that stays readable at typical projector resolutions. Don't go below it.
const MinText = 0.068

// Unit is a length that scales with the canvas: a fraction of its height.
func (c Ctx) Unit(f float64) float64 { return f * c.PH() }

// SmallText is the smallest readable text size for f on this screen, as it
// will actually be drawn.
func (c Ctx) SmallText(f *Font) int { return f.Drawn(c.Size(MinText)) }
