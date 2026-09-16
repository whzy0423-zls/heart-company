from collections.abc import Callable


def embed_in_batches(
    texts: list[str],
    embed: Callable[[list[str]], list[list[float]]],
    *,
    batch_size: int = 32,
    retries: int = 2,
) -> list[list[float]]:
    if batch_size <= 0:
        raise ValueError("batch_size must be positive")
    result: list[list[float]] = []
    for start in range(0, len(texts), batch_size):
        batch = texts[start : start + batch_size]
        for attempt in range(retries + 1):
            try:
                vectors = embed(batch)
                if len(vectors) != len(batch):
                    raise ValueError("embedding batch response count mismatch")
                result.extend(vectors)
                break
            except Exception:
                if attempt == retries:
                    raise
    return result
