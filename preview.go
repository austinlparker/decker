package decker

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// writePNG paints a rendered frame into a PNG the way a terminal would show
// it: each cell is cellW×cellH pixels, "▀" cells are split into two colored
// halves, and other characters are drawn with a small bitmap font. Used by
// `-snapshot -png file.png` to preview slides without a terminal.
func writePNG(frame string, w, h int, path string, t *Theme) error {
	return savePNG(frameImage(frame, w, h, t), path)
}

// frameImage paints a frame into an image (see writePNG).
func frameImage(frame string, w, h int, t *Theme) *image.RGBA {
	const cellW, cellH = 8, 16
	cv := lipgloss.NewCanvas(w, h)
	uv.NewStyledString(frame).Draw(cv, cv.Bounds())

	img := image.NewRGBA(image.Rect(0, 0, w*cellW, h*cellH))
	bgDefault := t.Background.Color()
	fgDefault := t.Text.Color()
	draw.Draw(img, img.Bounds(), &image.Uniform{bgDefault}, image.Point{}, draw.Src)

	d := &font.Drawer{Dst: img, Face: basicfont.Face7x13}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := cv.CellAt(x, y)
			if cell == nil || cell.Width == 0 {
				continue
			}
			fg, bg := cell.Style.Fg, cell.Style.Bg
			if fg == nil {
				fg = fgDefault
			}
			if bg == nil {
				bg = bgDefault
			}
			r := image.Rect(x*cellW, y*cellH, (x+1)*cellW, (y+1)*cellH)
			draw.Draw(img, r, &image.Uniform{bg}, image.Point{}, draw.Src)
			switch cell.Content {
			case "", " ":
			case "▀":
				top := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+cellH/2)
				draw.Draw(img, top, &image.Uniform{fg}, image.Point{}, draw.Src)
			case "▁":
				low := image.Rect(r.Min.X, r.Max.Y-cellH/8, r.Max.X, r.Max.Y)
				draw.Draw(img, low, &image.Uniform{fg}, image.Point{}, draw.Src)
			default:
				if drawBlockRune(img, []rune(cell.Content)[0], r, fg, bg) {
					continue
				}
				d.Src = &image.Uniform{fg}
				d.Dot = fixed.P(r.Min.X, r.Min.Y+12)
				d.DrawString(cell.Content)
			}
		}
	}
	return img
}

// writeSheet renders frames into one contact-sheet image, cols across,
// each shrunk by an integer factor.
func writeSheet(frames []string, w, h, cols, shrink int, path string, t *Theme) error {
	const cellW, cellH = 8, 16
	fw, fh := w*cellW/shrink, h*cellH/shrink
	rows := (len(frames) + cols - 1) / cols
	gap := 6
	sheet := image.NewRGBA(image.Rect(0, 0, cols*(fw+gap)+gap, rows*(fh+gap)+gap))
	draw.Draw(sheet, sheet.Bounds(), &image.Uniform{color.Black}, image.Point{}, draw.Src)
	for i, fr := range frames {
		src := frameImage(fr, w, h, t)
		ox, oy := gap+(i%cols)*(fw+gap), gap+(i/cols)*(fh+gap)
		for y := 0; y < fh; y++ {
			for x := 0; x < fw; x++ {
				var r, g, b uint32
				for yy := 0; yy < shrink; yy++ {
					for xx := 0; xx < shrink; xx++ {
						c := src.RGBAAt(x*shrink+xx, y*shrink+yy)
						r, g, b = r+uint32(c.R), g+uint32(c.G), b+uint32(c.B)
					}
				}
				n := uint32(shrink * shrink)
				sheet.Set(ox+x, oy+y, RGB{float32(r / n), float32(g / n), float32(b / n)}.Color())
			}
		}
	}
	return savePNG(sheet, path)
}

func savePNG(img image.Image, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
