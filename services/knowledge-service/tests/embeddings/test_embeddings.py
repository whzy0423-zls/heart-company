import httpx
import pytest

from app.embeddings.batching import embed_in_batches
from app.embeddings.client import OpenAICompatibleEmbeddingClient
from app.embeddings.indexing import DimensionMismatchError, InMemoryEmbeddingIndex, index_chunks
from app.ingestion.metadata import DocumentChunk


def test_embedding_client_sends_openai_compatible_request_and_retries_rate_limit() -> None:
    attempts = 0

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal attempts
        attempts += 1
        assert request.url.path == "/v1/embeddings"
        assert request.headers["authorization"] == "Bearer TOKEN"
        if attempts == 1:
            return httpx.Response(429, json={"error": "rate limited"})
        return httpx.Response(200, json={"data": [{"index": 0, "embedding": [0.1, 0.2]}]})

    http = httpx.Client(transport=httpx.MockTransport(handler))
    client = OpenAICompatibleEmbeddingClient("https://embedding.test", "TOKEN", "model", http_client=http, retries=2)

    assert client.embed(["文本"]) == [[0.1, 0.2]]
    assert attempts == 2


def test_embed_in_batches_retries_only_failed_batch() -> None:
    calls: list[list[str]] = []

    def embed(values: list[str]) -> list[list[float]]:
        calls.append(values)
        if values == ["c", "d"] and calls.count(values) == 1:
            raise RuntimeError("temporary")
        return [[float(ord(value))] for value in values]

    result = embed_in_batches(["a", "b", "c", "d"], embed, batch_size=2, retries=1)

    assert result == [[97.0], [98.0], [99.0], [100.0]]
    assert calls == [["a", "b"], ["c", "d"], ["c", "d"]]


def test_indexing_is_idempotent_and_checks_database_dimension() -> None:
    chunk = DocumentChunk("内容", "hash-1", {"page": 1})
    index = InMemoryEmbeddingIndex(dimension=2)
    embed_calls = 0

    def embed(values: list[str]) -> list[list[float]]:
        nonlocal embed_calls
        embed_calls += 1
        return [[0.1, 0.2] for _ in values]

    assert index_chunks([chunk], model="model", index_version="v1", index=index, embed=embed) == 1
    assert index_chunks([chunk], model="model", index_version="v1", index=index, embed=embed) == 0
    assert embed_calls == 1

    with pytest.raises(DimensionMismatchError):
        index_chunks(
            [DocumentChunk("新内容", "hash-2", {})],
            model="model",
            index_version="v1",
            index=index,
            embed=lambda _values: [[0.1, 0.2, 0.3]],
        )

