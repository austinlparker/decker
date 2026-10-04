package decker

import (
	"math"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// testTheme and testDeck stand in for a talk in the engine's tests: a few
// slides that exercise block letters, smooth type, the stock components,
// the character layer, builds, every transition, and the overlay.
var testTheme = &Theme{
	Background: Hex("#101820"),
	Text:       Hex("#F0F0F0"),
	Muted:      Hex("#A0A8B0"),
	Faint:      Hex("#384048"),
	Accent:     Hex("#FF7A00"),
	Accent2:    Hex("#40C0FF"),
	Warn:       Hex("#FF6060"),
	Good:       Hex("#70D050"),
	Panel:      Hex("#0A1016"),
	Display:    StockFont("SpaceGrotesk-Bold"),
	Body:       StockFont("SpaceGrotesk-Medium"),
	Mono:       StockFont("JetBrainsMono-ExtraBold"),
	Overlay: func(_ Ctx, p *Pixels) {
		p.Disc(float64(p.W)-4, float64(p.H)-4, 2, Hex("#FF7A00"), 1)
	},
}

func testDeck() *Deck {
	return &Deck{Name: "deck-test", Theme: testTheme, Slides: []Slide{
		{
			Title: "Block letters", HideChrome: true, Transition: TransitionDissolve,
			View: func(c Ctx) string {
				sc := c.Scene()
				f, lines, scale := FitBlock("Hello deck", c.X(0.9), c.Y(0.5), 2, 1, BlockShadow, BlockSolid)
				Block{Font: f, Scale: scale, Color: c.Theme.Accent, Align: Center, Gap: 1, Glow: 0.4,
					FX: BlockChain(BlockDecrypt(c.T, 0.8, c.Theme.Faint), BlockBeam(math.Mod(c.T, 4), 1))}.
					Draw(sc.Px, strings.Join(lines, "\n"), c.X(0.5), c.Y(0.1))
				s := c.SmallText(c.Theme.Body)
				Text{Font: c.Theme.Body, Size: s, Align: Center, Color: c.Theme.Text, FX: FadeUp(c.T-0.5, 0.4, s)}.
					Draw(sc.Px, "a deck for testing the engine", c.X(0.5), c.Y(0.75))
				return sc.Render()
			},
		},
		{
			Title: "Bullets", Steps: 3,
			View: func(c Ctx) string {
				sc := c.Scene()
				BulletList(c, sc.Px, []string{"one idea", "per slide", "with builds"}, c.X(0.08), c.Y(0.1), c.X(0.84), c.Y(0.8), 0)
				return sc.Render()
			},
		},
		{
			Title: "Diagram", Steps: 4, Transition: TransitionWipe, Notes: "a ring, a box and an arrow",
			View: func(c Ctx) string {
				sc := c.Scene()
				p := sc.Px
				Panel(c, p, c.X(0.05), c.Y(0.05), c.X(0.25), c.Y(0.15), "panel", c.Theme.Panel, c.Theme.Accent2, c.Theme.Text, 1)
				Arrow(p, c.X(0.3), c.Y(0.12), c.X(0.5), c.Y(0.12), 2, c.Theme.Muted, Ease(c.T, 0.5))
				LineLabel(c, p, c.Theme.Mono, "call", c.X(0.4), c.Y(0.12), c.Theme.Text, 1)
				Chip(c, p, "chip", c.X(0.55), c.Y(0.06), c.Theme.Background, c.Theme.Good, 1)
				CycleDiagram{Labels: []string{"look", "think", "act", "learn"}, Lap: 4}.Draw(c, p, c.Y(0.25))
				return sc.Render()
			},
		},
		{
			Title: "Character layer", Transition: TransitionNone,
			View: func(c Ctx) string {
				sc := c.Scene()
				sc.Px.VGradient(0, sc.Px.H-1, c.Theme.Background, c.Theme.Panel)
				box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Render("terminal text")
				sc.PutCenter(0, 0, box)
				sc.Text(1, 1, "plain", c.Theme.Muted.Color())
				return sc.Render()
			},
		},
		{
			Title: "Last",
			View: func(c Ctx) string {
				sc := c.Scene()
				PlaceholderBox(c, sc.Px, c.X(0.1), c.Y(0.1), c.X(0.8), c.Y(0.6), "a screenshot goes here")
				IllustrativeTag(c, sc.Px, c.X(0.9), c.Y(0.8))
				return sc.Render()
			},
		},
	}}
}

func TestPanickingSlideShowsError(t *testing.T) {
	s := Slide{Title: "boom", View: func(Ctx) string { panic("on purpose") }}
	out := renderSlide(s, Ctx{W: 60, H: 10, Theme: testTheme})
	if !strings.Contains(frameText(out), "on purpose") {
		t.Fatalf("the panic isn't on screen:\n%s", frameText(out))
	}
}

func TestOverlayDrawsOnSlides(t *testing.T) {
	d := testDeck()
	c := Ctx{W: 40, H: 12, T: Settled, Theme: testTheme}
	with := d.Render(4, c)
	plain := *testTheme
	plain.Overlay = nil
	c.Theme = &plain
	if d.Render(4, c) == with {
		t.Fatal("the overlay changed nothing")
	}
}

func TestDeckCheck(t *testing.T) {
	good := testDeck()
	if err := good.check(); err != nil {
		t.Fatal(err)
	}
	for name, d := range map[string]Deck{
		"no name":   {Theme: testTheme, Slides: good.Slides},
		"no theme":  {Name: "x", Slides: good.Slides},
		"no fonts":  {Name: "x", Theme: &Theme{}, Slides: good.Slides},
		"no slides": {Name: "x", Theme: testTheme},
	} {
		if d.check() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func frameText(s string) string { return strings.Join(frameLines(s), "\n") }
