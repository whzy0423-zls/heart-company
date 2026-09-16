from typing import Protocol

from app.domain.documents import RetrievedDocument
from app.retrieval.filters import RetrievalFilter


class VectorRetriever(Protocol):
    def search(self, query: str, filters: RetrievalFilter, limit: int) -> list[RetrievedDocument]: ...

