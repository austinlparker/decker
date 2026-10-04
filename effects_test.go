package decker

import "testing"

func TestChainCombines(t *testing.T) {
	fx := Chain(
		func(int) GlyphFX { return GlyphFX{DX: 1, DY: 2, Alpha: 0.5, Rune: 'a'} },
		func(int) GlyphFX { return GlyphFX{DX: 3, Alpha: 0.5, Rune: 'b'} },
		func(int) GlyphFX { return GlyphFX{Alpha: 1} },
	)(0)
	if fx != (GlyphFX{DX: 4, DY: 2, Alpha: 0.25, Rune: 'b'}) {
		t.Errorf("Chain = %+v", fx)
	}
}

func TestEffectsStartHiddenAndSettle(t *testing.T) {
	for name, fx := range map[string]GlyphEffect{
		"RiseIn": RiseIn(0, 0.1, 20),
		"DropIn": DropIn(0, 0.1, 20),
		"Decode": Decode(0, 1, 5),
		"TypeOn": TypeOn(0, 10),
	} {
		if got := fx(2); got != (GlyphFX{}) {
			t.Errorf("%s at t=0 = %+v, want hidden", name, got)
		}
	}
	for name, fx := range map[string]GlyphEffect{
		"RiseIn": RiseIn(10, 0.1, 20),
		"DropIn": DropIn(10, 0.1, 20),
		"Decode": Decode(10, 1, 5),
		"TypeOn": TypeOn(10, 10),
		"FadeUp": FadeUp(10, 1, 20),
	} {
		if got := fx(2); got.Alpha != 1 || got.DX != 0 || got.Rune != 0 || got.DY > 1e-6 || got.DY < -1e-6 {
			t.Errorf("%s settled = %+v, want untouched", name, got)
		}
	}
}

func TestTypeOnRevealsInOrder(t *testing.T) {
	fx := TypeOn(0.5, 10)
	for i, want := range []float64{1, 1, 1, 1, 1, 0, 0} {
		if got := fx(i).Alpha; got != want {
			t.Errorf("glyph %d alpha = %v, want %v", i, got, want)
		}
	}
}

func TestShineBandSweeps(t *testing.T) {
	if s := ShineBand(0.5, 1, 1)(0.5); s < 0.99 {
		t.Errorf("band at the middle of the sweep = %v at u=0.5, want ~1", s)
	}
	if s := ShineBand(0.5, 1, 1)(0.9); s != 0 {
		t.Errorf("shine far from the band = %v, want 0", s)
	}
}
