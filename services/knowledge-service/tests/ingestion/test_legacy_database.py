from app.ingestion.legacy_database import (
    LegacyDocument,
    classify_theory_library,
    legacy_rag_document,
    legacy_theory_document,
)


def test_legacy_rag_document_becomes_public_knowledge() -> None:
    document = legacy_rag_document(
        {
            "id": 7,
            "title": "九型人格概览",
            "content": "九型人格包含九种基本类型。",
            "tags": ["九型", "入门"],
            "source": "manual",
            "sort": 2,
            "update_time": "2026-09-16T00:00:00+00:00",
        }
    )

    assert document.id == "legacy-rag:7"
    assert document.library == "public"
    assert document.release_id is None
    assert document.enneagram_type is None
    assert document.metadata["tags"] == ["九型", "入门"]
    assert len(document.content_hash) == 64


def test_legacy_theory_document_preserves_release_and_enneagram_scope() -> None:
    document = legacy_theory_document(
        {
            "chunk_id": 11,
            "release_id": 77,
            "library_key": "enneagram-type-03",
            "title": "3号成就型",
            "content": "关注目标、效率与价值感。",
            "content_hash": "a" * 64,
            "keywords": ["成就", "效率"],
            "tags": ["九型人格"],
            "clinical_safety": "caution",
            "authority_level": 3,
            "evidence_level": "moderate",
        }
    )

    assert document.id == "legacy-theory:77:11"
    assert document.library == "enneagram"
    assert document.release_id == 77
    assert document.enneagram_type == 3
    # Existing production retrieval currently accepts safety level zero only;
    # preserve the old classification in metadata without silently hiding rows.
    assert document.safety_level == 0
    assert document.metadata["clinicalSafety"] == "caution"


def test_skill_and_generic_theory_libraries_are_classified() -> None:
    assert classify_theory_library("skill-systems-thinking") == ("skill", None)
    assert classify_theory_library("story-skill-fairy-bedtime") == ("skill", None)
    assert classify_theory_library("enneagram-core") == ("enneagram", None)
    assert classify_theory_library("custom-theory") == ("theory", None)


def test_embedding_input_is_bounded_without_truncating_stored_content() -> None:
    document = LegacyDocument(
        id="legacy-rag:1",
        library="public",
        release_id=None,
        enneagram_type=None,
        safety_level=0,
        title="标题",
        content="甲" * 8000,
        source="manual",
        locator={},
        metadata={},
        content_hash="b" * 64,
    )

    assert len(document.embedding_text(max_chars=6000)) == 6000
    assert len(document.content) == 8000
