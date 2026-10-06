# Command-line reference

[README](../README.md) · [Guide](guide.md) · [Troubleshooting](troubleshooting.md)

Every talk that calls `decker.Main` gets these flags. Run the commands from
the talk's `main` package. From the engine repository, the hello example is
`go run ./examples/hello`; `go run .` applies inside your own talk repository.

```sh
go run . -help
go run . -dev
go run . -presenter -length 45m
go run . -list
go run . -snapshot -slide 1 -step 2 -t 1.5 -w 240 -h 67 -png frame.png
go run . -sheet sheet.png -w 682 -h 171 -shrink 8
go run . -video talk.mp4 -slide 1 -until 3 -size 1280x720 -fps 30 -hold 4
```

## Modes

| Flag | Result |
| --- | --- |
| No mode flag | Present interactively in the terminal |
| `-dev` | Present and rebuild on Go source/module changes; preserve slide and step after a successful build |
| `-presenter` | Open notes, timer and previews in a second terminal window |
| `-list` | Print slide titles, build counts and sections, then exit |
| `-snapshot` | Render the selected slide and step as ANSI text to stdout, or PNG with `-png` |
| `-sheet FILE` | Write a four-column PNG contact sheet of all slides at their final step |
| `-video FILE` | Export animation and transitions as a silent H.264 video |

Choose one output mode. If flags are combined, precedence is video,
presenter, list, sheet, snapshot, then live presentation. `-dev` only applies
to live presentation.

## Flags and defaults

| Flag | Default | Applies to / meaning |
| --- | --- | --- |
| `-slide N` | `1` | Live, snapshot, video: starting slide, **1-based** |
| `-step N` | `1` | Live, snapshot: starting build step, **1-based** |
| `-fps N` | `60` | Live, video: frames per second; clamped to at least 1 |
| `-t SECONDS` | `1000` (`decker.Settled`) | Snapshot, sheet: time since slide and selected step began |
| `-w N` | `120` | Snapshot, sheet: width in terminal **cells** |
| `-h N` | `36` | Snapshot, sheet: height in terminal **cells**; use `-help` to print usage; `-h` requires a height value |
| `-png FILE` | empty | With `-snapshot`: write PNG instead of ANSI text |
| `-shrink N` | `4` | Sheet: downsample each rendered frame by this factor |
| `-socket PATH` | `os.TempDir()/DECK_NAME.sock` | Live, presenter: socket used to connect the two windows |
| `-length DURATION` | `30m` | Presenter: target talk length; Go duration syntax, e.g. `45m` |
| `-size WIDTHxHEIGHT` | `1920x1080` | Video: output dimensions in **pixels** |
| `-hold SECONDS` | `4` | Video: time per build step, overridden by positive `Slide.Hold` |
| `-until N` | `0` (end) | Video: last included slide, **1-based** |

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

Use the same `Deck.Name` and `-socket` in both windows. Start either window
first; the presenter waits and reconnects after dev rebuilds. A deck always
opens its socket, even without a presenter. To run two copies, give each
deck/presenter pair a different socket path.

Dev mode needs `go` on `PATH`, watches imported packages in the talk's module,
and writes rebuilt binaries to `.slides/DECK_NAME` under the working directory.
A Go workspace can include a local decker checkout for engine edits.
Edits to embedded images or fonts alone do not trigger a rebuild; save a
watched Go file after changing those assets.

## Export behavior

PNG snapshots and contact sheets render terminal cells, including the
character layer. Video reads the pixel canvas directly, so `Scene.Text`,
`Scene.Put` and `Scene.Sprite` content is absent; use `Text`, `Rich` or
`Block` for content needed in both outputs.

Video needs `ffmpeg` on `PATH` with the `libx264` encoder. `-slide` and
`-until` bound the export; it starts every included slide at its first
step. The first step gets transition time in addition to its hold time.
Video has no audio. PNG, sheet and video output replace an existing file.

Source of truth: [`cli.go`](../cli.go), [`png.go`](../png.go),
[`video.go`](../video.go) and [`dev.go`](../dev.go).
