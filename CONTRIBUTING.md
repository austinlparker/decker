# Contributing to decker

Bug reports, documentation fixes and code contributions are welcome. Use the
[issue tracker](https://github.com/austinlparker/decker/issues) for bugs and
feature requests, and open a pull request for a change.

For a rendering bug, include the Go version, operating system, terminal and
cell dimensions, the decker version, and a small deck that reproduces it.
Include the command and slide/step/time for snapshot or export failures.

## Working on the engine

Use Go **1.27.0 or later**, as required by [go.mod](go.mod). CI currently
checks Linux; the live engine uses Unix signals and dev reload uses Unix
process replacement. Read [AGENTS.md](AGENTS.md) for the rendering invariants
and extension recipes, and [the file map](docs/architecture.md) to find the
implementation.

Run these checks from the repository root:

```sh
gofmt -l .
for m in . bsky examples/showcase; do (cd $m && go mod tidy && go vet ./... && go test ./...); done
git diff --exit-code -- ':(glob)**/go.mod' ':(glob)**/go.sum'
```

`bsky/` and `examples/showcase/` are modules of their own, so `./...` from
the root leaves them out. The showcase posts a clip, so its tests need
ffmpeg. Its
`go.mod` replaces the engine with `../`, so a change to both is tested
together; when bsky starts using engine API that isn't released yet, raise
the engine version it requires to the one that release will be (the next
section's rules say which), or bsky's next release fails its check.

`gofmt -l .` should print nothing. `go test -short ./...` skips the slower
reference tests and is useful during edits, but run the full suite before a PR.
CI also checks the GoReleaser configuration.

## Rendering and API changes

Frames must depend only on `Ctx`; use `Hash01` for noise and `Ctx.T` or
`Ctx.StepT` for time.
Before 1.0 the exported API changes in place: when a name, signature or
behavior should change, change it, update every caller in the repository, and
call out the break in the PR. Don't keep the old form compiling beside the
new one: aliases, `Deprecated:` declarations, renaming vars and forwarding
functions fail `TestNoCompatShims` in `shim_test.go`.

Reference images in `testdata` pin how slides and the engine's screens look,
within a tolerance that absorbs the rounding differences between machines,
and reference text pins the character layer, escape sequences and traces
exactly. Do not regenerate them to hide a failure; a failure leaves what it
drew and a diff image where it says. For an intentional visual change, run:

```sh
UPDATE_GOLDEN=1 go test ./...
git diff -- testdata
```

Only the references that changed are rewritten. Look at every changed image
and explain the intended output change in the PR. A refactor should leave
`testdata` unchanged. New drawing APIs belong
in [the gallery](gallery_test.go), their Go comments, and the
[guide's toolbox](docs/guide.md#toolbox).

For changes to per-frame rendering, measure `go test -run '^$' -bench Live`
before and after. Describe exported API changes in the PR because the release
workflow uses the API diff to choose the next version.

## Documentation changes

Keep the [README](README.md), [guide](docs/guide.md), [CLI reference](docs/cli.md)
and Go comments aligned with the implementation. The README's complete first
program is [examples/hello/main.go](examples/hello/main.go); update both copies
together, then run the example with `list` and a PNG snapshot. Keep badge
claims tied to checked-in configuration or an actual service.

See [releasing](docs/releasing.md) for the automation and versioning rules.
