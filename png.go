package decker

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// A snapshot cell is cellW×cellH pixels.
const cellW, cellH = 8, 16

// writePNG saves frameImage's rendering to path (-snapshot -png).
func writePNG(g *grid, path string) error { return savePNG(frameImage(g), path) }

// frameImage paints a frame as a terminal would, to preview slides without
// one: block and box-drawing characters by hand, the rest in a bitmap font.
func frameImage(g *grid) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, g.W*cellW, g.H*cellH))
	d := &font.Drawer{Dst: img, Face: basicfont.Face7x13}
	cell := NewPixels(cellW, cellH, RGB{})
	for i := range g.Cells {
		c := &g.Cells[i]
		r := image.Rect(i%g.W*cellW, i/g.W*cellH, i%g.W*cellW+cellW, i/g.W*cellH+cellH)
		cell.Fill(rgbOf(c.bg))
		r0, _ := utf8.DecodeRuneInString(c.ch)
		text := c.ch != "" && c.ch != " "
		blocky := text && drawBlockRunePx(cell, r0, 0, 0, cellW, rgbOf(c.fg), 1)
		for j, p := range cell.Pix {
			q := p.q()
			img.SetRGBA(r.Min.X+j%cellW, r.Min.Y+j/cellW, color.RGBA{q[0], q[1], q[2], 255})
		}
		if text && !blocky {
			d.Src = image.NewUniform(color.RGBA{c.fg[0], c.fg[1], c.fg[2], 255})
			d.Dot = fixed.P(r.Min.X, r.Min.Y+12)
			d.DrawString(c.ch)
		}
	}
	return img
}

// sheetCols is how many frames a contact sheet puts in a row.
const sheetCols = 4

// writeSheet renders frames into one contact-sheet image, sheetCols across,
// each shrunk by an integer factor.
func writeSheet(frames []*grid, shrink int, path string) error {
	const gap = 6
	fw, fh := frames[0].W*cellW/shrink, frames[0].H*cellH/shrink
	rows := (len(frames) + sheetCols - 1) / sheetCols
	sheet := image.NewRGBA(image.Rect(0, 0, sheetCols*(fw+gap)+gap, rows*(fh+gap)+gap))
	fillRect(sheet, sheet.Bounds(), color.Black)
	tile := make([]RGB, fw*fh)
	for i, fr := range frames {
		src := frameImage(fr)
		full := make([]RGB, len(src.Pix)/4)
		for j := range full {
			full[j] = RGB{float32(src.Pix[4*j]), float32(src.Pix[4*j+1]), float32(src.Pix[4*j+2])}
		}
		boxScale(tile, fw, fh, full, src.Rect.Dx(), src.Rect.Dy())
		ox, oy := gap+(i%sheetCols)*(fw+gap), gap+(i/sheetCols)*(fh+gap)
		for j, c := range tile {
			sheet.Set(ox+j%fw, oy+j/fw, c.Color())
		}
	}
	return savePNG(sheet, path)
}

func fillRect(img *image.RGBA, r image.Rectangle, c color.Color) {
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

func savePNG(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
