from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import psycopg
from psycopg.rows import dict_row
from psycopg.types.json import Jsonb

from app.domain.documents import RetrievedDocument
from app.domain.queries import KnowledgeScope
from app.retrieval.tokenization import search_tokens


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


@dataclass(frozen=True)
class PendingEmbeddingDocument:
    id: str
    title: str
    content: str


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
                """SELECT id,content_hash,embedding_model,index_version
                   FROM knowledge_documents
                   WHERE id = ANY(%s::text[]) AND embedding IS NOT NULL AND public_source_id IS NULL""",
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
                   FROM knowledge_documents
                   WHERE content_hash = ANY(%s::text[]) AND embedding IS NOT NULL AND public_source_id IS NULL""",
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
                     embedding=EXCLUDED.embedding,update_time=now()
                   WHERE knowledge_documents.public_source_id IS NULL""",
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
                """SELECT id,content_hash,embedding_model,index_version
                   FROM knowledge_documents
                   WHERE id = ANY(%s::text[]) AND embedding IS NOT NULL AND public_source_id IS NULL""",
                (ids,),
            )
            return {row[0]: (row[1], row[2], row[3]) for row in cursor.fetchall()}

    def existing_content_identities(self, content_hashes: list[str]) -> set[tuple[str, str, str, str, int]]:
        if not content_hashes:
            return set()
        with psycopg.connect(self.database_url) as connection, connection.cursor() as cursor:
            cursor.execute(
                """SELECT content_hash,embedding_model,index_version,library_kind,COALESCE(release_id,0)
                   FROM knowledge_documents
                   WHERE content_hash = ANY(%s::text[]) AND embedding IS NOT NULL AND public_source_id IS NULL""",
                (content_hashes,),
            )
            return set(cursor.fetchall())

    def delete_by_index_version(self, index_version: str) -> None:
        with psycopg.connect(self.database_url) as connection, connection.cursor() as cursor:
            cursor.execute("DELETE FROM knowledge_documents WHERE index_version=%s", (index_version,))

    def count_missing_embeddings(self, *, library: str | None = None) -> int:
        where = "embedding IS NULL AND " + _SOURCE_GATE_SQL
        params: tuple[Any, ...] = ()
        if library:
            where += " AND library_kind=%s"
            params = (library,)
        with psycopg.connect(self.database_url) as connection, connection.cursor() as cursor:
            cursor.execute(f"SELECT count(*) FROM knowledge_documents WHERE {where}", params)
            row = cursor.fetchone()
        return int(row[0]) if row else 0

    def load_missing_embeddings(self, *, library: str | None = None, limit: int) -> list[PendingEmbeddingDocument]:
        where = "embedding IS NULL AND " + _SOURCE_GATE_SQL
        params: list[Any] = []
        if library:
            where += " AND library_kind=%s"
            params.append(library)
        params.append(limit)
        with psycopg.connect(self.database_url) as connection, connection.cursor() as cursor:
            cursor.execute(
                f"""SELECT id,title,content FROM knowledge_documents
                    WHERE {where} ORDER BY id LIMIT %s""",
                tuple(params),
            )
            rows = cursor.fetchall()
        return [PendingEmbeddingDocument(str(row[0]), str(row[1]), str(row[2])) for row in rows]

    def update_embeddings(
        self,
        documents: list[PendingEmbeddingDocument],
        vectors: list[list[float]],
        *,
        model: str,
        index_version: str,
    ) -> int:
        if len(documents) != len(vectors):
            raise ValueError("document and embedding counts differ")
        if not documents:
            return 0
        with psycopg.connect(self.database_url) as connection, connection.cursor() as cursor:
            cursor.executemany(
                """UPDATE knowledge_documents
                   SET embedding_model=%s,
                       index_version=CASE WHEN public_source_id IS NULL THEN %s ELSE index_version END,
                       embedding=%s::vector,update_time=now()
                   WHERE id=%s AND embedding IS NULL""",
                [
                    (model, index_version, _vector_literal(vector), document.id)
                    for document, vector in zip(documents, vectors, strict=True)
                ],
            )
            return max(int(cursor.rowcount), 0)

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
        params.update({"query": query, "managed_query": " | ".join(search_tokens(query).split()[:64]), "limit": limit})
        params["enabled_source_ids"] = None if scope.public else []
        return self._query(
            """SELECT * FROM ((SELECT id,content,library_kind,release_id,source,locator,metadata,
                      CASE WHEN title ILIKE '%%' || %(query)s || '%%' THEN 1.0 ELSE 0.7 END AS score
               FROM knowledge_documents
               WHERE public_source_id IS NULL AND """
            + _SCOPE_SQL
            + """ AND (title ILIKE '%%' || %(query)s || '%%' OR content ILIKE '%%' || %(query)s || '%%')
               ORDER BY score DESC,id LIMIT %(limit)s)
               UNION ALL
               (SELECT id,content,library_kind,release_id,source,locator,metadata,
                   ts_rank_cd(COALESCE(public_search_vector,to_tsvector('simple',search_text)),to_tsquery('simple',%(managed_query)s)) AS score
                FROM knowledge_documents WHERE public_source_id IS NOT NULL
                AND public_source_id = ANY(%(enabled_source_ids)s::text[]) AND """
            + _MANAGED_SCOPE_SQL
            + """ AND COALESCE(public_search_vector,to_tsvector('simple',search_text)) @@ to_tsquery('simple',%(managed_query)s)
                ORDER BY score DESC,id LIMIT %(limit)s)) matches
                ORDER BY score DESC,id LIMIT %(limit)s""",
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
            if "embedding" in params:
                # Online vector failures fall back to lexical results, not batch retry budgets.
                cursor.execute("SET LOCAL statement_timeout='5s'")
            if "enabled_source_ids" in params and params["enabled_source_ids"] is None:
                # A constant source set permits indexed filtering; the live gate still applies.
                cursor.execute("SELECT id FROM public_knowledge_sources WHERE enabled ORDER BY id")
                params = {**params, "enabled_source_ids": [row["id"] for row in cursor.fetchall()]}
            if "enabled_source_ids" in params:
                cursor.execute("SET LOCAL statement_timeout='8s'")
                # Exact GIN bitmaps avoid expensive TOAST rechecks on the full corpus.
                cursor.execute("SET LOCAL work_mem='64MB'")
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


_SOURCE_EXISTS_SQL = """EXISTS (
    SELECT 1 FROM public_knowledge_sources s WHERE s.id=knowledge_documents.public_source_id AND s.enabled
)"""

_SOURCE_GATE_SQL = "(public_source_id IS NULL OR " + _SOURCE_EXISTS_SQL + ")"

_LIBRARY_SCOPE_SQL = """ AND safety_level <= %(max_safety_level)s AND (
    (%(public)s AND library_kind='public' AND release_id IS NULL) OR
    (library_kind='theory' AND release_id = ANY(%(theory_release_ids)s::bigint[])) OR
    (library_kind='enneagram' AND release_id = ANY(%(enneagram_release_ids)s::bigint[]) AND
      (cardinality(%(enneagram_types)s::int[]) = 0 OR enneagram_type = ANY(%(enneagram_types)s::int[]))) OR
    (library_kind='skill' AND
      (release_id = ANY(%(skill_release_ids)s::bigint[]) OR release_id=%(skill_release_id)s))
)"""

_SCOPE_SQL = _SOURCE_GATE_SQL + _LIBRARY_SCOPE_SQL
# Managed rows need no nullable branch; direct EXISTS permits an indexed semi-join.
_MANAGED_SCOPE_SQL = _SOURCE_EXISTS_SQL + _LIBRARY_SCOPE_SQL


def _scope_params(scope: KnowledgeScope, enneagram_types: set[int], max_safety_level: int) -> dict[str, Any]:
    return {
        "public": scope.public,
        "theory_release_ids": scope.theory_release_ids,
        "enneagram_release_ids": scope.enneagram_release_ids,
        "skill_release_ids": scope.skill_release_ids,
        "skill_release_id": scope.skill_release_id,
        "enneagram_types": sorted(enneagram_types),
        "max_safety_level": max_safety_level,
    }


def _vector_literal(vector: list[float]) -> str:
    return "[" + ",".join(format(value, ".9g") for value in vector) + "]"
