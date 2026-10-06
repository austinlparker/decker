#!/usr/bin/env bash
# next-version.sh decides whether HEAD needs a release, and which version.
#
# It prints "version=vX.Y.Z", or "version=" when there is nothing to release,
# and writes the API changes since the last release to the file named by
# $NOTES (default api-changes.md) for the release notes. The bump comes from
# the exported API, compared with apidiff against the last tag:
#
#   incompatible changes  major (minor while at v0)
#   compatible additions  minor
#   no API change         patch
#
# Only library files count: a change to tests, goldens, docs or CI alone
# releases nothing. Needs apidiff on PATH and a checkout with full history and
# tags. Run it from the module root.
set -euo pipefail

notes=${NOTES:-api-changes.md}
module=$(go list -m)

last=$(git tag --merged HEAD --list 'v*' --sort=-v:refname |
	grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -n1 || true)
if [ -z "$last" ]; then
	printf '## First release\n' >"$notes"
	echo "version=v0.1.0"
	exit 0
fi

changed=$(git diff --name-only "$last" HEAD -- '*.go' go.mod go.sum 'fonts/**' ':!*_test.go' ':!testdata/**')
if [ -z "$changed" ]; then
	echo "nothing to release: no library changes since $last" >&2
	echo "version="
	exit 0
fi

base=$(mktemp -d)
trap 'git worktree remove --force "$base" >/dev/null 2>&1 || true' EXIT
git worktree add --quiet --detach "$base" "$last"
(cd "$base" && apidiff -m -w "$base/api.export" "$module")
apidiff -m "$base/api.export" "$module" >"$base/api.txt"

IFS=. read -r major minor patch <<<"${last#v}"
if grep -q '^Incompatible changes:' "$base/api.txt"; then
	if [ "$major" -eq 0 ]; then
		next="v0.$((minor + 1)).0"
	else
		next="v$((major + 1)).0.0"
	fi
elif grep -q '^Compatible changes:' "$base/api.txt"; then
	next="v$major.$((minor + 1)).0"
else
	next="v$major.$minor.$((patch + 1))"
fi

{
	echo "## API changes since $last"
	echo
	if [ -s "$base/api.txt" ]; then
		# apidiff's headings become bold lines; its "- " items are already
		# Markdown list items.
		sed -e 's/^Incompatible changes:$/**Breaking**\n/' \
			-e 's/^Compatible changes:$/\n**Added**\n/' "$base/api.txt"
	else
		echo "None: fixes and internal changes only."
	fi
} >"$notes"

echo "version=$next"
