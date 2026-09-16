from collections.abc import Callable

from app.domain.documents import RetrievedDocument


def rerank(
    query: str,
    documents: list[RetrievedDocument],
    *,
    top_k: int,
    scorer: Callable[[str, RetrievedDocument], float] | None = None,
) -> list[RetrievedDocument]:
    if scorer is None:
        return documents[:top_k]
    scored = [(scorer(query, document), index, document) for index, document in enumerate(documents)]
    scored.sort(key=lambda item: (-item[0], item[1]))
    return [document.model_copy(update={"score": score}) for score, _, document in scored[:top_k]]
