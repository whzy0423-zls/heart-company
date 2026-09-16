#!/usr/bin/env python3
import argparse
from pathlib import Path

from app.ingestion.catalog import build_catalog, write_catalog


def main() -> None:
    parser = argparse.ArgumentParser(description="Create a deterministic psychology-book catalog")
    parser.add_argument("source", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    entries = build_catalog(args.source)
    write_catalog(entries, args.output)
    counts: dict[str, int] = {}
    for entry in entries:
        counts[entry.status] = counts.get(entry.status, 0) + 1
    print(" ".join(f"{key}={counts[key]}" for key in sorted(counts)))


if __name__ == "__main__":
    main()
