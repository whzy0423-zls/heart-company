import asyncio
from typing import Protocol

from app.domain.documents import RetrievedDocument
from app.domain.queries import RetrievalQuery
from app.retrieval.filters import RetrievalFilter, filter_documents
from app.retrieval.hybrid import reciprocal_rank_fusion


class SearchRepository(Protocol):
    def lexical_search(self, query: str, scope, *, enneagram_types: set[int], max_safety_level: int, limit: int) -> list[RetrievedDocument]: ...

    def vector_search(self, embedding: list[float], scope, *, enneagram_types: set[int], max_safety_level: int, limit: int) -> list[RetrievedDocument]: ...


class EmbeddingProvider(Protocol):
    def embed(self, texts: list[str]) -> list[list[float]]: ...


class PostgresHybridRetriever:
    method = "hybrid"

    def __init__(self, repository: SearchRepository, embedding: EmbeddingProvider) -> None:
        self.repository = repository
        self.embedding = embedding

    async def __call__(self, query: RetrievalQuery) -> list[RetrievedDocument]:
        types = {query.profile.main_type} if query.profile.main_type else set()
        vector = (await asyncio.to_thread(self.embedding.embed, [query.query]))[0]
        lexical_task = asyncio.to_thread(
            self.repository.lexical_search,
            query.query,
            query.scope,
            enneagram_types=types,
            max_safety_level=0,
            limit=query.retrieval.lexical_k,
        )
        vector_task = asyncio.to_thread(
            self.repository.vector_search,
            vector,
            query.scope,
            enneagram_types=types,
            max_safety_level=0,
            limit=query.retrieval.vector_k,
        )
        lexical, semantic = await asyncio.gather(lexical_task, vector_task)
        fused = reciprocal_rank_fusion(lexical, semantic)
        filtered = filter_documents(
            fused,
            RetrievalFilter(scope=query.scope, enneagram_types=types, max_safety_level=0),
        )
        return filtered[: min(query.retrieval.rerank_k, query.retrieval.top_k)]
