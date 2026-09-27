import pytest

from app.embeddings.backfill import backfill_missing_embeddings
from app.repositories.documents import PendingEmbeddingDocument


class FakeRepository:
    def __init__(self, documents: list[PendingEmbeddingDocument]) -> None:
        self.documents = list(documents)
        self.updates: list[tuple[list[str], list[list[float]], str, str]] = []

    def load_missing_embeddings(self, *, library: str | None, limit: int):
        assert library in {None, "public"}
        return self.documents[:limit]

    def update_embeddings(self, documents, vectors, *, model: str, index_version: str):
        ids = [document.id for document in documents]
        self.updates.append((ids, vectors, model, index_version))
        self.documents = [document for document in self.documents if document.id not in ids]
        return len(ids)


class FakeEmbeddingClient:
    def __init__(self, dimension: int) -> None:
        self.dimension = dimension
        self.inputs: list[list[str]] = []

    def embed(self, texts: list[str]) -> list[list[float]]:
        self.inputs.append(texts)
        return [[float(index)] * self.dimension for index, _ in enumerate(texts)]


def test_backfill_missing_embeddings_runs_in_resumable_batches() -> None:
    repository = FakeRepository([
        PendingEmbeddingDocument("doc-1", "书一", "内容一"),
        PendingEmbeddingDocument("doc-2", "书二", "内容二"),
        PendingEmbeddingDocument("doc-3", "书三", "内容三"),
    ])
    embedding = FakeEmbeddingClient(3)

    indexed = backfill_missing_embeddings(
        repository,
        embedding,
        model="BAAI/bge-m3",
        index_version="bge-m3-v1",
        dimension=3,
        batch_size=2,
        library="public",
    )

    assert indexed == 3
    assert [update[0] for update in repository.updates] == [["doc-1", "doc-2"], ["doc-3"]]
    assert embedding.inputs == [["书一\n\n内容一", "书二\n\n内容二"], ["书三\n\n内容三"]]
    assert all(update[2:] == ("BAAI/bge-m3", "bge-m3-v1") for update in repository.updates)


def test_backfill_rejects_wrong_embedding_dimension_before_update() -> None:
    repository = FakeRepository([PendingEmbeddingDocument("doc-1", "标题", "内容")])

    with pytest.raises(RuntimeError, match="dimension/count"):
        backfill_missing_embeddings(
            repository,
            FakeEmbeddingClient(2),
            model="BAAI/bge-m3",
            index_version="bge-m3-v1",
            dimension=3,
            batch_size=16,
        )

    assert repository.updates == []


def test_backfill_bounds_embedding_input_without_changing_stored_content() -> None:
    content = "心" * 8000
    repository = FakeRepository([PendingEmbeddingDocument("doc-1", "标题", content)])
    embedding = FakeEmbeddingClient(3)

    assert backfill_missing_embeddings(
        repository,
        embedding,
        model="BAAI/bge-m3",
        index_version="bge-m3-v1",
        dimension=3,
        batch_size=16,
    ) == 1
    assert len(embedding.inputs[0][0]) == 6000
