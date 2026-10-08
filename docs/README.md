# Documentation

Start with the [project README](../README.md) and its complete first talk.

| Document | Purpose |
| --- | --- |
| [Authoring a talk](authoring.md) | For whoever writes the slides, person or agent: what to build yourself, what not to, and the review loop; start from the [starter](../examples/starter) |
| [Guide](guide.md) | Rendering, fonts, presentation, slide authoring, toolbox and deck tests |
| [CLI reference](cli.md) | All modes, flags, defaults, indexing and export behavior |
| [Troubleshooting](troubleshooting.md) | Common symptoms and fixes |
| [API reference](https://pkg.go.dev/github.com/austinlparker/decker) | Exported types, functions and methods |
| [Contributing](../CONTRIBUTING.md) | Engine changes, checks, goldens and bug reports |
| [Architecture](architecture.md) | Engine source-file map |
| [Releasing](releasing.md) | Version selection and release automation |
| [Font licenses](../fonts/README.md) | Bundled font inventory, OFL and BSD notices |

The runnable [hello example](../examples/hello/main.go) is mirrored in the
README. Its preview is generated from the repository root with:

```sh
go run ./examples/hello snapshot --step 2 -w 240 -h 67 --png docs/assets/hello.png
```

Two more examples are for talk authors. The [starter](../examples/starter) is a
talk to copy, with an `AGENTS.md` for its repository. The
[recipes](../examples/recipes) are visuals built from primitives, starting
with a trace waterfall, to copy and change.
Both review clean with `go run ./examples/starter review review`.

The README's example clips come from the [showcase example](../examples/showcase/main.go).
Each clip is a range of its slides, exported with `video` and converted to
a GIF (needs ffmpeg with libx264):

```sh
sh examples/showcase/record.sh
```

The [Decker icon](assets/decker.svg) is an accessible SVG with a transparent
background; its filled front slide keeps the terminal prompt legible on
light and dark pages. Edit the vector shapes to change its size or palette.
