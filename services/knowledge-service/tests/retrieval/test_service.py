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

