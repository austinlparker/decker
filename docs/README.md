# Documentation

Start with the [project README](../README.md) and its complete first talk.

| Document | Purpose |
| --- | --- |
| [Guide](guide.md) | Rendering, fonts, presentation, slide authoring, toolbox and deck tests |
| [CLI reference](cli.md) | All modes, flags, defaults, indexing and export behavior |
| [Troubleshooting](troubleshooting.md) | Common symptoms and fixes |
| [API reference](https://pkg.go.dev/github.com/austinlparker/decker) | Exported types, functions and methods |
| [Contributing](../CONTRIBUTING.md) | Engine changes, checks, goldens and bug reports |
| [Architecture](architecture.md) | Engine source-file map |
| [Releasing](releasing.md) | Version selection and release automation |
| [Font licenses](../fonts/README.md) | Bundled font inventory, OFL and BSD notices |
| [Block font alternatives](figlet-alternatives.md) | Explicitly licensed candidates, actual renderings and compatibility findings |
| [Documentation review](documentation-review.md) | Comparison with 20 popular Go packages and remaining gaps |

The runnable [hello example](../examples/hello/main.go) is mirrored in the
README. Its preview is generated from the repository root with:

```sh
go run ./examples/hello -snapshot -step 2 -w 240 -h 67 -png docs/assets/hello.png
```

The [Decker icon](assets/decker.svg) is an accessible SVG with a transparent
background; its filled front slide keeps the terminal prompt legible on
light and dark pages. Edit the vector shapes to change its size or palette.
