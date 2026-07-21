#!/usr/bin/env python3
"""Verify editorial image assets for the miniapp redesign."""

from __future__ import annotations

import argparse
import sys
from dataclasses import dataclass
from pathlib import Path

from PIL import Image, UnidentifiedImageError


ROOT = Path(__file__).resolve().parents[1]
ASSET_DIR = ROOT / "src" / "static" / "editorial"


@dataclass(frozen=True)
class AssetSpec:
    width: int
    height: int
    max_bytes: int


INITIAL_ASSETS = {
    "home-hero.webp": AssetSpec(1200, 900, 220 * 1024),
    "center-head.webp": AssetSpec(720, 480, 120 * 1024),
    "center-heart.webp": AssetSpec(720, 480, 120 * 1024),
    "center-gut.webp": AssetSpec(720, 480, 120 * 1024),
    "course-intro.webp": AssetSpec(800, 500, 140 * 1024),
    "course-growth.webp": AssetSpec(800, 500, 140 * 1024),
    "course-relation.webp": AssetSpec(800, 500, 140 * 1024),
}

RESULT_ASSETS = {
    f"result-{number}.webp": AssetSpec(640, 640, 150 * 1024)
    for number in range(1, 10)
}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--group",
        choices=("initial", "all"),
        default="all",
        help="asset group to verify (default: all)",
    )
    return parser.parse_args()


def verify_asset(path: Path, spec: AssetSpec) -> list[str]:
    errors: list[str] = []
    if not path.is_file():
        return [f"missing: {path.relative_to(ROOT)}"]

    byte_size = path.stat().st_size
    if byte_size >= spec.max_bytes:
        errors.append(
            f"too large: {path.name} is {byte_size} bytes; "
            f"must be < {spec.max_bytes} bytes"
        )

    try:
        with Image.open(path) as image:
            if image.format != "WEBP":
                errors.append(
                    f"wrong format: {path.name} is {image.format!r}; expected 'WEBP'"
                )
            if image.size != (spec.width, spec.height):
                errors.append(
                    f"wrong dimensions: {path.name} is {image.width}x{image.height}; "
                    f"expected {spec.width}x{spec.height}"
                )
            image.verify()
    except (OSError, UnidentifiedImageError) as exc:
        errors.append(f"unreadable: {path.name}: {exc}")

    return errors


def main() -> int:
    args = parse_args()
    expected = dict(INITIAL_ASSETS)
    if args.group == "all":
        expected.update(RESULT_ASSETS)

    errors: list[str] = []
    actual_names = (
        {path.name for path in ASSET_DIR.glob("*.webp")}
        if ASSET_DIR.is_dir()
        else set()
    )
    expected_names = set(expected)

    for unexpected in sorted(actual_names - expected_names):
        errors.append(f"unexpected: {ASSET_DIR.relative_to(ROOT) / unexpected}")

    for name, spec in expected.items():
        errors.extend(verify_asset(ASSET_DIR / name, spec))

    if errors:
        print(f"FAIL editorial assets ({args.group}): {len(errors)} error(s)")
        for error in errors:
            print(f"- {error}")
        return 1

    print(f"PASS editorial assets ({args.group}): {len(expected)} files verified")
    return 0


if __name__ == "__main__":
    sys.exit(main())
