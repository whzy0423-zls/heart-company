from fastapi.testclient import TestClient

from app.main import app


client = TestClient(app)


def test_liveness_is_public() -> None:
    response = client.get("/health/live")

    assert response.status_code == 200
    assert response.json() == {"status": "live"}


def test_readiness_reports_service_ready() -> None:
    response = client.get("/health/ready")

    assert response.status_code == 200
    assert response.json() == {"status": "ready"}
