#!/usr/bin/env python3
"""Convert hash-pinned OFL outline sources to bundled FIGlet half-block fonts."""

import argparse
import hashlib
import json
import math
from pathlib import Path

import fontTools
from fontTools.ttLib import TTFont
from PIL import Image, ImageDraw, ImageFont, features, __version__ as pillow_version


ROOT = Path(__file__).resolve().parent


def verified_bytes(path, digest):
    data = path.read_bytes()
    if hashlib.sha256(data).hexdigest() != digest:
        raise ValueError(f"{path}: differs from the recorded source hash")
    return data


def convert(source, entry, settings):
    path = source / entry["source_filename"]
    verified_bytes(path, entry["ttf_sha256"])
    license_name = entry["source_directory"] + "-OFL.txt"
    license_data = verified_bytes(source / license_name, entry["ofl_sha256"])
    license_text = "\n".join(line.rstrip() for line in license_data.decode("utf-8").splitlines()) + "\n"
    if (ROOT / license_name).read_text() != license_text:
        raise ValueError(f"{license_name}: retained notice differs from source")
    notice = license_text.splitlines()
    with TTFont(path) as ttf:
        if not all(code in ttf.getBestCmap() for code in range(32, 127)):
            raise ValueError(f"{path}: missing printable ASCII")
        copyrights = sorted(
            {record.toUnicode() for record in ttf["name"].names if record.nameID == 0}
        )
    if copyrights != entry["ttf_copyrights"]:
        raise ValueError(f"{path}: unexpected copyright metadata")

    probe = ImageFont.truetype(str(path), 100)
    bounds = probe.getbbox("H", anchor="ls")
    size = round(100 * settings["capital_ink_height_pixels"] / (bounds[3] - bounds[1]))
    if size != entry["pixel_size"]:
        raise ValueError(f"{path}: unexpected conversion size")
    font = ImageFont.truetype(str(path), size)
    boxes = [font.getbbox(chr(code), anchor="ls") for code in range(33, 127)]
    top = min(box[1] for box in boxes)
    bottom = max(box[3] for box in boxes)
    height = math.ceil((bottom - top) / 2) * 2
    if height // 2 != entry["height"]:
        raise ValueError(f"{path}: unexpected FIGlet height")
    glyphs = []
    for code in range(32, 127):
        char = chr(code)
        box = font.getbbox(char, anchor="ls")
        left = min(0, box[0])
        width = max(math.ceil(font.getlength(char)), box[2]) - left
        image = Image.new("L", (width, height), 0)
        ImageDraw.Draw(image).text((-left, -top), char, font=font, fill=255, anchor="ls")
        rows = []
        for y in range(0, height, 2):
            row = ""
            for x in range(width):
                bits = 2 * (image.getpixel((x, y)) >= settings["threshold"])
                bits += image.getpixel((x, y + 1)) >= settings["threshold"]
                row += " ▄▀█"[bits]
            rows.append(row + " ")
        if code != 32 and not any(char in "▄▀█" for row in rows for char in row):
            raise ValueError(f"{path}: glyph {code} has no ink after conversion")
        glyphs.append(rows)

    name = Path(entry["bundled_file"]).stem
    comments = [
        f"{name}: half-block conversion for Decker.",
        "Original font: " + entry["source_filename"],
        settings["source"] + "/" + entry["source_directory"],
        "Source SHA-256: " + entry["ttf_sha256"],
        "Full source hashes and conversion settings: display-sources.json.",
        "Printable ASCII; shared baseline; original advances plus one blank column.",
        "Converted font remains under SIL OFL 1.1; notices follow.",
        *["Source font metadata: " + copyright for copyright in copyrights],
        "",
        *notice,
    ]
    max_width = max(len(row) for glyph in glyphs for row in glyph)
    lines = [f"flf2a$ {height // 2} {height // 2} {max_width + 3} -1 {len(comments)} 0 0 0"]
    lines.extend(comments)
    for glyph in glyphs:
        lines.extend(row + ("@@" if i == len(glyph) - 1 else "@") for i, row in enumerate(glyph))
    return ("\n".join(lines) + "\n").encode("utf-8")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", type=Path, help="directory with recorded TTF and OFL sources")
    parser.add_argument("--check", action="store_true", help="verify bundled fonts without writing")
    args = parser.parse_args()
    settings = json.loads((ROOT / "display-sources.json").read_text())
    versions = {
        "pillow_version": pillow_version,
        "fonttools_version": fontTools.__version__,
        "freetype_version": features.version_module("freetype2"),
    }
    for key, actual in versions.items():
        if actual != settings[key]:
            raise ValueError(f"{key}: need {settings[key]}, got {actual}")
    for entry in settings["fonts"]:
        data = convert(args.source, entry, settings)
        output = ROOT / entry["bundled_file"]
        if args.check:
            if output.read_bytes() != data:
                raise ValueError(f"{output}: differs from regenerated font")
            print(f"Verified {output.name}")
        else:
            output.write_bytes(data)
            print(f"Generated {output.name}")


if __name__ == "__main__":
    main()
