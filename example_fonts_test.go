package decker_test

import (
	"embed"
	"fmt"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/austinlparker/decker"
)

//go:embed fonts/figlet/Decker*.flf
var exampleFigletFonts embed.FS

func ExampleStockFigletFont() {
	font := decker.StockFigletFont("Decker Sign")
	fmt.Println(font.Name, font.Has("Hello, Go!"))
	// Output: Decker Sign true
}

func ExampleStockFigletFontNames() {
	for _, name := range decker.StockFigletFontNames() {
		fmt.Println(name)
	}
	// Output:
	// Decker Geometric
	// Decker Sign
	// Decker Slab
	// Decker Square
	// Decker Stencil
	// Spleen 12x24
	// Spleen 5x8
	// Spleen 6x12
	// Spleen 6x12 Shadow
	// Spleen 8x16
}

func ExampleLoadFigletFont() {
	font := decker.LoadFigletFont(exampleFigletFonts, "fonts/figlet/Decker Square.flf")
	fmt.Println(font.Name, font.Has("Custom fonts!"))
	// Output: Decker Square true
}

func ExampleParseFigletFont() {
	data, err := exampleFigletFonts.ReadFile("fonts/figlet/Decker Slab.flf")
	if err != nil {
		panic(err)
	}
	font, err := decker.ParseFigletFont("My heading font", data)
	if err != nil {
		panic(err)
	}
	fmt.Println(font.Name, font.Has("Hello, Go!"))
	// Output: My heading font true
}

func TestFigFontCompatibility(t *testing.T) {
	data, err := exampleFigletFonts.ReadFile("fonts/figlet/Decker Sign.flf")
	if err != nil {
		t.Fatal(err)
	}
	assets := fstest.MapFS{"custom.flf": {Data: data}}
	parsed, err := decker.ParseFigFont("custom", data)
	if err != nil {
		t.Fatal(err)
	}
	var legacy *decker.FigFont = parsed
	var canonical *decker.FigletFont = legacy
	if canonical != parsed {
		t.Fatal("font type alias changed pointer identity")
	}
	if !reflect.DeepEqual(decker.StockFigFontNames(), decker.StockFigletFontNames()) {
		t.Fatal("legacy font catalog differs")
	}
	for _, pair := range [][2]*decker.FigletFont{
		{decker.LoadFigFont(assets, "custom.flf"), decker.LoadFigletFont(assets, "custom.flf")},
		{decker.StockFigFont("Decker Sign"), decker.StockFigletFont("Decker Sign")},
	} {
		if pair[0].Name != pair[1].Name || pair[0].Width("Hello, Go!") != pair[1].Width("Hello, Go!") {
			t.Fatal("legacy font loader differs")
		}
	}
	// Existing typed function references remain assignable across the alias.
	var _ func(string, []byte) (*decker.FigFont, error) = decker.ParseFigletFont
	var _ func(string, float64, float64, int, int, ...*decker.FigFont) (*decker.FigFont, []string, float64) = decker.FitBlock
	_ = decker.Block{Font: legacy}
}
