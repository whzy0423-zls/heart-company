from app.domain.documents import RetrievedDocument


def reciprocal_rank_fusion(
    lexical: list[RetrievedDocument],
    vector: list[RetrievedDocument],
    *,
    rank_constant: int = 60,
) -> list[RetrievedDocument]:
    scores: dict[str, float] = {}
    documents: dict[str, RetrievedDocument] = {}
    first_seen: dict[str, int] = {}
    position = 0
    for result_set in (lexical, vector):
        for rank, document in enumerate(result_set, start=1):
            scores[document.id] = scores.get(document.id, 0.0) + 1.0 / (rank_constant + rank)
            documents.setdefault(document.id, document)
            if document.id not in first_seen:
                first_seen[document.id] = position
                position += 1
    ordered = sorted(scores, key=lambda item: (-scores[item], first_seen[item]))
    return [replace_model_score(documents[item], scores[item]) for item in ordered]


def replace_model_score(document: RetrievedDocument, score: float) -> RetrievedDocument:
    return document.model_copy(update={"score": score})
