# Troubleshooting

[README](../README.md) · [Guide](guide.md) · [CLI reference](cli.md)

| Symptom | What to check |
| --- | --- |
| `go run .` says the package is not a main package | Decker is a library. Run `go run ./examples/hello` in this checkout, or run `go run .` in your talk's directory. |
| The toolchain is too old | This checkout requires Go 1.27.0 or later. Check `go version` and the talk's `go.mod`; `GOTOOLCHAIN=local` prevents automatic toolchain download. |
| Missing `Theme`, fonts or slides | Set `Deck.Name`, a non-nil `Theme` with `Display`, `Body`, `Mono`, and at least one slide. See the [complete example](../examples/hello/main.go). |
| Font loading panics | Call `StockFont` with a bundled font name, or ensure the file passed to `LoadFont` exists in its filesystem and is a valid TTF/OTF. Load fonts once, outside `View`. |
| Native Windows compilation fails | The engine uses `syscall.SIGWINCH` and `syscall.Exec`; CI checks Linux. Use a Linux environment such as WSL. macOS uses the same Unix primitives but is not covered by the current CI matrix. |
| Text is jagged, tiny or clipped | Use a true-color terminal with Unicode half-block support, increase the cell count by reducing its font size, and fit text to a box with `Font.Fit`. Run `-review` to find every build that clips or shrinks text at small and projector sizes. |
| Code, table rows or a diagram's text are cut off | Run `go run . -review review`: it names the slide, build, size and element, and how much is missing (`Code "main.go": 5 of 10 lines visible`). Give the element a bigger rect, show fewer lines, or split it across builds. `Code.Measure` tells you what fits before drawing. |
| Animation lags | Try `-fps 30`, run directly in the terminal, and let large-area animations settle. Benchmark the talk with `decktest.Frames`; measure the engine with `go test -run '^$' -bench Live`. |
| Presenter waits forever | Start the deck in the other window. Both processes must use the same deck name and socket path and be able to access the same filesystem socket. |
| “another deck is already running” | Quit the other deck or give the new deck and its presenter a matching, different `-socket` path. The engine replaces stale sockets itself. |
| Dev mode does not reload | Run from the talk's main-package directory and keep `go` on `PATH`. Imported same-module Go packages are watched; an embedded asset change needs a Go-file save to trigger rebuilding. |
| Dev mode shows a red panel | Read the compiler error, fix the source and save again. The previously built deck keeps running until a build succeeds. |
| Snapshot lacks a later build | `-step` defaults to 1. Use the final step number from `-list`; `-t` alone does not advance builds. |
| PNG contains an error instead of the slide | Rendering catches `View`/placed-element/overlay panics and draws them. A successful export does not prove the slide was valid; inspect the image, and run `-review` or `decktest.Review`, which list every panic. |
| Text disappears in video | Video excludes native character-layer content. Draw required text on the pixel canvas with `Text`, `Rich` or `Block`. |
| `ffmpeg` export fails | Check `ffmpeg -version`, the `libx264` encoder, an output directory that exists and is writable, and positive even `-size` dimensions. |
| Replay, snapshots and video disagree | Keep drawing a pure function of `Ctx`. Use `c.T`, `c.StepT`, `c.Since` and `Hash01`; keep clocks, mutable global state and nondeterministic random calls outside frames. |
| Terminal has stale cells | Press `ctrl+l` to redraw. Avoid logging to the terminal while the live deck owns it; use a file. |

For a reproducible engine bug, follow the information checklist in
[CONTRIBUTING.md](../CONTRIBUTING.md) and open an
[issue](https://github.com/austinlparker/decker/issues).
