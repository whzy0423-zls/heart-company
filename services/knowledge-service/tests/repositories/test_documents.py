import os

import pytest

from app.domain.queries import KnowledgeScope
from app.repositories.documents import DocumentRecord, PostgresDocumentRepository


DATABASE_URL = os.getenv("TEST_DATABASE_URL")


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
