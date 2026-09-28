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
