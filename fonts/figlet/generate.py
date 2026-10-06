#!/usr/bin/env python3
"""Convert pinned Spleen BDF sources to Decker's stock FIGlet fonts."""

import argparse
import hashlib
from pathlib import Path
import re


REVISION = "57f9219328c9f5873085320fe8bc8f7dd34b8791"
SOURCE_HASHES = {
    "5x8": "40488184d075d0c752cdd239b441c5ece51e50b353156f2496c756c384ab01cb",
    "6x12": "fc0743d164690f99b7e2e1b9d503180e4c719a9831ae03fd8f6da18c857dee27",
    "8x16": "b38b32a66920068965a3101f98071d310c5c74659fe86e55d346140770f8f6e8",
    "12x24": "ab7f434db312de6700e55ee7f174ebd0bd57fef16f7a15bc9ac9478fb26f2cd1",
}


def read_bdf(path, expected_hash):
    data = path.read_bytes()
    if hashlib.sha256(data).hexdigest() != expected_hash:
        raise ValueError(f"{path}: source differs from pinned Spleen revision")
    text = data.decode("utf-8")
    width, height, bx, by = map(
        int, re.search(r"^FONTBOUNDINGBOX (.*)$", text, re.M).group(1).split()
    )
    glyphs = {}
    for glyph in re.findall(r"^STARTCHAR .*?^ENDCHAR", text, re.M | re.S):
        code = int(re.search(r"^ENCODING (.*)$", glyph, re.M).group(1))
        if not 32 <= code <= 126:
            continue
        gw, gh, gx, gy = map(
            int, re.search(r"^BBX (.*)$", glyph, re.M).group(1).split()
        )
        bitmap = re.search(r"^BITMAP\n(.*?)^ENDCHAR", glyph, re.M | re.S)
        bits = bitmap.group(1).splitlines()
        if len(bits) != gh:
            raise ValueError(f"{path}: wrong bitmap height for {code}")
        grid = [[0] * width for _ in range(height)]
        for y, value in enumerate(bits):
            row = int(value, 16)
            for x in range(gw):
                px, py = gx - bx + x, height - (gy - by + gh) + y
                if 0 <= px < width and 0 <= py < height:
                    grid[py][px] = (row >> (len(value) * 4 - 1 - x)) & 1
        glyphs[code] = grid
    if set(glyphs) != set(range(32, 127)) or height % 2:
        raise ValueError(f"{path}: expected all printable ASCII and an even height")
    return width, height, glyphs


def figlet(width, height, glyphs, size, license_text, shadow=False):
    rows = height // 2 + int(shadow)
    columns = width + int(shadow)
    comments = [
        f"Spleen {size}" + (" with a shaded shadow." if shadow else "."),
        "Converted from BDF to FIGlet half-blocks for Decker.",
        f"https://github.com/fcambus/spleen/tree/{REVISION}",
        "Printable ASCII only; fixed-width glyph cells, original bitmap spacing.",
        "Each pair of vertical bitmap pixels becomes a half-block character.",
    ]
    if shadow:
        comments.append(
            "Shadow offset: one column right and two bitmap pixels down; "
            "unoccupied shadow cells use the light shade character."
        )
    comments.extend(["", *license_text.splitlines()])
    lines = [
        f"flf2a$ {rows} {rows} {columns + 3} -1 {len(comments)} 0 0 0",
        *comments,
    ]
    for code in range(32, 127):
        grid = glyphs[code]
        for y in range(rows):
            line = []
            for x in range(columns):
                bits = 0
                if y * 2 < height and x < width:
                    bits = grid[y * 2][x] * 2 + grid[y * 2 + 1][x]
                rune = " ▄▀█"[bits]
                if shadow and not bits and y > 0 and x > 0:
                    if any(grid[(y - 1) * 2 + dy][x - 1] for dy in (0, 1)):
                        rune = "░"
                line.append(rune)
            lines.append("".join(line) + ("@@" if y == rows - 1 else "@"))
    return "\n".join(lines) + "\n"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source_dir", type=Path, help="checkout of the pinned Spleen revision")
    parser.add_argument("--check", action="store_true", help="verify generated files without writing")
    args = parser.parse_args()
    output = Path(__file__).resolve().parent
    license_text = (output / "Spleen-LICENSE.txt").read_text(encoding="utf-8")
    if (args.source_dir / "LICENSE").read_text(encoding="utf-8") != license_text:
        raise ValueError("upstream Spleen license differs from the retained notice")
    for size, expected_hash in SOURCE_HASHES.items():
        width, height, glyphs = read_bdf(
            args.source_dir / f"spleen-{size}.bdf", expected_hash
        )
        for shadow in ([False, True] if size == "6x12" else [False]):
            name = f"Spleen {size}" + (" Shadow" if shadow else "") + ".flf"
            generated = figlet(width, height, glyphs, size, license_text, shadow).encode("utf-8")
            path = output / name
            if args.check:
                if path.read_bytes() != generated:
                    raise ValueError(f"{path}: generated file differs")
            else:
                path.write_bytes(generated)
            print(f"{'Verified' if args.check else 'Generated'} {name}")


if __name__ == "__main__":
    main()
