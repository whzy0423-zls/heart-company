from fastapi.testclient import TestClient

from app.dependencies import get_answer_generator, get_retriever
from app.domain.documents import RetrievedDocument
from app.main import app


AUTH = {"Authorization": "Bearer test-service-token"}
BODY = {
    "requestId": "req-answer",
    "query": "我最近有点焦虑",
    "scene": "app_chat",
    "scope": {"public": True, "theoryReleaseIds": [], "enneagramReleaseIds": [], "skillReleaseId": None},
    "profile": {},
    "retrieval": {"topK": 8},
    "messages": [{"role": "user", "content": "历史消息"}],
}


async def retriever(_query):
    return [
        RetrievedDocument(
            id="doc-1", content="先观察身体感受。", library="public", releaseId=None,
            score=0.9, source="心理学.pdf", locator={"page": 10},
        )
    ]


async def generator(_query, _documents):
    yield "先慢"
    yield "下来。"


def test_answer_returns_citations_and_trace() -> None:
    app.dependency_overrides[get_retriever] = lambda: retriever
    app.dependency_overrides[get_answer_generator] = lambda: generator
    try:
        response = TestClient(app).post("/internal/v1/answer", json=BODY, headers=AUTH)
    finally:
        app.dependency_overrides.clear()

    assert response.status_code == 200
    payload = response.json()
    assert payload["answer"] == "先慢下来。"
    assert payload["citations"] == [{"documentId": "doc-1", "source": "心理学.pdf", "locator": {"page": 10}}]
    assert payload["traceId"]


def test_stream_emits_protocol_events_in_order() -> None:
    app.dependency_overrides[get_retriever] = lambda: retriever
    app.dependency_overrides[get_answer_generator] = lambda: generator
    try:
        response = TestClient(app).post("/internal/v1/answer/stream", json=BODY, headers=AUTH)
    finally:
        app.dependency_overrides.clear()

    assert response.status_code == 200
    assert response.headers["content-type"].startswith("text/event-stream")
    event_names = [line.removeprefix("event: ") for line in response.text.splitlines() if line.startswith("event: ")]
    assert event_names == ["retrieval_started", "retrieval_done", "token", "token", "citations", "done"]
