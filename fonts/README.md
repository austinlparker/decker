# Bundled fonts and licenses

Decker's own code, documentation, examples and SVG icon use the root
[MIT license](../LICENSE). The third-party font files keep their own terms;
MIT does not relicense them.

## TrueType fonts: SIL OFL 1.1

All three bundled TrueType files declare **SIL Open Font License 1.1** in
their embedded license metadata. Their embedded copyright statements match
the existing notices in this directory.

| File | Embedded version | Copyright holder | License notice |
| --- | --- | --- | --- |
| `SpaceGrotesk-Bold.ttf` | 2.000 | Copyright 2020 The Space Grotesk Project Authors | [SpaceGrotesk-OFL.txt](SpaceGrotesk-OFL.txt) |
| `SpaceGrotesk-Medium.ttf` | 2.000 | Copyright 2020 The Space Grotesk Project Authors | [SpaceGrotesk-OFL.txt](SpaceGrotesk-OFL.txt) |
| `JetBrainsMono-ExtraBold.ttf` | 2.305 | Copyright 2020 The JetBrains Mono Project Authors | [JetBrainsMono-OFL.txt](JetBrainsMono-OFL.txt) |

Upstream confirmation:

- [Space Grotesk](https://github.com/floriankarsten/space-grotesk) and its
  [OFL notice](https://github.com/floriankarsten/space-grotesk/blob/master/OFL.txt).
- [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) and its
  [OFL notice](https://github.com/JetBrains/JetBrainsMono/blob/master/OFL.txt).
  The upstream project's separate Apache license for source code does not
  replace the OFL license for these font binaries.

OFL permits using, embedding, modifying and redistributing fonts, including
bundling them with commercial software. Retain the copyright and full OFL
notices when redistributing these files or a program that embeds them.
`StockFont` embeds the TrueType files in the executable, so distribute the
applicable notices with that executable as well as with source releases.

The fonts cannot be sold by themselves and must remain under OFL. Modified
fonts must also follow OFL's Reserved Font Name conditions where applicable.
Slides and images made with the fonts do not inherit the font license.
See the notices above for the complete terms.

## Default FIGlet fonts: BSD-2-Clause

The five stock block fonts are generated from **Spleen 2.2.0** by Frederic
Cambus under **BSD-2-Clause**, including a derived shaded variant. They
replace the previous files whose redistribution permissions could not be
established. All bundled font files now have explicit license grants.

See the [per-file inventory, provenance and generator](figlet/README.md)
and [full Spleen notice](figlet/Spleen-LICENSE.txt). Every generated `.flf`
also includes that notice in its comment header. Retain the notice in
source and accompanying materials when distributing binaries.

## Optional FIGlet fonts: SIL OFL 1.1

The bundled catalog also contains five half-block conversions: **Decker
Sign** (Bungee), **Decker Geometric** (Rubik Mono One), **Decker Stencil**
(Black Ops One), **Decker Slab** (Alfa Slab One) and **Decker Square**
(Russo One). They are optional faces selected through `StockFigletFont`;
the Spleen defaults remain unchanged. `StockFigletFontNames` lists all ten.

These converted font files remain under **SIL OFL 1.1**. Their own source
copyright and full licenses are embedded in every `.flf` header, and
separate notices are retained in `fonts/figlet/`. Preserve those notices
when redistributing source or binaries that embed the fonts. Modified
font names respect the source fonts' Reserved Font Names; the original
design names are attribution, not the converted fonts' primary names.
See the [inventory and reproducible converter](figlet/README.md#optional-display-fonts).

Consumers may supply their own FIGlet or smooth fonts through the
[font loading APIs](../docs/guide.md#choosing-and-loading-fonts). They retain
their own license terms and need not be added to Decker's stock catalog.
