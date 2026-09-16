from dataclasses import dataclass, field

from app.domain.documents import RetrievedDocument
from app.domain.queries import KnowledgeScope


@dataclass(frozen=True)
class RetrievalFilter:
    scope: KnowledgeScope
    enneagram_types: set[int] = field(default_factory=set)
    max_safety_level: int = 0


def _release_allowed(document: RetrievedDocument, scope: KnowledgeScope) -> bool:
    if document.library == "public":
        return scope.public and document.release_id is None
    if document.library == "theory":
        return document.release_id in scope.theory_release_ids
    if document.library == "enneagram":
        return document.release_id in scope.enneagram_release_ids
    if document.library == "skill":
        return document.release_id is not None and document.release_id == scope.skill_release_id
    return False


def filter_documents(documents: list[RetrievedDocument], filters: RetrievalFilter) -> list[RetrievedDocument]:
    result: list[RetrievedDocument] = []
    for document in documents:
        if not _release_allowed(document, filters.scope):
            continue
        document_type = document.metadata.get("enneagramType")
        if document_type is not None and filters.enneagram_types and document_type not in filters.enneagram_types:
            continue
        if int(document.metadata.get("safetyLevel", 0)) > filters.max_safety_level:
            continue
        result.append(document)
    return result
