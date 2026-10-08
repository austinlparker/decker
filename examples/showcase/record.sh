#!/bin/sh
# Records the README's example clips: each clip is a range of slides exported
# with the video command, then turned into a GIF with its own palette. Needs ffmpeg with
# libx264. Run from the repository root:
#
#	sh examples/showcase/record.sh
set -eu

out=docs/assets
size=${SIZE:-960x540}
fps=${FPS:-15}
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

go build -o "$tmp/showcase" ./examples/showcase

# name first-slide last-slide
while read -r name from until; do
	"$tmp/showcase" video "$tmp/$name.mp4" --slide "$from" --until "$until" --size "$size" --fps 30 </dev/null >/dev/null
	ffmpeg -nostdin -v error -y -i "$tmp/$name.mp4" -filter_complex \
		"fps=$fps,split[a][b];[a]palettegen=max_colors=128:stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=4:diff_mode=rectangle" \
		"$out/showcase-$name.gif"
	echo "$out/showcase-$name.gif"
done <<EOF
type 1 3
data 4 5
diagrams 6 8
EOF
