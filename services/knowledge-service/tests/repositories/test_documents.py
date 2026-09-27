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
