# Bundled block fonts

The five default FIGlet fonts are conversions of **Spleen 2.2.0** by
Frederic Cambus, licensed under **BSD-2-Clause**. The original copyright
and full license are retained in [Spleen-LICENSE.txt](Spleen-LICENSE.txt)
and in every generated font's comment header, including the embedded
copies in compiled programs. These font files keep BSD terms separately
from Decker's MIT code license.

| Exported font | Bundled file | Source bitmap | Declared FIGlet rows |
| --- | --- | --- | --- |
| `BlockSmall` | `Spleen 5x8.flf` | 5×8 | 4 |
| `BlockSolid` | `Spleen 6x12.flf` | 6×12 | 6 |
| `BlockShadow` | `Spleen 6x12 Shadow.flf` | 6×12 with a generated shaded shadow | 7 |
| `BlockHuge` | `Spleen 8x16.flf` | 8×16 | 8 |
| `BlockFancy` | `Spleen 12x24.flf` | 12×24 | 12 |

Decker trims blank padding rows when rendering; `FigletFont.Rows()` reports
its layout height. All fonts contain the 95 printable ASCII input glyphs,
including distinct lowercase letters, digits, quotes and punctuation.
The FIGlet loader currently loads ASCII only, even when the source BDF
contains additional Unicode glyphs.

## Optional display fonts

`StockFigletFontNames()` lists all ten bundled faces in alphabetical order.
`StockFigletFont(name)` selects any one by the name below, without `.flf`.
It panics for an unknown name; load each font once before rendering frames.

| Name | Original design | Declared FIGlet rows | Font notice |
| --- | --- | --- | --- |
| `Decker Sign` | Bungee | 14 | [bungee-OFL.txt](bungee-OFL.txt) |
| `Decker Geometric` | Rubik Mono One | 14 | [rubikmonoone-OFL.txt](rubikmonoone-OFL.txt) |
| `Decker Stencil` | Black Ops One | 14 | [blackopsone-OFL.txt](blackopsone-OFL.txt) |
| `Decker Slab` | Alfa Slab One | 15 | [alfaslabone-OFL.txt](alfaslabone-OFL.txt) |
| `Decker Square` | Russo One | 15 | [russoone-OFL.txt](russoone-OFL.txt) |

These are outline-to-half-block conversions under **SIL OFL 1.1**, with
source notices retained in the files themselves and separately above.
Notices normalize line endings and trailing whitespace while preserving
the full copyright and license text; source hashes identify the original
upstream bytes.
Each font has all 95 printable ASCII input glyphs. Bungee and Rubik Mono
One use uppercase-shaped lowercase; the other three have true lowercase.
The declared heights include padding and tall punctuation; use `Rows()`
for layout, and `FitBlock` to fit headings. `Block.Drop` adds an independently
colored shadow to any face, including Bungee.

The source TTF binaries are not bundled. Exact source and license hashes,
copyright metadata and conversion settings are in
[display-sources.json](display-sources.json). Converted primary names avoid
the reserved names "Alfa Slab" and "Russo". Black Ops One's embedded TTF
copyright names the PinyonScript project, while its accompanying OFL
notice names Black-Ops; both upstream statements are retained. Both
declare OFL 1.1. Keep all font notices when distributing binaries, and
keep derived font files under OFL rather than Decker's MIT code license.

The [display converter](generate_display.py) verifies those hashes and
notices, rasterizes at approximately 18 pixels of capital ink height with
a shared baseline, then packs each pair of vertical pixels into a FIGlet
half-block. It preserves each glyph's advance plus one blank column;
full-width layout disables additional kerning. Regeneration requires
Pillow 12.3.0, fontTools 4.61.1 and FreeType 2.14.3; these are asset-generation
tools, not Go build or runtime dependencies.

Download each `source_filename` from its `source_directory` beneath
`https://raw.githubusercontent.com/google/fonts/main/ofl/`, together with
that directory's `OFL.txt`, saved as `<source_directory>-OFL.txt`. Place
them together in a source directory, then run:

```sh
python3 fonts/figlet/generate_display.py /path/to/display-sources
python3 fonts/figlet/generate_display.py /path/to/display-sources --check
```

The converter rejects downloads whose hashes differ from the recorded
versions. If upstream changes, retrieve matching sources from its history
or review and update the manifest and rendered output deliberately.

For consumer fonts, `LoadFigletFont` accepts an `fs.FS`, and
`ParseFigletFont(name, data)` parses bytes with an error return. Neither needs
catalog registration. See the [embedding example and renderer constraints](../../docs/guide.md#choosing-and-loading-fonts).

## Provenance and conversion

Sources: [Spleen's author repository](https://github.com/fcambus/spleen),
pinned to revision
[`57f9219328c9f5873085320fe8bc8f7dd34b8791`](https://github.com/fcambus/spleen/tree/57f9219328c9f5873085320fe8bc8f7dd34b8791).
The BDF files identify BSD-2-Clause directly, agreeing with the upstream
[license](https://github.com/fcambus/spleen/blob/57f9219328c9f5873085320fe8bc8f7dd34b8791/LICENSE).

The checked-in [generator](generate.py) verifies SHA-256 hashes of all
four original BDF sources and checks the retained license against
upstream. Each pair of vertical bitmap pixels becomes a `▀`, `▄`, `█`
or blank FIGlet cell. Fixed-width cells preserve the original spacing;
there is no added smushing or kerning.

The shadow variant derives from Spleen 6×12. It offsets the bitmap by one
column right and two pixels down, adding `░` in shadow cells that contain
no foreground ink. Shades are painted as solid cells at reduced opacity,
so `Block.Color`, gradients and solid-cell effects apply to the shadow.
For an independently colored drop shadow, use `Block.Drop`.

## Regenerate

Python 3 is needed only to regenerate assets, not to build or use Decker.
From the repository root:

```sh
git clone https://github.com/fcambus/spleen.git /tmp/decker-spleen-source
git -C /tmp/decker-spleen-source checkout 57f9219328c9f5873085320fe8bc8f7dd34b8791
python3 fonts/figlet/generate.py /tmp/decker-spleen-source
python3 fonts/figlet/generate.py /tmp/decker-spleen-source --check
```

Keep the copyright, BSD conditions and disclaimer in source distributions
and documentation or other accompanying materials for binary releases.
Do not substitute other source revisions without reviewing their license,
updating the source hashes, and reviewing intended rendering changes.

## Migration from the previous fonts

Spleen replaces ANSI Shadow, ANSI Regular, Calvin S, DOS Rebel and Delta
Corps Priest 1. Those files had no explicit redistribution grants and
have been removed. The exported Go variable names remain available, but
their `FigletFont.Name`, glyph measurements, case, punctuation and appearance
change. `FitBlock` recomputes wrapping and scaling for the new faces.
`DropQuotes` now retains ASCII quote marks supported by Spleen.

See the [font inventory](../README.md) for TrueType font licenses.
