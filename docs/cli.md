# Command-line reference

[README](../README.md) · [Guide](guide.md) · [Troubleshooting](troubleshooting.md)

Every talk that calls `decker.Main` gets these commands. Run them from the
talk's `main` package. From the engine repository, the hello example is
`go run ./examples/hello`; `go run .` applies inside your own talk repository.
`--help` lists the commands, and `COMMAND --help` a command's flags.

```sh
go run . --help
go run . --dev
go run . present --length 45m
go run . present --presentation-font-size 5 --previews image
go run . list
go run . review review
go run . handout handout -w 160 -h 45
go run . snapshot --slide 1 --step 2 -t 1.5 -w 240 -h 67 --png frame.png
go run . sheet sheet.png -w 682 -h 171 --shrink 8
go run . video talk.mp4 --slide 1 --until 3 --size 1280x720 --fps 30 --hold 4
```

## Commands

| Command | Result |
| --- | --- |
| none, or `live` | Present interactively in the terminal; `--dev` rebuilds on Go source or module changes and keeps the slide and build after a successful build |
| `present` | Show notes, timer and previews; `p` opens the deck in a new Ghostty window on macOS |
| `list` | Print slide titles, build counts and sections |
| `review DIR` | Check every build of every slide at 240×67, 320×90 and 682×171 for clipped, dropped, off-canvas, unreadable or overlapping content; print the issues, write `DIR/index.md` with pictures, and exit 1 on errors |
| `handout DIR` | Write `DIR/handout.md` and a PNG thumbnail of every slide at its final build |
| `snapshot` | Render one slide and build as ANSI text to stdout, or as a PNG with `--png` |
| `sheet FILE` | Write a four-column PNG contact sheet of all slides at their final build |
| `video FILE` | Export animation and transitions as a silent H.264 video |

Flags go after the command, and each command takes only its own: a flag for
another command, a second command, a missing `DIR` or `FILE`, or a value out
of range is an error, with exit status 80, and the command's usage. Flags
alone run the deck live, so `go run . --dev` needs no `live`. Long flags take
two dashes; `-w`, `-h` and `-t` are the short forms of `--width`,
`--height` and `--time`. An argument in the old single-dash form, such as
`-review`, is an error that says so.

## Flags and defaults

| Flag | Default | Commands: meaning |
| --- | --- | --- |
| `--slide N` | `1` | live, present, snapshot: slide to start at, **1-based**; video: first slide |
| `--step N` | `1` | live, present, snapshot: build to start at, **1-based** |
| `--fps N` | `60` | live, present (for the deck `p` opens), video: frames per second, at least 1 |
| `--dev` | off | live, present (for the deck `p` opens): rebuild and reload on save |
| `--socket PATH` | `os.TempDir()/DECK_NAME.sock` | live, present: socket linking the two windows |
| `-t`, `--time SECONDS` | `1000` (`decker.Settled`) | snapshot, sheet, handout: time since the slide and its build began |
| `-w`, `--width N` | `120` | snapshot, sheet, handout: width in terminal **cells**, positive |
| `-h`, `--height N` | `36` | snapshot, sheet, handout: height in terminal **cells**, positive |
| `--png FILE` | none | snapshot: write a PNG instead of ANSI text |
| `--shrink N` | `4` | sheet: downsample each frame by this factor, at least 1 |
| `--length DURATION` | `30m` | present: the talk's length; Go duration syntax, e.g. `45m` |
| `--presentation-font-size PT` | `4` | present: positive, finite font size in points for the Ghostty window opened with `p` |
| `--previews MODE` | `auto` | present: `auto` selects images in Ghostty/kitty outside tmux/zellij; `image` forces images; `cells` forces half-block previews |
| `--size WIDTHxHEIGHT` | `1920x1080` | video: output size in **pixels** |
| `--hold SECONDS` | `4` | video: time per build, overridden by a positive `Slide.Hold` |
| `--until N` | end | video: last slide, **1-based** |

Use even video dimensions for the `yuv420p` H.264 encoder. `--help` prints
help; `-h` is a height.

## Steps and time

Go slide code uses **0-based** indexes: `Steps: 2` has `Ctx.Step` 0 and 1,
so `c.Reached(1)` becomes true on the second build. The command line uses
1-based numbers: `--step 2` captures that second build. `Steps: 0` means one
build state.

A snapshot defaults to the **first** step, even at settled time. Pass the
last step explicitly to see the whole slide. Its `Ctx.T` and `Ctx.StepT`
both receive `-t`; it renders one slide without an inter-slide transition.
A contact sheet and a handout select the final step automatically.

## Presenter and dev mode

In Ghostty 1.3+ on macOS, run `present`, then press `p`. The deck opens
in a separate window with the configured `--presentation-font-size`; the
presenter's font stays unchanged. macOS may request Automation permission
for Ghostty. The same executable and working directory are used, including
for `go run`; the deck runs with the presenter's `--fps` and `--dev`.
It passes the shared socket explicitly, so differing terminal temp
directories do not separate the two windows.

Pressing `p` while connected or opening does nothing. Retrying during a
disconnect reuses the launched window if it still exists; after it closes,
`p` opens a new one at the last reported slide and step. Quitting the
presenter does not close the deck. Launch errors are shown in the
presenter. Other platforms can start the deck manually in another window.

Use the same `Deck.Name` and `--socket` in both windows. Start either window
first; the presenter waits and reconnects after dev rebuilds. A deck always
opens its socket, even without a presenter. To run two copies, give each
deck/presenter pair a different socket path.

Dev mode needs `go` on `PATH`, watches imported packages in the talk's module,
and writes rebuilt binaries to `.slides/DECK_NAME` under the working directory.
A Go workspace can include a local decker checkout for engine edits.
Edits to embedded images or fonts alone do not trigger a rebuild; save a
watched Go file after changing those assets.

## Review

`review DIR` draws every build of every slide, settled (as it looks once
its animations finish), at 240×67, 320×90 and 682×171 cells, and prints one
line per issue: slide and build (1-based), title, the sizes it happens at,
severity, code and message. Then it writes the same issues by slide and
build to `DIR/index.md`. It exits with status 1 when it finds an error.
Codes a slide lists in `Slide.Allow` are left out. The guide's
[Reviewing a deck](guide.md#reviewing-a-deck) lists the codes.

It also writes images, linked from `index.md`: `DIR/frames/NN-B-WxH.png`
for each build with issues, the frame from its pixel canvas scaled up with
what it drew outlined and its issues boxed, numbered and listed; and
`DIR/sheet-WxH.png` for each size, every build framed in the color of its
worst issue. The images omit character-layer text, as video does.

## Export behavior

PNG snapshots and contact sheets render terminal cells, including the
character layer. Video reads the pixel canvas directly, so `Scene.Text`,
`Scene.Put` and `Scene.Sprite` content is absent; use `Text`, `Rich` or
`Block` for content needed in both outputs.

Video needs `ffmpeg` on `PATH` with the `libx264` encoder. `--slide` and
`--until` bound the export; it starts every included slide at its first
step. The first step gets transition time in addition to its hold time.
Video has no audio. PNG, sheet and video output replace an existing file.

`handout DIR` creates `DIR` if needed and writes `handout.md` and one
thumbnail per slide, named `01.png`, `02.png`… (padded to the slide count's
digits, at least two). Each thumbnail is the slide's final build, rendered
like `snapshot --png` at `-w`×`-h` and `-t`. The Markdown has the deck's
name as its heading, the slide and build counts, then for each slide a
`## N. Title` heading, its section and build count in italics, the
thumbnail, its notes as written and a list of its `Slide.Sources`, linked
when they have a URL. A closing `## Sources` section lists every source once,
merged by URL (by label without one), with the slides that cite it; a deck
with no sources has no such section. The output depends only on the deck and
the flags. Existing files of the same names are replaced; others in `DIR`
are left alone.

Source of truth: [`cli.go`](../cli.go), [`present_window.go`](../present_window.go),
[`kitty.go`](../kitty.go), [`handout.go`](../handout.go), [`png.go`](../png.go),
[`video.go`](../video.go) and [`dev.go`](../dev.go).
