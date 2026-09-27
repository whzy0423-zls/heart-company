from fastapi.testclient import TestClient

from app.dependencies import get_retriever
from app.domain.documents import RetrievedDocument
from app.main import app


AUTH = {"Authorization": "Bearer test-service-token"}


def request_body() -> dict:
    return {
        "requestId": "req-scope",
        "query": "测试范围",
        "scene": "app_chat",
        "scope": {
            "public": False,
            "theoryReleaseIds": [101],
            "enneagramReleaseIds": [],
            "skillReleaseId": None,
        },
        "profile": {},
        "retrieval": {"topK": 8},
    }


def test_document_inside_requested_release_is_returned() -> None:
    async def allowed_retriever(_query):
        return [
            RetrievedDocument(
                id="doc-1",
                content="允许范围内的内容",
                library="theory",
                release_id=101,
                score=0.91,
                source="book.pdf",
                locator={"page": 12},
            )
        ]

    app.dependency_overrides[get_retriever] = lambda: allowed_retriever
    try:
        response = TestClient(app).post(
            "/internal/v1/retrieve", json=request_body(), headers=AUTH
        )
    finally:
        app.dependency_overrides.clear()

    assert response.status_code == 200
    assert response.json()["documents"][0]["releaseId"] == 101


def test_document_outside_requested_release_is_rejected() -> None:
    async def leaking_retriever(_query):
        return [
            RetrievedDocument(
                id="doc-leak",
                content="不应跨范围返回",
                library="theory",
                release_id=999,
                score=0.99,
                source="other.pdf",
                locator={"page": 1},
            )
        ]

    app.dependency_overrides[get_retriever] = lambda: leaking_retriever
    try:
        response = TestClient(app).post(
            "/internal/v1/retrieve", json=request_body(), headers=AUTH
        )
    finally:
        app.dependency_overrides.clear()

    assert response.status_code == 500
    assert response.json()["detail"] == "retriever returned document outside requested scope"


def test_multiple_skill_releases_are_allowed_without_cross_scope_leakage() -> None:
    captured = None

    async def skill_retriever(query):
        nonlocal captured
        captured = query.scope.skill_release_ids
        return [
            RetrievedDocument(
                id="skill-202",
                content="技能知识",
                library="skill",
                release_id=202,
                score=0.91,
                source="skill-book",
                locator={},
            )
        ]

    body = request_body()
    body["scope"]["theoryReleaseIds"] = []
    body["scope"]["skillReleaseIds"] = [201, 202]
    app.dependency_overrides[get_retriever] = lambda: skill_retriever
    try:
        response = TestClient(app).post(
            "/internal/v1/retrieve", json=body, headers=AUTH
        )
    finally:
        app.dependency_overrides.clear()

    assert response.status_code == 200
    assert captured == [201, 202]
    assert response.json()["documents"][0]["releaseId"] == 202
