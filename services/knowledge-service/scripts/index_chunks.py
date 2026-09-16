#!/usr/bin/env python3
import argparse
import json
import os
from pathlib import Path

from app.embeddings.client import OpenAICompatibleEmbeddingClient
from app.repositories.documents import DocumentRecord, PostgresDocumentRepository


def require_env(name: str) -> str:
    value = os.getenv(name, "").strip()
    if not value:
        raise SystemExit(f"{name} is required")
    return value


def main() -> None:
    parser = argparse.ArgumentParser(description="Embed prepared chunks and upsert them into pgvector")
    parser.add_argument("chunks", type=Path)
    parser.add_argument("--library", choices=["public", "theory", "enneagram", "skill"], default="public")
    parser.add_argument("--release-id", type=int)
    parser.add_argument("--enneagram-type", type=int)
    parser.add_argument("--index-version", default="v1")
    parser.add_argument("--batch-size", type=int, default=32)
    args = parser.parse_args()
    if args.library != "public" and args.release_id is None:
        raise SystemExit("--release-id is required for non-public libraries")

    database_url = require_env("DATABASE_URL")
    api_base = require_env("EMBEDDING_API_BASE")
    api_key = require_env("EMBEDDING_API_KEY")
    model = require_env("EMBEDDING_MODEL")
    configured_dimension = int(os.getenv("EMBEDDING_DIMENSION", "1024"))
    repository = PostgresDocumentRepository(database_url)
    actual_dimension = repository.vector_dimension()
    if configured_dimension != actual_dimension:
        raise SystemExit(f"embedding dimension {configured_dimension} does not match database vector({actual_dimension})")
    embedding = OpenAICompatibleEmbeddingClient(api_base, api_key, model)
    rows = [json.loads(line) for line in args.chunks.read_text(encoding="utf-8").splitlines() if line.strip()]
    indexed = 0
    skipped = 0
    for start in range(0, len(rows), args.batch_size):
        batch = rows[start : start + args.batch_size]
        existing = repository.existing_identities([row["id"] for row in batch])
        content_identities = repository.existing_content_identities([row["content_hash"] for row in batch])
        pending = []
        seen = set(content_identities)
        for row in batch:
            identity = (row["content_hash"], model, args.index_version, args.library, args.release_id or 0)
            if existing.get(row["id"]) == (row["content_hash"], model, args.index_version) or identity in seen:
                continue
            seen.add(identity)
            pending.append(row)
        skipped += len(batch) - len(pending)
        if not pending:
            continue
        vectors = embedding.embed([row["content"] for row in pending])
        if any(len(vector) != actual_dimension for vector in vectors):
            raise SystemExit("embedding API returned a dimension that does not match the database")
        records = [
            DocumentRecord(
                id=row["id"],
                library=args.library,
                release_id=args.release_id,
                enneagram_type=args.enneagram_type,
                safety_level=0,
                title=row["title"],
                content=row["content"],
                source=row["source"],
                locator=row["locator"],
                metadata={"kind": row["kind"], "extractor": row["extractor"]},
                content_hash=row["content_hash"],
                embedding_model=model,
                index_version=args.index_version,
                embedding=vector,
            )
            for row, vector in zip(pending, vectors, strict=True)
        ]
        indexed += repository.upsert(records)
        print(f"progress={min(start + len(batch), len(rows))}/{len(rows)} indexed={indexed} skipped={skipped}", flush=True)
    print(f"indexed={indexed} skipped={skipped} total={len(rows)}")


if __name__ == "__main__":
    main()
