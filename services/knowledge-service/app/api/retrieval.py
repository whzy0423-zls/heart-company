from fastapi import APIRouter, Depends, HTTPException, status

from app.dependencies import Retriever, get_retriever, require_service_token
from app.domain.documents import RetrievalResponse, RetrievalTrace, RetrievedDocument
from app.domain.queries import KnowledgeScope, RetrievalQuery


router = APIRouter(
    prefix="/internal/v1",
    tags=["retrieval"],
    dependencies=[Depends(require_service_token)],
)


def document_in_scope(document: RetrievedDocument, scope: KnowledgeScope) -> bool:
    if document.library == "public":
        return scope.public and document.release_id is None
    if document.library == "theory":
        return document.release_id in scope.theory_release_ids
    if document.library == "enneagram":
        return document.release_id in scope.enneagram_release_ids
    if document.library == "skill":
        return document.release_id is not None and document.release_id == scope.skill_release_id
    return False


@router.post("/retrieve", response_model=RetrievalResponse, response_model_by_alias=True)
async def retrieve(
    query: RetrievalQuery,
    retriever: Retriever = Depends(get_retriever),
) -> RetrievalResponse:
    documents = await retriever(query)
    if any(not document_in_scope(document, query.scope) for document in documents):
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="retriever returned document outside requested scope",
        )
    limited = documents[: query.retrieval.top_k]
    return RetrievalResponse(
        requestId=query.request_id,
        documents=limited,
        trace=RetrievalTrace(
            retrievalMethod=getattr(retriever, "method", "fixture"),
            candidateCount=len(documents),
            returnedCount=len(limited),
        ),
    )
