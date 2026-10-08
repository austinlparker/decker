# Command-line reference

[README](../README.md) · [Guide](guide.md) · [Troubleshooting](troubleshooting.md)

Every talk that calls `decker.Main` gets these flags. Run the commands from
the talk's `main` package. From the engine repository, the hello example is
`go run ./examples/hello`; `go run .` applies inside your own talk repository.

```sh
go run . -help
go run . -dev
go run . -presenter -length 45m
go run . -presenter -presentation-font-size 5 -previews image
go run . -list
go run . -snapshot -slide 1 -step 2 -t 1.5 -w 240 -h 67 -png frame.png
go run . -review review -sizes 240x67,682x171
go run . -sheet sheet.png -w 682 -h 171 -shrink 8
go run . -video talk.mp4 -slide 1 -until 3 -size 1280x720 -fps 30 -hold 4
```

## Modes

| Flag | Result |
| --- | --- |
| No mode flag | Present interactively in the terminal |
| `-dev` | Present and rebuild on Go source/module changes; preserve slide and step after a successful build |
| `-presenter` | Show notes, timer and previews; `p` opens the deck in a new Ghostty window on macOS |
| `-list` | Print slide titles, build counts and sections, then exit |
| `-review DIR` | Check every build of every slide at several sizes for clipped, dropped, off-canvas or unreadable content; print the issues, write `DIR/index.md` and `DIR/report.json`, and exit non-zero on errors |
| `-snapshot` | Render the selected slide and step as ANSI text to stdout, or PNG with `-png` |
| `-sheet FILE` | Write a four-column PNG contact sheet of all slides at their final step |
| `-video FILE` | Export animation and transitions as a silent H.264 video |

Choose one output mode. If flags are combined, precedence is video,
presenter, list, review, sheet, snapshot, then live presentation. `-dev` only applies
to live presentation, including a deck opened with `p` from the presenter.

## Flags and defaults

| Flag | Default | Applies to / meaning |
| --- | --- | --- |
| `-slide N` | `1` | Live, presenter launch, snapshot, review, video: starting slide, **1-based** |
| `-step N` | `1` | Live, presenter launch, snapshot: starting build step, **1-based** |
| `-fps N` | `60` | Live, video: frames per second; clamped to at least 1 |
| `-t SECONDS` | `1000` (`decker.Settled`) | Snapshot, sheet: time since slide and selected step began |
| `-w N` | `120` | Snapshot, sheet: width in terminal **cells** |
| `-h N` | `36` | Snapshot, sheet: height in terminal **cells**; use `-help` to print usage; `-h` requires a height value |
| `-png FILE` | empty | With `-snapshot`: write PNG instead of ANSI text |
| `-shrink N` | `4` | Sheet: downsample each rendered frame by this factor |
| `-socket PATH` | `os.TempDir()/DECK_NAME.sock` | Live, presenter: socket used to connect the two windows |
| `-length DURATION` | `30m` | Presenter: target talk length; Go duration syntax, e.g. `45m` |
| `-presentation-font-size PT` | `4` | Presenter: positive, finite font size in points for the Ghostty window opened with `p` |
| `-previews MODE` | `auto` | Presenter: `auto` selects images in Ghostty/kitty outside tmux/zellij; `image` forces images; `cells` forces half-block previews |
| `-size WIDTHxHEIGHT` | `1920x1080` | Video: output dimensions in **pixels** |
| `-hold SECONDS` | `4` | Video: time per build step, overridden by positive `Slide.Hold` |
| `-until N` | `0` (end) | Video, review: last included slide, **1-based** |
| `-sizes LIST` | `240x67,320x90,682x171` | Review: frame sizes in **cells**, comma-separated |
| `-strict` | `false` | Review: exit non-zero on warnings as well as errors |

`-sheet`, `-video` and `-png` take filenames. Width/height must be positive;
use even video dimensions for the `yuv420p` H.264 encoder. Register a talk's
own flags before calling `Main`, and avoid these reserved names.

## Steps and time

Go slide code uses **0-based** indexes: `Steps: 2` has `Ctx.Step` 0 and 1,
so `c.Reached(1)` becomes true on the second build. The command line uses
1-based numbers: `-step 2` captures that second build. `Steps: 0` means one
build state.

A snapshot defaults to the **first** step, even at settled time. Pass the
last step explicitly to see the whole slide. Its `Ctx.T` and `Ctx.StepT`
both receive `-t`; it renders one slide without an inter-slide transition.
A contact sheet selects the final step automatically.

## Presenter and dev mode

In Ghostty 1.3+ on macOS, run `-presenter`, then press `p`. The deck opens
in a separate window with the configured `-presentation-font-size`; the
presenter's font stays unchanged. macOS may request Automation permission
for Ghostty. The same executable and working directory are used, including
for `go run`; the launcher preserves the talk's custom flags and `-dev`.
It passes the shared socket explicitly, so differing terminal temp
directories do not separate the two windows.

Pressing `p` while connected or opening does nothing. Retrying during a
disconnect reuses the launched window if it still exists; after it closes,
`p` opens a new one at the last reported slide and step. Quitting the
presenter does not close the deck. Launch errors are shown in the
presenter. Other platforms can start the deck manually in another window.

Use the same `Deck.Name` and `-socket` in both windows. Start either window
first; the presenter waits and reconnects after dev rebuilds. A deck always
opens its socket, even without a presenter. To run two copies, give each
deck/presenter pair a different socket path.

Dev mode needs `go` on `PATH`, watches imported packages in the talk's module,
and writes rebuilt binaries to `.slides/DECK_NAME` under the working directory.
A Go workspace can include a local decker checkout for engine edits.
Edits to embedded images or fonts alone do not trigger a rebuild; save a
watched Go file after changing those assets.

## Review

`-review DIR` draws every build of each slide in range, settled (as it
looks once its animations finish), at every `-sizes` size, and prints one
line per issue: slide and build (1-based), title, the sizes it happens at,
severity, code and message. Then it writes the same issues by slide and
build to `DIR/index.md`, and as JSON to `DIR/report.json` (slides and builds
1-based; `rect` is `[x, y, w, h]` in canvas pixels at that size). It exits
with status 1 when it finds an error, or with `-strict` a warning. Codes a
slide lists in `Slide.Allow` are left out. The guide's
[Reviewing a deck](guide.md#reviewing-a-deck) lists the codes.

## Export behavior

PNG snapshots and contact sheets render terminal cells, including the
character layer. Video reads the pixel canvas directly, so `Scene.Text`,
`Scene.Put` and `Scene.Sprite` content is absent; use `Text`, `Rich` or
`Block` for content needed in both outputs.

Video needs `ffmpeg` on `PATH` with the `libx264` encoder. `-slide` and
`-until` bound the export; it starts every included slide at its first
step. The first step gets transition time in addition to its hold time.
Video has no audio. PNG, sheet and video output replace an existing file.

Source of truth: [`cli.go`](../cli.go), [`present_window.go`](../present_window.go),
[`kitty.go`](../kitty.go), [`png.go`](../png.go),
[`video.go`](../video.go) and [`dev.go`](../dev.go).
