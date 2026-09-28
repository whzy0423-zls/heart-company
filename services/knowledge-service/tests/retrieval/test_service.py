import pytest

from app.domain.documents import RetrievedDocument
from app.domain.queries import KnowledgeScope, RetrievalQuery
from app.retrieval.service import PostgresHybridRetriever


def doc(identifier: str, score: float) -> RetrievedDocument:
    return RetrievedDocument(
        id=identifier, content=identifier, library="public", releaseId=None,
        score=score, source="book", locator={},
    )


class RepositoryStub:
    def lexical_search(self, *_args, **_kwargs):
        return [doc("both", 1), doc("lexical", 0.8)]

    def vector_search(self, *_args, **_kwargs):
        return [doc("both", 1), doc("vector", 0.9)]


class EmbeddingStub:
    def embed(self, texts: list[str]):
        assert texts == ["问题"]
        return [[0.1, 0.2]]


class WeakVectorOnlyRepositoryStub:
    def lexical_search(self, *_args, **_kwargs):
        return []

    def vector_search(self, *_args, **_kwargs):
        return [doc("weather-word-overlap", 0.5723), doc("weak-semantic", 0.5316)]


class StrongVectorRepositoryStub:
    def lexical_search(self, *_args, **_kwargs):
        return []

    def vector_search(self, *_args, **_kwargs):
        return [doc("strong-semantic", 0.7347), doc("supporting-semantic", 0.5316)]


@pytest.mark.asyncio
async def test_postgres_hybrid_retriever_runs_both_channels_and_rrf() -> None:
    retriever = PostgresHybridRetriever(RepositoryStub(), EmbeddingStub())
    query = RetrievalQuery(
        requestId="req", query="问题", scene="app_chat",
        scope=KnowledgeScope(public=True), profile={"mainType": 3},
        retrieval={"topK": 3, "lexicalK": 20, "vectorK": 20, "rerankK": 3},
    )

    result = await retriever(query)

    assert [item.id for item in result] == ["both", "lexical", "vector"]


@pytest.mark.asyncio
async def test_postgres_hybrid_retriever_drops_weak_vector_only_matches() -> None:
    retriever = PostgresHybridRetriever(WeakVectorOnlyRepositoryStub(), EmbeddingStub())
    query = RetrievalQuery(
        requestId="req", query="问题", scene="app_chat",
        scope=KnowledgeScope(public=True), profile={},
        retrieval={"topK": 3, "lexicalK": 20, "vectorK": 20, "rerankK": 3},
    )

    result = await retriever(query)

    assert result == []


@pytest.mark.asyncio
async def test_postgres_hybrid_retriever_keeps_ranked_vector_set_when_best_match_is_strong() -> None:
    retriever = PostgresHybridRetriever(StrongVectorRepositoryStub(), EmbeddingStub())
    query = RetrievalQuery(
        requestId="req", query="问题", scene="app_chat",
        scope=KnowledgeScope(public=True), profile={},
        retrieval={"topK": 3, "lexicalK": 20, "vectorK": 20, "rerankK": 3},
    )

    result = await retriever(query)

    assert [item.id for item in result] == ["strong-semantic", "supporting-semantic"]


@pytest.mark.asyncio
async def test_retriever_without_embedding_and_failed_embedding_keep_lexical_results() -> None:
    class BrokenEmbedding:
        def embed(self, _texts):
            raise RuntimeError("embedding endpoint unavailable")

    query = RetrievalQuery(requestId="req",query="问题",scene="app_chat",scope=KnowledgeScope(public=True))
    for embedding in (None, BrokenEmbedding()):
        result = await PostgresHybridRetriever(RepositoryStub(), embedding)(query)
        assert [item.id for item in result] == ["both", "lexical"]
