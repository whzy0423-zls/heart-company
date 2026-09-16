#!/usr/bin/env python3
import argparse
from pathlib import Path

from app.ingestion.pipeline import prepare_documents, write_prepared_chunks


def main() -> None:
    parser = argparse.ArgumentParser(description="Extract and chunk canonical books into an import-ready JSONL index")
    parser.add_argument("catalog", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--limit", type=int, default=20)
    args = parser.parse_args()

    chunks, reports = prepare_documents(args.catalog, limit=args.limit)
    write_prepared_chunks(chunks, args.output)
    prepared = sum(report.status == "prepared" for report in reports)
    failed = sum(report.status == "failed" for report in reports)
    print(f"prepared={prepared} failed={failed} chunks={len(chunks)} output={args.output}")
    for report in reports:
        if report.error:
            print(f"failed: {report.path}: {report.error}")


if __name__ == "__main__":
    main()

