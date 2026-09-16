from app.domain.documents import RetrievedDocument
from app.domain.queries import KnowledgeScope
from app.retrieval.context_builder import build_context
from app.retrieval.filters import RetrievalFilter, filter_documents
from app.retrieval.hybrid import reciprocal_rank_fusion
from app.retrieval.reranker import rerank


def document(id: str, library: str, release_id: int | None, score: float, *, enneagram_type: int | None = None):
    return RetrievedDocument(
        id=id,
        content=f"content-{id}",
        library=library,
        releaseId=release_id,
        score=score,
        source="book.pdf",
        locator={},
        metadata={"enneagramType": enneagram_type} if enneagram_type else {},
    )


def test_filters_remove_cross_release_and_cross_enneagram_type_documents() -> None:
    documents = [
        document("allowed", "theory", 101, 1.0),
        document("wrong-release", "theory", 999, 1.0),
        document("type-three", "enneagram", 103, 1.0, enneagram_type=3),
        document("type-eight", "enneagram", 103, 1.0, enneagram_type=8),
    ]
    scope = KnowledgeScope(public=False, theoryReleaseIds=[101], enneagramReleaseIds=[103])

    filtered = filter_documents(documents, RetrievalFilter(scope=scope, enneagram_types={3}, max_safety_level=1))

    assert [item.id for item in filtered] == ["allowed", "type-three"]


def test_rrf_merges_and_deduplicates_lexical_and_vector_results() -> None:
    lexical = [document("a", "public", None, 0.8), document("b", "public", None, 0.7)]
    vector = [document("b", "public", None, 0.9), document("c", "public", None, 0.6)]

    fused = reciprocal_rank_fusion(lexical, vector)

    assert [item.id for item in fused] == ["b", "a", "c"]
    assert len({item.id for item in fused}) == 3


def test_reranker_takes_top_k_and_context_builder_enforces_rune_limit() -> None:
    candidates = [document(str(index), "public", None, float(index)) for index in range(10)]
    ranked = rerank("query", candidates, top_k=8, scorer=lambda _q, item: item.score)

    assert len(ranked) == 8
    assert ranked[0].id == "9"
    context = build_context(ranked, max_runes=35)
    assert len(context) <= 35
    assert "[book.pdf]" in context

