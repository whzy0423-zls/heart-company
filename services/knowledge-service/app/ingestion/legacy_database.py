from __future__ import annotations

import argparse
import hashlib
import os
import re
from dataclasses import dataclass
from typing import Any, Iterable, Iterator

import psycopg
from psycopg.rows import dict_row

from app.embeddings.client import OpenAICompatibleEmbeddingClient
from app.repositories.documents import DocumentRecord, PostgresDocumentRepository


@dataclass(frozen=True)
class LegacyDocument:
    id: str
    library: str
    release_id: int | None
    enneagram_type: int | None
    safety_level: int
    title: str
    content: str
    source: str
    locator: dict[str, Any]
    metadata: dict[str, Any]
    content_hash: str

    def embedding_text(self, *, max_chars: int = 6000) -> str:
        text = f"{self.title}\n\n{self.content}".strip()
        return text[:max_chars]


def classify_theory_library(library_key: str) -> tuple[str, int | None]:
    match = re.fullmatch(r"enneagram-type-(\d{2})", library_key)
    if match:
        enneagram_type = int(match.group(1))
        if 1 <= enneagram_type <= 9:
            return "enneagram", enneagram_type
    if library_key == "enneagram-core":
        return "enneagram", None
    if library_key.startswith(("skill-", "story-skill-")):
        return "skill", None
    return "theory", None


def legacy_rag_document(row: dict[str, Any]) -> LegacyDocument:
    title = str(row["title"]).strip()
    content = str(row["content"]).strip()
    digest = hashlib.sha256(f"{title}\n\n{content}".encode()).hexdigest()
    return LegacyDocument(
        id=f"legacy-rag:{row['id']}",
        library="public",
        release_id=None,
        enneagram_type=None,
        safety_level=0,
        title=title,
        content=content,
        source=str(row.get("source") or "legacy-rag"),
        locator={"legacyRagId": int(row["id"])},
        metadata={
            "importedFrom": "rag_documents",
            "tags": list(row.get("tags") or []),
            "sort": int(row.get("sort") or 0),
            "legacyUpdateTime": str(row.get("update_time") or ""),
        },
        content_hash=digest,
    )


def legacy_theory_document(row: dict[str, Any]) -> LegacyDocument:
    library, enneagram_type = classify_theory_library(str(row["library_key"]))
    content = str(row["content"]).strip()
    content_hash = str(row.get("content_hash") or "").strip()
    if not re.fullmatch(r"[0-9a-f]{64}", content_hash):
        content_hash = hashlib.sha256(content.encode()).hexdigest()
    release_id = int(row["release_id"])
    chunk_id = int(row["chunk_id"])
    return LegacyDocument(
        id=f"legacy-theory:{release_id}:{chunk_id}",
        library=library,
        release_id=release_id,
        enneagram_type=enneagram_type,
        # Keep retrieval parity with the legacy service. The original safety
        # classification remains available to policy layers in metadata.
        safety_level=0,
        title=str(row["title"]).strip(),
        content=content,
        source=f"theory:{row['library_key']}",
        locator={
            "legacyChunkId": chunk_id,
            "libraryKey": str(row["library_key"]),
            "releaseId": release_id,
        },
        metadata={
            "importedFrom": "theory_chunks",
            "keywords": list(row.get("keywords") or []),
            "tags": list(row.get("tags") or []),
            "clinicalSafety": str(row.get("clinical_safety") or "general"),
            "authorityLevel": int(row.get("authority_level") or 0),
            "evidenceLevel": str(row.get("evidence_level") or "unknown"),
        },
        content_hash=content_hash,
    )


def load_legacy_documents(database_url: str, source: str = "all") -> list[LegacyDocument]:
    documents: list[LegacyDocument] = []
    with psycopg.connect(database_url, row_factory=dict_row) as connection, connection.cursor() as cursor:
        if source in {"all", "public"}:
            cursor.execute(
                """SELECT id,title,content,tags,source,sort,update_time
                   FROM rag_documents
                   WHERE status='enabled'
                   ORDER BY id"""
            )
            documents.extend(legacy_rag_document(row) for row in cursor.fetchall())
        if source in {"all", "theory"}:
            cursor.execute(
                """SELECT chunk.id AS chunk_id,release.id AS release_id,library.key AS library_key,
                          chunk.title,chunk.content,chunk.content_hash,chunk.keywords,chunk.tags,
                          chunk.clinical_safety,chunk.authority_level,chunk.evidence_level
                   FROM theory_libraries library
                   JOIN theory_library_releases release
                     ON release.library_id=library.id
                    AND release.version=library.current_version
                    AND release.status='active'
                   JOIN theory_release_cards mapping ON mapping.release_id=release.id
                   JOIN theory_chunks chunk ON chunk.id=mapping.chunk_id
                   WHERE library.status='enabled' AND chunk.status='enabled'
                   ORDER BY release.id,chunk.id"""
            )
            documents.extend(legacy_theory_document(row) for row in cursor.fetchall())
    return documents


def batches(items: list[LegacyDocument], size: int) -> Iterator[list[LegacyDocument]]:
    if size <= 0:
        raise ValueError("batch size must be positive")
    for start in range(0, len(items), size):
        yield items[start : start + size]


def migrate_documents(
    documents: list[LegacyDocument],
    repository: PostgresDocumentRepository,
    embedding: OpenAICompatibleEmbeddingClient,
    *,
    model: str,
    index_version: str,
    dimension: int,
    batch_size: int,
) -> tuple[int, int]:
    indexed = 0
    skipped = 0
    for batch in batches(documents, batch_size):
        existing = repository.existing_identities([item.id for item in batch])
        content_identities = repository.existing_content_identities([item.content_hash for item in batch])
        pending: list[LegacyDocument] = []
        seen = set(content_identities)
        for item in batch:
            identity = (item.content_hash, model, index_version, item.library, item.release_id or 0)
            if existing.get(item.id) == (item.content_hash, model, index_version) or identity in seen:
                continue
            seen.add(identity)
            pending.append(item)
        skipped += len(batch) - len(pending)
        if not pending:
            continue
        vectors = embedding.embed([item.embedding_text() for item in pending])
        if len(vectors) != len(pending) or any(len(vector) != dimension for vector in vectors):
            raise RuntimeError("embedding response dimension/count does not match the configured index")
        records = [
            DocumentRecord(
                id=item.id,
                library=item.library,
                release_id=item.release_id,
                enneagram_type=item.enneagram_type,
                safety_level=item.safety_level,
                title=item.title,
                content=item.content,
                source=item.source,
                locator=item.locator,
                metadata=item.metadata,
                content_hash=item.content_hash,
                embedding_model=model,
                index_version=index_version,
                embedding=vector,
            )
            for item, vector in zip(pending, vectors, strict=True)
        ]
        indexed += repository.upsert(records)
        print(
            f"progress={min(indexed + skipped, len(documents))}/{len(documents)} "
            f"indexed={indexed} skipped={skipped}",
            flush=True,
        )
    return indexed, skipped


def require_env(name: str) -> str:
    value = os.getenv(name, "").strip()
    if not value:
        raise SystemExit(f"{name} is required")
    return value


def main(argv: Iterable[str] | None = None) -> None:
    parser = argparse.ArgumentParser(description="Migrate legacy RAG and active theory releases into LangChain pgvector")
    parser.add_argument("--source", choices=["all", "public", "theory"], default="all")
    parser.add_argument("--index-version", default="bge-m3-v1")
    parser.add_argument("--batch-size", type=int, default=16)
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(list(argv) if argv is not None else None)

    database_url = require_env("DATABASE_URL")
    documents = load_legacy_documents(database_url, args.source)
    counts = {kind: sum(item.library == kind for item in documents) for kind in ("public", "theory", "enneagram", "skill")}
    print(f"prepared={len(documents)} counts={counts}", flush=True)
    if args.dry_run:
        return

    api_base = require_env("EMBEDDING_API_BASE")
    api_key = require_env("EMBEDDING_API_KEY")
    model = require_env("EMBEDDING_MODEL")
    dimension = int(os.getenv("EMBEDDING_DIMENSION", "1024"))
    repository = PostgresDocumentRepository(database_url)
    actual_dimension = repository.vector_dimension()
    if dimension != actual_dimension:
        raise SystemExit(f"embedding dimension {dimension} does not match database vector({actual_dimension})")
    indexed, skipped = migrate_documents(
        documents,
        repository,
        OpenAICompatibleEmbeddingClient(api_base, api_key, model),
        model=model,
        index_version=args.index_version,
        dimension=dimension,
        batch_size=args.batch_size,
    )
    print(f"indexed={indexed} skipped={skipped} total={len(documents)}", flush=True)


if __name__ == "__main__":
    main()
