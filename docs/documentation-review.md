# Documentation review

Reviewed October 6, 2026, against base commit `d739743` and 20 popular Go
libraries/frameworks on GitHub.

Decker's original documentation explained its rendering model and advanced
features well, but required too much reading before a first working talk.
The highest-value changes are a complete first program, explicit requirements,
clear navigation, and a reference that distinguishes cells, pixels, step
indexes and export modes. Those changes are implemented in this revision.

## Baseline and limits

“Top” means GitHub stars, not downloads, dependents, maintenance health or
documentation quality. The baseline includes reusable Go libraries and
application frameworks, including CLI and GUI frameworks. It excludes
standalone applications, language implementations, books, tutorial repositories,
link collections and frameworks whose principal implementation is another
language. CLI-first applications such as Lux and Vegeta were excluded even
when they also offer library APIs; application-building frameworks such as
Wails were included.

Discovery used GitHub's star-sorted [Go topic](https://github.com/topics/go?o=desc&s=stars)
listings (pages 1–6), the [Golang topic](https://github.com/topics/golang?o=desc&s=stars&page=2),
and targeted repository searches. Repository pages supplied the star counts
below. These are the 20 highest-starred eligible packages found in that search,
sorted within this cohort. GitHub's global repository search and search API
were unavailable during review, and topic membership is voluntary, so this
is a **best-effort top-20 package baseline, not a certified global ranking**.
GitHub counts are rounded and retrieved pages may be cached; small differences
near the cutoff should not be treated as significant.

For each project, the README and its linked learning route were sampled:
quick start, guide, examples or package documentation. This is a qualitative
review of first-use clarity, navigation and reference design, not an execution
test of those projects or an audit of every page. Failed page fetches are
listed below rather than interpreted as broken public links.

## What the 20 projects teach us

Each repository link is the README and star-count source. The learning-route
link identifies the accompanying documentation sampled. “Friction” is an
editorial judgment based on the reviewed pages.

| Rank | Package / approximate stars | README and learning route: what works | Friction / lesson for decker |
| --- | --- | --- | --- |
| 1 | [Gin](https://github.com/gin-gonic/gin), 89.3k | Complete first application and expected response; [quick start](https://gin-gonic.com/en/docs/quickstart/) includes module setup and verification. | Large benchmark tables extend the README. The sampled README says Go 1.26+, while the quick start says 1.25+; synchronize requirements with `go.mod`. |
| 2 | [Bubble Tea](https://github.com/charmbracelet/bubbletea), 45.3k | Teaches its model through a concrete program; [package docs](https://pkg.go.dev/charm.land/bubbletea/v2) and examples give the next step. | The tutorial is assembled across several blocks. Provide a complete runnable file alongside the explanation. |
| 3 | [Cobra](https://github.com/spf13/cobra), 44.7k | Clear command/argument/flag model and a detailed [user guide](https://github.com/spf13/cobra/blob/main/site/content/user_guide.md). | Generator and library instructions are separate paths, and the README requires a guide hop to assemble a program. Make the shortest route explicit. |
| 4 | [Fiber](https://github.com/gofiber/fiber), 40.2k | Runnable hello world and prominent limitations; [docs](https://docs.gofiber.io/) grow an app step by step. | README sponsorship and feature sections add scrolling. Sampled requirements differ between README (1.26+) and docs (1.25+); avoid duplicated unsynchronized requirements. |
| 5 | [GORM](https://github.com/go-gorm/gorm), 40.0k | Compact README routes to [guides](https://gorm.io/docs/), which distinguish generics and traditional APIs. | First success depends on leaving GitHub and choosing an API path. Keep a minimal working program in the repository. |
| 6 | [Wails](https://github.com/wailsapp/wails), 36.5k | README separates stable v2 and beta v3; [installation](https://wails.io/docs/gettingstarted/installation/) explains platform dependencies and diagnostics. | Many platform requirements precede building. Decker should state its Unix dependency before installation, with one short path to success. |
| 7 | [go-zero](https://github.com/zeromicro/go-zero), 33.4k | Generation commands, file tree and run/verification steps make its [README walkthrough](https://github.com/zeromicro/go-zero/blob/master/readme.md) inspectable. | Architecture/background and assistant setup precede the quick start. Put reader onboarding ahead of maintainer and tooling detail. |
| 8 | [Echo](https://github.com/labstack/echo), 32.8k | A full program, current import path and a version-support section; [quick start](https://echo.labstack.com/guide/quickstart/) is a distinct route. | Middleware and sponsor inventories compete with first use. Keep the README's resource table task-oriented and short. |
| 9 | [Beego](https://github.com/beego/beego), 32.4k | README walks through directory creation, module initialization, source, dependencies, build and verification; [docs entry](https://beegodoc.com/en-US/) links onward. | Multiple documentation destinations require choosing a source. Give decker one documentation index and one API destination. |
| 10 | [Viper](https://github.com/spf13/viper), 30.5k | [README guide](https://github.com/spf13/viper/blob/master/README.md) organizes configuration sources and behaviors by task. | It is long and mixes basic usage with remote stores. Preserve decker's detailed reference in a guide rather than its landing page. |
| 11 | [Fyne](https://github.com/fyne-io/fyne), 28.7k | Requirements precede a complete GUI; [getting started](https://docs.fyne.io/started/) and reference are separate learning paths. | Native build prerequisites make first use environment-dependent. Clearly distinguish Go, terminal and optional FFmpeg requirements. |
| 12 | [Go kit](https://github.com/go-kit/kit), 27.4k | Explicit goals/non-goals; the [stringsvc tutorial](https://gokit.io/examples/stringsvc.html) grows a service and links complete milestones. | The README favors motivation/ecosystem over first execution. Decker needs its rendering explanation after the quick start. |
| 13 | [Testify](https://github.com/stretchr/testify), 26.2k | Examples map to assert, require, mock and suite; [package docs](https://pkg.go.dev/github.com/stretchr/testify) explain behavior and usage constraints. | A large surface needs clear entry points. Make `decktest` discoverable and include imports in a talk's test example. |
| 14 | [Kratos](https://github.com/go-kratos/kratos), 26.0k | Requirements and migration links are explicit; [quick start](https://go-kratos.dev/docs/getting-started/start/) describes template renaming, generation, tests and verification. | CLI/template/tool versions create several setup steps. Avoid adding a generator when a single Go file is enough. |
| 15 | [Logrus](https://github.com/sirupsen/logrus), 25.8k | Maintenance status appears early; [package comments](https://github.com/sirupsen/logrus/blob/master/doc.go) explain the basic logging model. | Long feature guidance benefits from task navigation. State actual support and status; badges do not establish them. |
| 16 | [Iris](https://github.com/kataras/iris), 25.6k | A compact program is near the top; the [example index](https://github.com/kataras/iris/blob/main/_examples/README.md) provides topic routes. | Current-version instructions share a page with extensive upcoming-version material. Keep stable usage distinct from proposals. |
| 17 | [Colly](https://github.com/gocolly/colly), 25.5k | Concrete callbacks in README; [getting started](https://go-colly.org/docs/introduction/start/) explains collector lifecycle and callback order. | Sponsor content precedes the example, and its code block omits `package main`. Include all program scaffolding in decker's first example. |
| 18 | [Gorilla WebSocket](https://github.com/gorilla/websocket), 24.9k | Short README routes to examples and [package documentation](https://github.com/gorilla/websocket/blob/main/doc.go), including concurrency constraints. | A compact landing page still needs a practical learning path. Pair decker's API link with a runnable talk and guide. |
| 19 | [zap](https://github.com/uber-go/zap), 24.7k | Quick start differentiates its logger APIs; [FAQ](https://github.com/uber-go/zap/blob/master/FAQ.md) explains tradeoffs and common surprises. | Benchmark tables add landing-page bulk. Link reproducible benchmarks instead of advertising unmeasured rendering claims. |
| 20 | [urfave/cli](https://github.com/urfave/cli), 24.3k | Compact README, [versioned getting started](https://cli.urfave.org/v3/getting-started/) and repository-backed docs. | Leaving GitHub is required to reach a first program. Decker can keep a complete quick start in the README and detailed docs alongside it. |

The go-zero introduction URL and two GitHub example-directory URLs could not
be fetched. The review used go-zero's README walkthrough, Bubble Tea's package
docs, and Iris's example-index file instead. Beego's fetched docs landing page
was very short, so its deeper documentation was not independently assessed.
Echo's older `/docs/quick-start` endpoint returned a redirect page; the current
`/guide/quickstart/` route was inspected.

## Shared patterns worth adopting

1. **First success is concrete.** Include module creation, a complete program,
   the run command and an observable result. Gin, Echo and Fyne show the
   pattern; Bubble Tea and Go kit reinforce it with complete source milestones.
2. **A README is an entry point.** Link a guide, API reference, runnable example
   and contributor instructions by task. Detailed catalogs belong in the guide.
3. **Requirements and boundaries are part of onboarding.** Explain platform,
   external tools, indexes and output limitations before readers encounter them.
4. **Reference and explanation serve different jobs.** Teach the canvas and
   frame model in prose; give flag defaults and units in a separate reference.
5. **Trust signals need evidence.** A CI badge should link to the actual
   workflow; a version badge should link to tags; license and coverage claims
   need a declared license and coverage service.

Patterns to avoid: sponsor/promotional material before first use, benchmark
walls without context, undefined helper functions, conflicting version
requirements, and release-maintainer instructions mixed into slide authoring.

## Decker before and after

Scope: the original README (about 500 lines), `doc.go`, public API comments,
`decktest` comments, `AGENTS.md`, its `CLAUDE.md` include, font provenance and
license notices, workflows and release script. Behavior was cross-checked with
`cli.go`, `deck.go`, `ctx.go`, `slide.go`, `font.go`, `dev.go`, `link.go`,
`model.go`, `png.go`, `video.go` and the test helpers. Scores below are
qualitative findings, not a measured ranking or a user study.

| Area | Original finding | Implemented result |
| --- | --- | --- |
| Purpose and rendering explanation | Strong: distinct terminal canvas, two font systems and frame purity | Retained in the guide; landing page states the purpose immediately |
| First run | Installation only; examples depend on undefined `talk`, theme and title helpers | Complete program mirrored in `examples/hello`, commands and expected output |
| Requirements | Go floor and Unix dependencies absent; FFmpeg requirement incomplete | Go 1.27.0, Linux CI scope, native Windows limitation, terminal capabilities, libx264 |
| Navigation | One long file combines users, reference and maintainers | README task map, docs index, guide, CLI, troubleshooting, contribution, architecture and release pages |
| Visual identity | No icon or actual rendered preview | Editable SVG and a PNG generated from the runnable example |
| Status signals | No badges or direct API route | Shields for main-branch CI, `go.mod` requirement, semver tag, API reference and MIT code license |
| CLI coverage | Useful commands, but missing a complete flag/default/unit reference | Every registered flag, mode precedence, step/time behavior, file replacement and export differences |
| Step semantics | CLI numbering described; `Steps` prose could be read as number of next presses | Explicit 0-based Go states vs 1-based CLI; `Steps: 2` explained |
| Snapshot/video expectations | Settled time could suggest all builds; native text omitted from video was easy to miss | Selected-step behavior and character-layer export differences stated prominently |
| Package documentation | Useful type map but entry snippet uses undefined `talk`; `Scene` description ambiguous | Self-contained minimal function, learning links, required fonts and clearer scene/canvas description |
| API comment coverage | Alignment constants use trailing remarks; `HideChrome` has no explanation | Named comments for `Left`, `Center`, `Right`, and the dev-footer meaning of `HideChrome` |
| Testing | Strong goldens and benchmark explanation; example omitted imports | Full talk test-file scaffolding, golden-change rules and runnable-example validation |
| Contribution/help | Agent-oriented invariants but no conventional contributor entry point | `CONTRIBUTING.md`, reproducible bug-report guidance and symptom/fix table |
| Release accuracy | Implied immediate proxy/pkg.go.dev availability; described CI as every push | Actual main-branch push scope and asynchronous discovery clarified |
| Licensing | No engine license; FIGlet provenance lacks stated licenses | MIT added for Decker's code, docs, examples and icon; three TTFs confirmed OFL 1.1; previous FIGlet files replaced by BSD-licensed Spleen conversions, documented in the [font inventory](../fonts/README.md) |

The SVG uses stacked slide outlines and a `>` prompt with cursor, linking the
project's two core ideas. It has a viewBox, title and description, no embedded
fonts, raster data, scripts or external resources. Cyan and amber match the
hello example, and the filled front slide keeps the prompt legible in both
GitHub themes.

## Priorities and remaining decisions

| Priority | Finding | Status / next action |
| --- | --- | --- |
| High | New users cannot run the first example | Fixed: complete README program and runnable example |
| High | Requirements, index conventions and export boundaries are easy to miss | Fixed: README requirements, CLI reference and troubleshooting |
| High | Engine source has no declared license | Fixed: maintainer selected MIT; root license covers Decker's code, docs, icon and example, with third-party fonts excluded |
| High | Bundled FIGlet files lack explicit license grants | Fixed: removed the five files after investigating their missing grants; replaced them with Spleen conversions carrying explicit BSD-2-Clause notices, pinned source hashes and a reproducible [generator](../fonts/figlet/README.md) |
| Medium | Detailed user and maintainer material competes for attention | Fixed: docs split with preserved guide content and contributor instructions |
| Medium | Visual/API entry points are absent | Fixed: real preview, SVG and five evidence-backed badges |
| Medium | Font choice and consumer font loading need clear entry points | Fixed: ten bundled FIGlet choices, catalog discovery, filesystem and bytes-based custom loading, executable examples and retained BSD/OFL notices; canonical `FigletFont` names retain deprecated `FigFont` aliases for compatibility |
| Medium | Platform claims exceed the current CI evidence if generalized | Documented: Linux CI; Unix requirements; no claim of tested macOS or native Windows support. Add CI coverage if expanding support |
| Low | Documentation copies may drift | README example is checked against source during this review; contributor instructions identify the two copies. Consider a lightweight drift check when the example changes frequently |
| Low | Advanced recipes are mostly snippets | Guide retains them and labels the template context; add focused complete example decks as users request them |

## Validation

Validation completed:

- README program matches `examples/hello/main.go` exactly. Both it and the
  package-comment example compiled and rendered in a separate talk module
  using a local `replace` and the checkout's dependency checksums. Remote
  `go get @latest` was not exercised.
- `-list`, `-help`, first/final-step PNG snapshots and contact-sheet export
  succeeded. FFmpeg produced a six-frame H.264 video at 320×180.
- The actual hello preview and the SVG rasterized with librsvg/Cairo were
  visually inspected. SVG structure and accessibility text were checked.
- 79 local links/anchors, code-fence balance and README/example equality
  passed. CLI flags/defaults were compared with the generated usage output.
- `gofmt -l .`, `git diff --check`, `go mod tidy`, module-file comparison,
  `go vet ./...` and `go test ./...` passed. The first sandboxed test run
  blocked Unix sockets; the full rerun with socket access passed.
- The initial documentation pass left goldens and module files unchanged.
  The subsequent authorized Spleen replacement intentionally changes stock
  font names, glyphs and measurements, while preserving exported identifiers.
  `go.mod`, `go.sum`, TrueType assets and rendering implementation are unchanged.
- License follow-up: all three TTFs' embedded metadata and both OFL notices
  agree with the upstream OFL grants. The original five FIGlet files had no explicit grants and were replaced
  with Spleen conversions under BSD-2-Clause. Source hashes, retained
  notices, regeneration and printable ASCII coverage were checked.
  TrueType files and existing OFL notices are unchanged.
- Spleen migration: six gallery keys changed (`Block_stock_A`,
  `Block_stock_B`, `Block_options`, `Block_effects_A`, `Block_effects_B`,
  `FigFont_API`). The other 86 gallery hashes are unchanged. Of 2,125
  internal hashes, 809 change where the block-letter fixture appears in
  transitions, video, PNGs, model views, presenter previews and terminal
  output; no keys were added or removed. The new stock font preview and
  affected gallery rendering were visually inspected.
- Optional display fonts: five OFL conversions are bundled alongside the
  Spleen defaults. Full copyright/license headers, exact source hashes and
  reproducible generation were checked. Each printable ASCII glyph paints
  through Decker; consumer fonts work through filesystem and bytes loaders.
  Five `51.*:Block_font_catalog` gallery keys were added, and `24.1:Position`
  changed because the gallery now has 51 slides. Every other existing
  gallery hash and all internal hashes were unchanged by this addition.
- Naming follow-up: `FigletFont`, `LoadFigletFont`, `ParseFigletFont`,
  `StockFigletFont` and `StockFigletFontNames` are the canonical API.
  Deprecated aliases preserve the shorter spellings and typed consumer
  function references. Examples and documentation use the canonical names;
  the historical gallery title remains to preserve its rendering hashes.
  The full tests, vet, formatting, module-file and font regeneration checks
  passed, with both golden files unchanged by the naming update.

Remote badge-image availability and hosted package indexing are separate from
repository validation. The badges use documented Shields routes and link to
the checked-in workflow, module file, tags, package URL and MIT license.
The license badge explicitly describes Decker's code; third-party font terms
are documented separately. The badges do not claim coverage, a quality grade
or cross-platform certification.
