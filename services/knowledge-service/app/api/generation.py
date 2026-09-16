import json
import uuid
from collections.abc import AsyncIterator

from fastapi import APIRouter, Depends, HTTPException, status
from fastapi.responses import StreamingResponse

from app.dependencies import (
    AnswerGenerator,
    Retriever,
    get_answer_generator,
    get_retriever,
    require_service_token,
)
from app.domain.documents import AnswerResponse, Citation, RetrievedDocument
from app.domain.queries import AnswerQuery
from app.api.retrieval import document_in_scope


router = APIRouter(
    prefix="/internal/v1",
    tags=["generation"],
    dependencies=[Depends(require_service_token)],
)


async def _retrieve_scoped(query: AnswerQuery, retriever: Retriever) -> list[RetrievedDocument]:
    documents = await retriever(query)
    if any(not document_in_scope(document, query.scope) for document in documents):
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="retriever returned document outside requested scope",
        )
    return documents[: query.retrieval.top_k]


def _citations(documents: list[RetrievedDocument]) -> list[Citation]:
    return [Citation(documentId=item.id, source=item.source, locator=item.locator) for item in documents]


@router.post("/answer", response_model=AnswerResponse, response_model_by_alias=True)
async def answer(
    query: AnswerQuery,
    retriever: Retriever = Depends(get_retriever),
    generator: AnswerGenerator = Depends(get_answer_generator),
) -> AnswerResponse:
    documents = await _retrieve_scoped(query, retriever)
    tokens = [token async for token in generator(query, documents)]
    return AnswerResponse(
        requestId=query.request_id,
        answer="".join(tokens),
        citations=_citations(documents),
        traceId=uuid.uuid4().hex,
    )


def _event(name: str, data: dict) -> str:
    return f"event: {name}\ndata: {json.dumps(data, ensure_ascii=False, separators=(',', ':'))}\n\n"


@router.post("/answer/stream")
async def answer_stream(
    query: AnswerQuery,
    retriever: Retriever = Depends(get_retriever),
    generator: AnswerGenerator = Depends(get_answer_generator),
) -> StreamingResponse:
    async def stream() -> AsyncIterator[str]:
        trace_id = uuid.uuid4().hex
        try:
            yield _event("retrieval_started", {"requestId": query.request_id})
            documents = await _retrieve_scoped(query, retriever)
            yield _event("retrieval_done", {"documentCount": len(documents)})
            async for token in generator(query, documents):
                yield _event("token", {"text": token})
            yield _event(
                "citations",
                {"citations": [item.model_dump(by_alias=True) for item in _citations(documents)]},
            )
            yield _event("done", {"traceId": trace_id})
        except Exception as exc:
            yield _event("error", {"code": "generation_failed", "message": str(exc)})

    return StreamingResponse(stream(), media_type="text/event-stream")
