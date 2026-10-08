package decker_test

import (
	"embed"
	"fmt"

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
