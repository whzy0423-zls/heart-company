import os

import pytest

from app.repositories import documents as documents_module
from app.domain.queries import KnowledgeScope
from app.repositories.documents import DocumentRecord, PostgresDocumentRepository


DATABASE_URL = os.getenv("TEST_DATABASE_URL")


class RecordingCursor:
    def __init__(self) -> None:
        self.queries: list[str] = []

    def __enter__(self):
        return self

    def __exit__(self, *_args) -> None:
        return None

    def execute(self, query, _params=None) -> None:
        self.queries.append(str(query))

    def executemany(self, query, _params) -> None:
        self.queries.append(str(query))

    def fetchall(self):
        return []


class RecordingConnection:
    def __init__(self, cursor: RecordingCursor) -> None:
        self._cursor = cursor

    def __enter__(self):
        return self

    def __exit__(self, *_args) -> None:
        return None

    def cursor(self):
        return self._cursor


def test_identity_checks_do_not_treat_null_embeddings_as_indexed(monkeypatch) -> None:
    cursor = RecordingCursor()
    monkeypatch.setattr(
        documents_module.psycopg,
        "connect",
        lambda *_args, **_kwargs: RecordingConnection(cursor),
    )
    repository = PostgresDocumentRepository("postgres://fixture")

    repository.existing_identities(["doc-1"])
    repository.existing_content_identities(["hash-1"])

    assert len(cursor.queries) == 2
    assert all("embedding IS NOT NULL" in query for query in cursor.queries)


def test_upsert_repairs_matching_rows_with_null_embeddings(monkeypatch) -> None:
    cursor = RecordingCursor()
    monkeypatch.setattr(
        documents_module.psycopg,
        "connect",
        lambda *_args, **_kwargs: RecordingConnection(cursor),
    )
    repository = PostgresDocumentRepository("postgres://fixture")
    record = DocumentRecord(
        "doc-1", "public", None, None, 0, "标题", "内容", "source", {}, {},
        "hash-1", "BAAI/bge-m3", "v1", [0.1, 0.2],
    )

    assert repository.upsert([record]) == 1
    select_queries = [query for query in cursor.queries if query.lstrip().upper().startswith("SELECT")]
    assert len(select_queries) == 2
    assert all("embedding IS NOT NULL" in query for query in select_queries)


def test_lexical_and_vector_have_live_source_gate_and_indexed_managed_search(monkeypatch):
    repository = PostgresDocumentRepository("postgres://fixture")
    queries = []
    monkeypatch.setattr(repository,"_query",lambda sql,params: queries.append((sql,params)) or [])
    monkeypatch.setattr(repository,"vector_dimension",lambda:2)
    scope = KnowledgeScope(public=True)
    repository.lexical_search("情绪管理 RAG",scope,enneagram_types=set(),max_safety_level=0,limit=10)
    repository.vector_search([0,1],scope,enneagram_types=set(),max_safety_level=0,limit=10)
    assert all("public_source_id IS NULL OR EXISTS" in sql and "s.enabled" in sql for sql,_ in queries)
    assert "to_tsvector('simple',search_text)" in queries[0][0]
    assert "AND COALESCE(public_search_vector,to_tsvector('simple',search_text)) @@" in queries[0][0]
    assert "public_source_id IS NULL AND" in queries[0][0]
    assert "UNION ALL" in queries[0][0] and "ts_rank_cd" in queries[0][0]
    assert "public_source_id = ANY(%(enabled_source_ids)s::text[])" in queries[0][0]
    assert queries[0][1]["managed_query"] == "情绪 | 绪管 | 管理 | rag"
    repository.lexical_search(" ".join(str(index) for index in range(100)),scope,enneagram_types=set(),max_safety_level=0,limit=10)
    assert len(queries[-1][1]["managed_query"].split(" | ")) == 64


def test_lexical_source_ids_are_refreshed_on_each_query_connection(monkeypatch):
    class SourceCursor(RecordingCursor):
        def __init__(self, source_id):
            super().__init__()
            self.source_id = source_id
            self.params = []

        def execute(self, query, params=None):
            super().execute(query, params)
            self.params.append(params)

        def fetchall(self):
            if "SELECT id FROM public_knowledge_sources" in self.queries[-1]:
                return [{"id": self.source_id}]
            return []

    cursors = [SourceCursor("enabled-a"), SourceCursor("enabled-b")]
    connections = iter(RecordingConnection(cursor) for cursor in cursors)
    monkeypatch.setattr(documents_module.psycopg, "connect", lambda *_args, **_kwargs: next(connections))
    repository = PostgresDocumentRepository("postgres://fixture")
    for _ in cursors:
        repository.lexical_search("情绪", KnowledgeScope(public=True), enneagram_types=set(), max_safety_level=0, limit=10)
    for cursor in cursors:
        assert "WHERE enabled ORDER BY id" in cursor.queries[0]
        assert cursor.params[-1]["enabled_source_ids"] == [cursor.source_id]
        assert "s.enabled" in cursor.queries[-1]


def test_managed_lexical_gate_can_be_planned_as_a_semi_join(monkeypatch):
    repository = PostgresDocumentRepository("postgres://fixture")
    queries = []
    monkeypatch.setattr(repository, "_query", lambda sql, params: queries.append(sql) or [])
    repository.lexical_search(
        "growth", KnowledgeScope(public=True),
        enneagram_types=set(), max_safety_level=0, limit=10,
    )

    legacy, managed = queries[0].split("UNION ALL", 1)
    assert "public_source_id IS NULL OR EXISTS" in legacy
    assert "public_source_id IS NULL OR EXISTS" not in managed
    assert "EXISTS (" in managed and "s.enabled" in managed
    assert "public_source_id = ANY(%(enabled_source_ids)s::text[])" in managed


def test_lexical_bitmap_budget_is_local_to_its_query_connection(monkeypatch):
    lexical_cursor, vector_cursor = RecordingCursor(), RecordingCursor()
    connections = iter(RecordingConnection(cursor) for cursor in (lexical_cursor, vector_cursor))
    monkeypatch.setattr(documents_module.psycopg, "connect", lambda *_args, **_kwargs: next(connections))
    repository = PostgresDocumentRepository("postgres://fixture")
    monkeypatch.setattr(repository, "vector_dimension", lambda: 2)
    scope = KnowledgeScope(public=True)

    repository.lexical_search("growth", scope, enneagram_types=set(), max_safety_level=0, limit=10)
    repository.vector_search([0, 1], scope, enneagram_types=set(), max_safety_level=0, limit=10)

    assert lexical_cursor.queries[-2] == "SET LOCAL work_mem='64MB'"
    assert all("work_mem" not in query for query in vector_cursor.queries)


def test_embedding_backfill_preserves_managed_cleaning_version(monkeypatch):
    from app.repositories.documents import PendingEmbeddingDocument
    cursor = RecordingCursor()
    cursor.rowcount = 1
    monkeypatch.setattr(documents_module.psycopg,"connect",lambda *_args,**_kwargs:RecordingConnection(cursor))
    repository = PostgresDocumentRepository("postgres://fixture")
    repository.update_embeddings([PendingEmbeddingDocument("managed","title","content")],[[0,1]],model="model",index_version="embedding-v2")
    assert "CASE WHEN public_source_id IS NULL" in cursor.queries[-1]


def test_embedding_backfill_does_not_load_disabled_managed_sources(monkeypatch):
    cursor = RecordingCursor()
    cursor.fetchone = lambda:(0,)
    monkeypatch.setattr(documents_module.psycopg,"connect",lambda *_args,**_kwargs:RecordingConnection(cursor))
    repository = PostgresDocumentRepository("postgres://fixture")
    assert repository.count_missing_embeddings(library="public") == 0
    assert repository.load_missing_embeddings(library="public",limit=20) == []
    assert all("public_source_id IS NULL OR EXISTS" in sql and "s.enabled" in sql for sql in cursor.queries)


@pytest.mark.skipif(not DATABASE_URL, reason="TEST_DATABASE_URL is not configured")
def test_postgres_repository_upserts_idempotently_and_filters_scope() -> None:
    repository = PostgresDocumentRepository(DATABASE_URL)
    repository.delete_by_index_version("test-v1")
    records = [
        DocumentRecord("public-1", "public", None, None, 0, "公共", "焦虑时先呼吸", "public.txt", {}, {}, "h1", "BAAI/bge-m3", "test-v1", [1.0, 0.0] + [0.0] * 1022),
        DocumentRecord("theory-101", "theory", 101, None, 0, "理论", "三号人格关注成就", "theory.txt", {"page": 1}, {}, "h2", "BAAI/bge-m3", "test-v1", [0.0, 1.0] + [0.0] * 1022),
        DocumentRecord("theory-999", "theory", 999, None, 0, "越界", "不应出现", "other.txt", {}, {}, "h3", "BAAI/bge-m3", "test-v1", [0.0, 1.0] + [0.0] * 1022),
        DocumentRecord("public-duplicate-content", "public", None, None, 0, "重复", "焦虑时先呼吸", "duplicate.txt", {}, {}, "h1", "BAAI/bge-m3", "test-v1", [1.0, 0.0] + [0.0] * 1022),
    ]

    assert repository.upsert(records) == 3
    assert repository.upsert(records) == 0
    assert ("h1", "BAAI/bge-m3", "test-v1", "public", 0) in repository.existing_content_identities(["h1"])
    scope = KnowledgeScope(public=True, theoryReleaseIds=[101])
    lexical = repository.lexical_search("三号人格", scope, enneagram_types={3}, max_safety_level=0, limit=20)
    vector = repository.vector_search([0.0, 1.0] + [0.0] * 1022, scope, enneagram_types={3}, max_safety_level=0, limit=20)

    assert {item.id for item in lexical} <= {"public-1", "theory-101"}
    assert "theory-101" in {item.id for item in vector}
    assert "theory-999" not in {item.id for item in vector}
    assert repository.vector_dimension() == 1024
    repository.delete_by_index_version("test-v1")
