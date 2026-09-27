from __future__ import annotations

import argparse
import os
from collections.abc import Callable, Iterable
from typing import Protocol

from app.embeddings.client import OpenAICompatibleEmbeddingClient
from app.repositories.documents import PendingEmbeddingDocument, PostgresDocumentRepository


class EmbeddingClient(Protocol):
    def embed(self, texts: list[str]) -> list[list[float]]: ...


class EmbeddingBackfillRepository(Protocol):
    def load_missing_embeddings(self, *, library: str | None, limit: int) -> list[PendingEmbeddingDocument]: ...

    def update_embeddings(
        self,
        documents: list[PendingEmbeddingDocument],
        vectors: list[list[float]],
        *,
        model: str,
        index_version: str,
    ) -> int: ...


def embedding_text(document: PendingEmbeddingDocument, max_chars: int = 6000) -> str:
    return f"{document.title}\n\n{document.content}".strip()[:max_chars]


def backfill_missing_embeddings(
    repository: EmbeddingBackfillRepository,
    embedding: EmbeddingClient,
    *,
    model: str,
    index_version: str,
    dimension: int,
    batch_size: int,
    library: str | None = None,
    limit: int | None = None,
    on_progress: Callable[[int], None] | None = None,
) -> int:
    if batch_size <= 0:
        raise ValueError("batch size must be positive")
    if limit is not None and limit <= 0:
        return 0

    indexed = 0
    while limit is None or indexed < limit:
        current_limit = batch_size if limit is None else min(batch_size, limit - indexed)
        documents = repository.load_missing_embeddings(library=library, limit=current_limit)
        if not documents:
            break
        vectors = embedding.embed([embedding_text(document) for document in documents])
        if len(vectors) != len(documents) or any(len(vector) != dimension for vector in vectors):
            raise RuntimeError("embedding response dimension/count does not match the configured index")
        updated = repository.update_embeddings(
            documents,
            vectors,
            model=model,
            index_version=index_version,
        )
        if updated <= 0:
            raise RuntimeError("embedding backfill made no progress")
        indexed += updated
        if on_progress is not None:
            on_progress(indexed)
    return indexed


def require_env(name: str) -> str:
    value = os.getenv(name, "").strip()
    if not value:
        raise SystemExit(f"{name} is required")
    return value


def main(argv: Iterable[str] | None = None) -> None:
    parser = argparse.ArgumentParser(description="Resume embeddings for knowledge rows whose vector is missing")
    parser.add_argument("--library", choices=["public", "theory", "enneagram", "skill"])
    parser.add_argument("--index-version", default="bge-m3-v1")
    parser.add_argument("--batch-size", type=int, default=16)
    parser.add_argument("--limit", type=int)
    args = parser.parse_args(list(argv) if argv is not None else None)

    repository = PostgresDocumentRepository(require_env("DATABASE_URL"))
    dimension = int(os.getenv("EMBEDDING_DIMENSION", "1024"))
    actual_dimension = repository.vector_dimension()
    if dimension != actual_dimension:
        raise SystemExit(f"embedding dimension {dimension} does not match database vector({actual_dimension})")
    initial = repository.count_missing_embeddings(library=args.library)
    target = initial if args.limit is None else min(initial, args.limit)
    print(f"pending={initial} target={target} library={args.library or 'all'}", flush=True)
    indexed = backfill_missing_embeddings(
        repository,
        OpenAICompatibleEmbeddingClient(
            require_env("EMBEDDING_API_BASE"),
            require_env("EMBEDDING_API_KEY"),
            require_env("EMBEDDING_MODEL"),
        ),
        model=require_env("EMBEDDING_MODEL"),
        index_version=args.index_version,
        dimension=dimension,
        batch_size=args.batch_size,
        library=args.library,
        limit=args.limit,
        on_progress=lambda count: print(f"progress={count}/{target}", flush=True),
    )
    remaining = repository.count_missing_embeddings(library=args.library)
    print(f"indexed={indexed} remaining={remaining}", flush=True)


if __name__ == "__main__":
    main()
