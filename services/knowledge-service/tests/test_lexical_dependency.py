from types import SimpleNamespace

from pydantic import SecretStr

from app import dependencies
from app.retrieval.service import PostgresHybridRetriever


def test_database_url_without_embedding_configuration_enables_lexical_retriever(monkeypatch):
    monkeypatch.setattr(dependencies,"get_settings",lambda:SimpleNamespace(
        database_url="postgres://fixture",embedding_api_base="",embedding_api_key=SecretStr(""),embedding_model=""))
    retriever = dependencies.get_retriever()
    assert isinstance(retriever,PostgresHybridRetriever)
    assert retriever.method == "lexical" and retriever.embedding is None


def test_online_embedding_uses_request_budget_without_batch_retries(monkeypatch):
    monkeypatch.setattr(dependencies, "get_settings", lambda: SimpleNamespace(
        database_url="postgres://fixture", embedding_api_base="https://embedding.test",
        embedding_api_key=SecretStr("TOKEN"), embedding_model="model", embedding_dimension=2))
    monkeypatch.setattr(dependencies.PostgresDocumentRepository, "vector_dimension", lambda _self: 2)
    retriever = dependencies.get_retriever()
    try:
        assert retriever.embedding.retries == 0
        assert retriever.embedding.http.timeout.read == 2.0
        assert retriever.embedding.http.timeout.connect == 2.0
    finally:
        retriever.embedding.http.close()
