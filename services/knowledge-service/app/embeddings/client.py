from __future__ import annotations

import time

import httpx


class OpenAICompatibleEmbeddingClient:
    def __init__(
        self,
        api_base: str,
        api_key: str,
        model: str,
        *,
        http_client: httpx.Client | None = None,
        retries: int = 3,
    ) -> None:
        self.api_base = api_base.rstrip("/")
        self.api_key = api_key
        self.model = model
        self.http = http_client or httpx.Client(timeout=30)
        self.retries = retries

    def embed(self, texts: list[str]) -> list[list[float]]:
        for attempt in range(self.retries + 1):
            response = self.http.post(
                self.api_base + "/v1/embeddings",
                headers={"Authorization": f"Bearer {self.api_key}"},
                json={"model": self.model, "input": texts},
            )
            if response.status_code not in {429, 500, 502, 503, 504} or attempt == self.retries:
                response.raise_for_status()
                break
            time.sleep(min(0.05 * (2**attempt), 0.5))
        rows = sorted(response.json()["data"], key=lambda row: row["index"])
        vectors = [row["embedding"] for row in rows]
        if len(vectors) != len(texts):
            raise ValueError("embedding response count does not match input count")
        return vectors

