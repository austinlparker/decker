# Releasing

Talks depend on decker by version, so a release is a semver tag and a GitHub
Release; there are no binaries. It all happens on its own:

- **CI** ([ci.yml](../.github/workflows/ci.yml)) runs on pushes to `main` and every pull request:
  gofmt, `go mod tidy`, vet and the full test suite, goldens included.
- **Release** ([release.yml](../.github/workflows/release.yml)) runs on every push to `main`
  and once a day. If the library changed since the last tag (not just tests,
  goldens, docs or CI), it works out the next version from the exported API
  with `apidiff` ([next-version.sh](../.github/scripts/next-version.sh)): a breaking change bumps
  the major version (the minor one while decker is v0), an addition the minor,
  anything else the patch. Then it runs the tests again, tags, and
  [GoReleaser](https://goreleaser.com) ([.goreleaser.yaml](../.goreleaser.yaml)) publishes the
  release: the API diff, then the pull requests merged since the last tag,
  grouped by label (`.github/release.yml`: `breaking`, `bug` or `fix`, the
  rest; `skip-changelog` leaves one out).
- **Dependencies** ([dependabot.yml](../.github/dependabot.yml)) are updated weekly, one grouped
  pull request for Go modules and one for Actions. Patch and minor updates
  merge themselves when CI passes ([dependabot workflow](../.github/workflows/dependabot.yml)); the
  next daily run releases them.

To hold a merge back, put `[skip release]` in its title (the PR title, for a
squash merge); the next release picks it up. To release now, or as a version
of your choosing (a `v1.0.0`, a `-rc.1`), run the Release workflow by hand
from the Actions tab.

The selection script checks `*.go`, `go.mod`, `go.sum` and `fonts/**`,
excluding `*_test.go` and `testdata/**`. Go comment edits and new example
programs therefore count toward a release even when the exported API is
unchanged; they normally produce a patch bump. Markdown under `docs/` alone
does not trigger one, while font documentation under `fonts/` does.

Each release ends by asking the Go module proxy for the new version, so
`go get github.com/austinlparker/decker@latest` can discover it sooner. This warms the module proxy; pkg.go.dev indexing
is separate and availability is not guaranteed to be immediate.

See [contributing](../CONTRIBUTING.md) for checks before a release.
