from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from typing import Protocol

from app.ingestion.metadata import DocumentChunk


class DimensionMismatchError(ValueError):
    pass


@dataclass(frozen=True)
class EmbeddingRecord:
    content_hash: str
    model: str
    index_version: str
    vector: list[float]
    chunk: DocumentChunk


class EmbeddingIndex(Protocol):
    @property
    def dimension(self) -> int: ...

    def contains(self, content_hash: str, model: str, index_version: str) -> bool: ...

    def add(self, record: EmbeddingRecord) -> None: ...


class InMemoryEmbeddingIndex:
    def __init__(self, dimension: int) -> None:
        self.dimension = dimension
        self.records: dict[tuple[str, str, str], EmbeddingRecord] = {}

    def contains(self, content_hash: str, model: str, index_version: str) -> bool:
        return (content_hash, model, index_version) in self.records

    def add(self, record: EmbeddingRecord) -> None:
        self.records[(record.content_hash, record.model, record.index_version)] = record


def index_chunks(
    chunks: list[DocumentChunk],
    *,
    model: str,
    index_version: str,
    index: EmbeddingIndex,
    embed: Callable[[list[str]], list[list[float]]],
) -> int:
    pending = [chunk for chunk in chunks if not index.contains(chunk.content_hash, model, index_version)]
    if not pending:
        return 0
    vectors = embed([chunk.text for chunk in pending])
    if len(vectors) != len(pending):
        raise ValueError("embedding response count does not match chunk count")
    for chunk, vector in zip(pending, vectors, strict=True):
        if len(vector) != index.dimension:
            raise DimensionMismatchError(
                f"embedding dimension {len(vector)} does not match index dimension {index.dimension}"
            )
        index.add(EmbeddingRecord(chunk.content_hash, model, index_version, vector, chunk))
    return len(pending)
