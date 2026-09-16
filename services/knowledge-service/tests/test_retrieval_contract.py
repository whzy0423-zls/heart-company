from fastapi.testclient import TestClient

from app.main import app


client = TestClient(app)
AUTH = {"Authorization": "Bearer test-service-token"}


def request_body() -> dict:
    return {
        "requestId": "req-1",
        "query": "三号人格面对压力时有什么表现？",
        "scene": "app_chat",
        "scope": {
            "public": True,
            "theoryReleaseIds": [101],
            "enneagramReleaseIds": [103],
            "skillReleaseId": None,
        },
        "profile": {"mainType": 3, "wingType": 2},
        "retrieval": {
            "topK": 8,
            "vectorK": 20,
            "lexicalK": 20,
            "rerankK": 8,
            "maxContextRunes": 8000,
        },
    }


def test_retrieve_requires_internal_bearer_token() -> None:
    response = client.post("/internal/v1/retrieve", json=request_body())

    assert response.status_code == 401


def test_retrieve_rejects_wrong_internal_bearer_token() -> None:
    response = client.post(
        "/internal/v1/retrieve",
        json=request_body(),
        headers={"Authorization": "Bearer wrong"},
    )

    assert response.status_code == 401


def test_retrieve_returns_stable_contract() -> None:
    response = client.post("/internal/v1/retrieve", json=request_body(), headers=AUTH)

    assert response.status_code == 200
    assert response.json() == {
        "requestId": "req-1",
        "documents": [],
        "trace": {
            "retrievalMethod": "fixture",
            "candidateCount": 0,
            "returnedCount": 0,
        },
    }


def test_retrieve_rejects_invalid_request_schema() -> None:
    body = request_body()
    body["query"] = ""

    response = client.post("/internal/v1/retrieve", json=body, headers=AUTH)

    assert response.status_code == 422
