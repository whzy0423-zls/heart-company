from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import psycopg
from psycopg.rows import dict_row
from psycopg.types.json import Jsonb

from app.domain.documents import RetrievedDocument
from app.domain.queries import KnowledgeScope


@dataclass(frozen=True)
class DocumentRecord:
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
    embedding_model: str
    index_version: str
    embedding: list[float]


class PostgresDocumentRepository:
    def __init__(self, database_url: str) -> None:
        self.database_url = database_url

    def vector_dimension(self) -> int:
        with psycopg.connect(self.database_url) as connection, connection.cursor() as cursor:
            cursor.execute(
                """SELECT format_type(atttypid, atttypmod)
                   FROM pg_attribute
                   WHERE attrelid='knowledge_documents'::regclass AND attname='embedding'"""
            )
            value = cursor.fetchone()
        if not value or not value[0].startswith("vector("):
            raise RuntimeError("knowledge_documents.embedding vector column is unavailable")
        return int(value[0][7:-1])

    def upsert(self, records: list[DocumentRecord]) -> int:
        if not records:
            return 0
        ids = [record.id for record in records]
        with psycopg.connect(self.database_url) as connection, connection.cursor() as cursor:
            cursor.execute(
                "SELECT id,content_hash,embedding_model,index_version FROM knowledge_documents WHERE id = ANY(%s::text[])",
                (ids,),
            )
            existing = {row[0]: row[1:] for row in cursor.fetchall()}
            candidates = [
                record
                for record in records
                if existing.get(record.id) != (record.content_hash, record.embedding_model, record.index_version)
            ]
            cursor.execute(
                """SELECT content_hash,embedding_model,index_version,library_kind,COALESCE(release_id,0)
                   FROM knowledge_documents WHERE content_hash = ANY(%s::text[])""",
                ([record.content_hash for record in candidates],),
            )
            known_identities = set(cursor.fetchall())
            pending: list[DocumentRecord] = []
            for record in candidates:
                identity = (
                    record.content_hash,
                    record.embedding_model,
                    record.index_version,
                    record.library,
                    record.release_id or 0,
                )
                if identity in known_identities:
                    continue
                known_identities.add(identity)
                pending.append(record)
            cursor.executemany(
                """INSERT INTO knowledge_documents
                   (id,library_kind,release_id,enneagram_type,safety_level,title,content,source,locator,metadata,
                    content_hash,embedding_model,index_version,embedding,update_time)
                   VALUES (%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s::vector,now())
                   ON CONFLICT (id) DO UPDATE SET
                     library_kind=EXCLUDED.library_kind,release_id=EXCLUDED.release_id,
                     enneagram_type=EXCLUDED.enneagram_type,safety_level=EXCLUDED.safety_level,
                     title=EXCLUDED.title,content=EXCLUDED.content,source=EXCLUDED.source,
                     locator=EXCLUDED.locator,metadata=EXCLUDED.metadata,content_hash=EXCLUDED.content_hash,
                     embedding_model=EXCLUDED.embedding_model,index_version=EXCLUDED.index_version,
                     embedding=EXCLUDED.embedding,update_time=now()""",
                [
                    (
                        record.id,
                        record.library,
                        record.release_id,
                        record.enneagram_type,
                        record.safety_level,
                        record.title,
                        record.content,
                        record.source,
                        Jsonb(record.locator),
                        Jsonb(record.metadata),
                        record.content_hash,
                        record.embedding_model,
                        record.index_version,
                        _vector_literal(record.embedding),
                    )
                    for record in pending
                ],
            )
        return len(pending)

    def existing_identities(self, ids: list[str]) -> dict[str, tuple[str, str, str]]:
        if not ids:
            return {}
        with psycopg.connect(self.database_url) as connection, connection.cursor() as cursor:
            cursor.execute(
                "SELECT id,content_hash,embedding_model,index_version FROM knowledge_documents WHERE id = ANY(%s::text[])",
                (ids,),
            )
            return {row[0]: (row[1], row[2], row[3]) for row in cursor.fetchall()}

    def existing_content_identities(self, content_hashes: list[str]) -> set[tuple[str, str, str, str, int]]:
        if not content_hashes:
            return set()
        with psycopg.connect(self.database_url) as connection, connection.cursor() as cursor:
            cursor.execute(
                """SELECT content_hash,embedding_model,index_version,library_kind,COALESCE(release_id,0)
                   FROM knowledge_documents WHERE content_hash = ANY(%s::text[])""",
                (content_hashes,),
            )
            return set(cursor.fetchall())

    def delete_by_index_version(self, index_version: str) -> None:
        with psycopg.connect(self.database_url) as connection, connection.cursor() as cursor:
            cursor.execute("DELETE FROM knowledge_documents WHERE index_version=%s", (index_version,))

    def lexical_search(
        self,
        query: str,
        scope: KnowledgeScope,
        *,
        enneagram_types: set[int],
        max_safety_level: int,
        limit: int,
    ) -> list[RetrievedDocument]:
        params = _scope_params(scope, enneagram_types, max_safety_level)
        params.update({"query": query, "limit": limit})
        return self._query(
            """SELECT id,content,library_kind,release_id,source,locator,metadata,
                      CASE WHEN title ILIKE '%%' || %(query)s || '%%' THEN 1.0 ELSE 0.7 END AS score
               FROM knowledge_documents
               WHERE """
            + _SCOPE_SQL
            + """ AND (title ILIKE '%%' || %(query)s || '%%' OR content ILIKE '%%' || %(query)s || '%%')
               ORDER BY score DESC, id LIMIT %(limit)s""",
            params,
        )

    def vector_search(
        self,
        embedding: list[float],
        scope: KnowledgeScope,
        *,
        enneagram_types: set[int],
        max_safety_level: int,
        limit: int,
    ) -> list[RetrievedDocument]:
        if len(embedding) != self.vector_dimension():
            raise ValueError("query embedding dimension does not match database vector dimension")
        params = _scope_params(scope, enneagram_types, max_safety_level)
        params.update({"embedding": _vector_literal(embedding), "limit": limit})
        return self._query(
            """SELECT id,content,library_kind,release_id,source,locator,metadata,
                      1 - (embedding <=> %(embedding)s::vector) AS score
               FROM knowledge_documents
               WHERE embedding IS NOT NULL AND """
            + _SCOPE_SQL
            + " ORDER BY embedding <=> %(embedding)s::vector, id LIMIT %(limit)s",
            params,
        )

    def _query(self, sql: str, params: dict[str, Any]) -> list[RetrievedDocument]:
        with psycopg.connect(self.database_url, row_factory=dict_row) as connection, connection.cursor() as cursor:
            cursor.execute(sql, params)
            rows = cursor.fetchall()
        return [
            RetrievedDocument(
                id=row["id"],
                content=row["content"],
                library=row["library_kind"],
                releaseId=row["release_id"],
                score=float(row["score"]),
                source=row["source"],
                locator=row["locator"],
                metadata=row["metadata"],
            )
            for row in rows
        ]


_SCOPE_SQL = """safety_level <= %(max_safety_level)s AND (
    (%(public)s AND library_kind='public' AND release_id IS NULL) OR
    (library_kind='theory' AND release_id = ANY(%(theory_release_ids)s::bigint[])) OR
    (library_kind='enneagram' AND release_id = ANY(%(enneagram_release_ids)s::bigint[]) AND
      (cardinality(%(enneagram_types)s::int[]) = 0 OR enneagram_type = ANY(%(enneagram_types)s::int[]))) OR
    (library_kind='skill' AND release_id=%(skill_release_id)s)
)"""


def _scope_params(scope: KnowledgeScope, enneagram_types: set[int], max_safety_level: int) -> dict[str, Any]:
    return {
        "public": scope.public,
        "theory_release_ids": scope.theory_release_ids,
        "enneagram_release_ids": scope.enneagram_release_ids,
        "skill_release_id": scope.skill_release_id,
        "enneagram_types": sorted(enneagram_types),
        "max_safety_level": max_safety_level,
    }


def _vector_literal(vector: list[float]) -> str:
    return "[" + ",".join(format(value, ".9g") for value in vector) + "]"
