#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Generate the Binnacle brand assets from one description of the mark.

The M1 compass-card mark (imp-docs/DESIGN.md §Logo, ADR 027) is defined once
here; every SVG and PNG under assets/brand is written from it, then the files
the app and the landing page serve are copied to web/static and landing/assets.
The wordmark is set in the vendored Barlow Condensed SemiBold, shaped with
HarfBuzz (kerning included) and converted to outlines, so no file depends on a
font being installed where it is shown.

Requirements: python3 with fontTools and brotli (to read the woff2 files),
hb-shape (HarfBuzz utilities) and ImageMagick built with librsvg.

    python3 scripts/brand-assets.py
"""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from pathlib import Path

from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont

ROOT = Path(__file__).resolve().parent.parent
BRAND = ROOT / "assets" / "brand"
FONTS = ROOT / "web" / "static" / "fonts"
WORDMARK_FONT = FONTS / "barlow-condensed" / "BarlowCondensed-SemiBold.woff2"
COPY_FONT = FONTS / "barlow" / "Barlow-Regular.woff2"

# Colours from web/src/styles/tokens.css (ADR 027). Brass is the dark-theme
# accent, slate the dark background, cream the dark foreground, muted the
# dark muted text.
BRASS = "#d2a24c"
SLATE = "#1f242b"
CREAM = "#e9e4d8"
MUTED = "#a4abb5"

NAME = "Binnacle"
TAGLINE = "Lightweight, Coolify-aware monitoring for Docker servers."


# --------------------------------------------------------------------------
# The mark
# --------------------------------------------------------------------------


@dataclass(frozen=True)
class Geometry:
    """One drawing of the compass card on the 24-unit grid."""

    ring_radius: float = 10.5
    stroke: float = 1.5
    tip: float = 2.5  # north needle from (12, tip) to (12, 24 - tip)
    half_width: float = 2.2  # needle half width at the hub
    east_west: bool = True  # the faint needle at 45 percent opacity
    hub: float = 1.7  # radius of the hub cut-out, 0 for none


# Full mark: sidebar, auth card, lockups, social image.
FULL = Geometry()
# 32 px favicon: heavier ring, no east-west needle, no hub (too small to read).
FAVICON_32 = Geometry(stroke=1.8, east_west=False, hub=0)
# 16 px favicon: heaviest ring and a slightly wider needle so it survives.
FAVICON_16 = Geometry(stroke=2.4, east_west=False, hub=0, tip=3, half_width=2.5)


def _num(value: float) -> str:
    text = f"{value:.3f}".rstrip("0").rstrip(".")
    return text if text != "-0" else "0"


def _hub_subpath(radius: float) -> str:
    # A circle as two arcs; with fill-rule evenodd it cuts a hole in the
    # needle it is appended to, so the hub is transparent on any surface.
    top = 12 - radius
    return (
        f"M12 {_num(top)}a{_num(radius)} {_num(radius)} 0 1 0 0 {_num(2 * radius)}"
        f"a{_num(radius)} {_num(radius)} 0 1 0 0 {_num(-2 * radius)}Z"
    )


def mark_elements(g: Geometry, colour: str) -> str:
    """The mark's elements on the 24-unit grid, painted in one colour."""
    hub = _hub_subpath(g.hub) if g.hub else ""
    lo, hi = g.tip, 24 - g.tip
    parts = [
        f'<circle cx="12" cy="12" r="{_num(g.ring_radius)}" fill="none" '
        f'stroke="{colour}" stroke-width="{_num(g.stroke)}"/>'
    ]
    if g.east_west:
        parts.append(
            f'<path fill="{colour}" fill-opacity=".45" fill-rule="evenodd" '
            f'd="M{_num(lo)} 12 12 {_num(12 - g.half_width)} {_num(hi)} 12 '
            f'12 {_num(12 + g.half_width)}Z{hub}"/>'
        )
    parts.append(
        f'<path fill="{colour}" fill-rule="evenodd" '
        f'd="M12 {_num(lo)} {_num(12 + g.half_width)} 12 12 {_num(hi)} '
        f'{_num(12 - g.half_width)} 12Z{hub}"/>'
    )
    return "".join(parts)


def svg(width: float, height: float, body: str, *, title: str = NAME, size=None) -> str:
    """A standalone SVG document. size overrides the intrinsic width/height."""
    w, h = size if size else (width, height)
    return (
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {_num(width)} {_num(height)}" '
        f'width="{_num(w)}" height="{_num(h)}" role="img" aria-label="{title}">'
        f"<title>{title}</title>{body}</svg>\n"
    )


def favicon_adaptive() -> str:
    """Slate ring for light tabs, cream for dark, chosen by the browser."""
    style = (
        "<style>path,circle{fill:" + SLATE + ";stroke:" + SLATE + "}"
        "circle{fill:none}"
        "@media (prefers-color-scheme:dark){path,circle{fill:" + CREAM + ";stroke:" + CREAM + "}"
        "circle{fill:none}}</style>"
    )
    # Tabs show the icon at 16 CSS pixels, so the 16 px drawing is the one
    # that has to hold up; on high-density screens it is simply sharper.
    return svg(24, 24, style + mark_elements(FAVICON_16, SLATE), size=(32, 32))


def app_icon(rounded: bool) -> str:
    """Brass tile with a slate card. rounded=False is full bleed for iOS,
    which applies its own mask to apple-touch icons."""
    radius = ' rx="5.52"' if rounded else ""  # 23 percent of 24
    tile = f'<rect width="24" height="24"{radius} fill="{BRASS}"/>'
    card = Geometry(ring_radius=8, stroke=1.6, tip=4.5, half_width=1.8, east_west=False, hub=0)
    return svg(24, 24, tile + mark_elements(card, SLATE))


# --------------------------------------------------------------------------
# Type set as outlines
# --------------------------------------------------------------------------


class Face:
    def __init__(self, woff2: Path, workdir: Path):
        self.font = TTFont(woff2)
        self.glyphs = self.font.getGlyphSet()
        self.upem = self.font["head"].unitsPerEm
        self.cap_height = self.font["OS/2"].sCapHeight
        # hb-shape reads TrueType; write a decompressed copy for it.
        self.ttf = workdir / (woff2.stem + ".ttf")
        plain = TTFont(woff2)
        plain.flavor = None
        plain.save(self.ttf)

    def shape(self, text: str) -> list[dict]:
        out = subprocess.run(
            ["hb-shape", "--output-format=json", str(self.ttf), text],
            check=True,
            capture_output=True,
            text=True,
        ).stdout
        return json.loads(out)


@dataclass
class Line:
    path: str
    left: float  # ink bounds in output units, y down, baseline at y=baseline
    top: float
    right: float
    bottom: float


def set_line(face: Face, text: str, *, cap: float, x: float, baseline: float,
             tracking_em: float = 0.0) -> Line:
    """Outline text so its cap height is `cap` output units."""
    scale = cap / face.cap_height
    tracking = tracking_em * face.upem
    pen = SVGPathPen(face.glyphs, ntos=lambda v: f"{v:.2f}".rstrip("0").rstrip("."))
    bounds = BoundsPen(face.glyphs)
    cursor = 0.0
    shaped = face.shape(text)
    for index, item in enumerate(shaped):
        name = item["g"]
        ox = cursor + item.get("dx", 0)
        oy = item.get("dy", 0)
        transform = (scale, 0, 0, -scale, x + ox * scale, baseline - oy * scale)
        face.glyphs[name].draw(TransformPen(pen, transform))
        face.glyphs[name].draw(TransformPen(bounds, transform))
        cursor += item["ax"]
        if index < len(shaped) - 1:
            cursor += tracking
    left, top, right, bottom = bounds.bounds
    return Line(pen.getCommands(), left, top, right, bottom)


def line_width(face: Face, text: str, cap: float) -> float:
    line = set_line(face, text, cap=cap, x=0, baseline=0)
    return line.right - line.left


def balance(face: Face, text: str, cap: float, max_width: float) -> list[str]:
    """One line if it fits, otherwise the two-line break with the shortest
    longer line, so no word is left on its own."""
    if line_width(face, text, cap) <= max_width:
        return [text]
    words = text.split()
    best: tuple[float, list[str]] | None = None
    for cut in range(1, len(words)):
        lines = [" ".join(words[:cut]), " ".join(words[cut:])]
        widest = max(line_width(face, line, cap) for line in lines)
        if widest <= max_width and (best is None or widest < best[0]):
            best = (widest, lines)
    if best is None:
        raise ValueError(f"cannot fit {text!r} in two lines of {max_width}")
    return best[1]


# --------------------------------------------------------------------------
# Compositions
# --------------------------------------------------------------------------


def wordmark(face: Face, colour: str) -> str:
    cap = 100.0
    line = set_line(face, NAME, cap=cap, x=0, baseline=cap, tracking_em=-0.01)
    # Translate so the ink box starts at the origin.
    width = line.right - line.left
    height = line.bottom - line.top
    body = (
        f'<path fill="{colour}" transform="translate({_num(-line.left)} {_num(-line.top)})" '
        f'd="{line.path}"/>'
    )
    return svg(width, height, body, size=(width / 2, height / 2))


def lockup_body(face: Face, mark_colour: str, text_colour: str, *, cap: float,
                x: float, baseline: float) -> tuple[str, float]:
    """Mark then wordmark. The mark's ring is 1.2x the cap height across and
    sits centred on the capitals; the gap is 0.5x the cap height."""
    ring = 2 * FULL.ring_radius + FULL.stroke  # outer diameter in grid units
    scale = 1.2 * cap / ring
    centre_y = baseline - cap / 2
    mark_left = x - (12 - ring / 2) * scale  # put the ring's outer edge at x
    mark = (
        f'<g transform="translate({_num(mark_left)} {_num(centre_y - 12 * scale)}) '
        f'scale({_num(scale)})">{mark_elements(FULL, mark_colour)}</g>'
    )
    text_x = x + 1.2 * cap + 0.5 * cap
    line = set_line(face, NAME, cap=cap, x=text_x, baseline=baseline, tracking_em=-0.01)
    shift = text_x - line.left  # align the ink, not the side bearing, to the gap
    text = (
        f'<path fill="{text_colour}" transform="translate({_num(shift)} 0)" '
        f'd="{line.path}"/>'
    )
    return mark + text, line.right + shift


def lockup(face: Face, mark_colour: str, text_colour: str) -> str:
    cap = 100.0
    top_pad = 0.1 * cap  # the ring rises 0.1 cap above the capitals
    body, right = lockup_body(face, mark_colour, text_colour, cap=cap, x=0,
                              baseline=cap + top_pad)
    width, height = right, 1.2 * cap
    # Ascender of the l rises above the cap height; measure it.
    line = set_line(face, NAME, cap=cap, x=0, baseline=cap + top_pad)
    top = min(0.0, line.top)
    return svg(width, height - top, f'<g transform="translate(0 {_num(-top)})">{body}</g>',
               size=(width / 2, (height - top) / 2))


def readme_header(face: Face) -> str:
    width, height, cap = 1200.0, 300.0, 84.0
    body, right = lockup_body(face, BRASS, CREAM, cap=cap, x=0, baseline=0)
    left = (width - right) / 2
    baseline = height / 2 + cap / 2
    content, _ = lockup_body(face, BRASS, CREAM, cap=cap, x=left, baseline=baseline)
    background = f'<rect width="{_num(width)}" height="{_num(height)}" rx="12" fill="{SLATE}"/>'
    return svg(width, height, background + content, size=(width, height))


def social(face: Face, copy_face: Face) -> str:
    """1200x630 card: mark at left, wordmark and the tagline beside it, the
    group centred on slate."""
    width, height = 1200.0, 630.0
    margin, gap = 96.0, 64.0
    ring = 2 * FULL.ring_radius + FULL.stroke
    mark_size = 248.0  # ring outer diameter in pixels
    scale = mark_size / ring
    word_cap = 108.0
    copy_cap = 26.0  # cap height of the copy; about 37 px type
    line_gap = copy_cap * 1.95
    lines = balance(copy_face, TAGLINE, copy_cap, width - 2 * margin - mark_size - gap)
    text_width = max(
        [line_width(face, NAME, word_cap)] + [line_width(copy_face, t, copy_cap) for t in lines]
    )
    mark_x = (width - (mark_size + gap + text_width)) / 2
    text_x = mark_x + mark_size + gap
    mark = (
        f'<g transform="translate({_num(mark_x - (12 - ring / 2) * scale)} '
        f'{_num(height / 2 - 12 * scale)}) scale({_num(scale)})">'
        f"{mark_elements(FULL, BRASS)}</g>"
    )
    block = word_cap + 0.75 * word_cap + copy_cap + line_gap * (len(lines) - 1)
    baseline = height / 2 - block / 2 + word_cap
    word = set_line(face, NAME, cap=word_cap, x=text_x, baseline=baseline, tracking_em=-0.01)
    parts = [
        f'<rect width="{_num(width)}" height="{_num(height)}" fill="{SLATE}"/>',
        mark,
        f'<path fill="{CREAM}" transform="translate({_num(text_x - word.left)} 0)" '
        f'd="{word.path}"/>',
    ]
    y = baseline + 0.75 * word_cap + copy_cap
    for text in lines:
        line = set_line(copy_face, text, cap=copy_cap, x=text_x, baseline=y)
        parts.append(
            f'<path fill="{MUTED}" transform="translate({_num(text_x - line.left)} 0)" '
            f'd="{line.path}"/>'
        )
        y += line_gap
    return svg(width, height, "".join(parts), title=f"{NAME}: {TAGLINE}", size=(width, height))


# --------------------------------------------------------------------------
# Output
# --------------------------------------------------------------------------


def write(path: Path, text: str) -> None:
    path.write_text(text, encoding="utf-8")
    print(f"wrote {path.relative_to(ROOT)}")


def render(svg_text: str, out: Path, size: int | tuple[int, int], workdir: Path) -> None:
    """Rasterise with ImageMagick's librsvg delegate at an exact pixel size."""
    w, h = (size, size) if isinstance(size, int) else size
    # Re-declare the intrinsic size so one SVG unit maps to the target pixels.
    head, rest = svg_text.split(">", 1)
    head = re.sub(r'width="[^"]*"', f'width="{w}"', head)
    head = re.sub(r'height="[^"]*"', f'height="{h}"', head)
    tmp = workdir / (out.stem + ".svg")
    tmp.write_text(head + ">" + rest, encoding="utf-8")
    subprocess.run(
        ["magick", "-background", "none", f"rsvg:{tmp}", "-strip",
         "-define", "png:compression-level=9", f"PNG32:{out}"],
        check=True,
    )
    print(f"wrote {out.relative_to(ROOT)} ({w}x{h})")


def main() -> int:
    for tool in ("hb-shape", "magick"):
        if shutil.which(tool) is None:
            print(f"brand-assets: {tool} not found", file=sys.stderr)
            return 1
    BRAND.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory() as tmp:
        work = Path(tmp)
        face = Face(WORDMARK_FONT, work)
        copy_face = Face(COPY_FONT, work)

        sources = {
            "binnacle-mark.svg": svg(24, 24, mark_elements(FULL, BRASS)),
            "binnacle-mark-on-light.svg": svg(24, 24, mark_elements(FULL, SLATE)),
            "binnacle-mark-mono.svg": svg(24, 24, mark_elements(FULL, "currentColor")),
            "favicon.svg": favicon_adaptive(),
            "favicon-16.svg": svg(24, 24, mark_elements(FAVICON_16, SLATE), size=(16, 16)),
            "favicon-32.svg": svg(24, 24, mark_elements(FAVICON_32, SLATE), size=(32, 32)),
            "app-icon.svg": app_icon(rounded=True),
            "binnacle-wordmark.svg": wordmark(face, CREAM),
            "binnacle-wordmark-on-light.svg": wordmark(face, SLATE),
            "binnacle-lockup.svg": lockup(face, BRASS, CREAM),
            "binnacle-lockup-on-light.svg": lockup(face, SLATE, SLATE),
            "binnacle-header.svg": readme_header(face),
            "binnacle-social.svg": social(face, copy_face),
        }
        for name, text in sources.items():
            write(BRAND / name, text)

        render(sources["favicon-16.svg"], BRAND / "favicon-16.png", 16, work)
        render(sources["favicon-32.svg"], BRAND / "favicon-32.png", 32, work)
        render(app_icon(rounded=False), BRAND / "apple-touch-icon.png", 180, work)
        render(sources["app-icon.svg"], BRAND / "app-icon-180.png", 180, work)
        render(sources["app-icon.svg"], BRAND / "app-icon-512.png", 512, work)
        render(sources["binnacle-social.svg"], BRAND / "binnacle-social.png", (1200, 630), work)

    # What the app and the landing page serve.
    served = {
        ROOT / "web" / "static": [
            "favicon.svg", "favicon-16.png", "favicon-32.png", "apple-touch-icon.png",
        ],
        ROOT / "web" / "static" / "brand": ["binnacle-mark.svg", "binnacle-mark-on-light.svg"],
        ROOT / "landing" / "assets": [
            "favicon.svg", "favicon-16.png", "favicon-32.png", "apple-touch-icon.png",
            "binnacle-social.png",
        ],
        ROOT / "landing" / "assets" / "brand": ["binnacle-mark.svg"],
    }
    for target, names in served.items():
        target.mkdir(parents=True, exist_ok=True)
        for name in names:
            shutil.copyfile(BRAND / name, target / name)
            print(f"copied {name} -> {(target / name).relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
