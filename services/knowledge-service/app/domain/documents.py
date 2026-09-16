from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field


class RetrievedDocument(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    id: str
    content: str
    library: Literal["public", "theory", "enneagram", "skill"]
    release_id: int | None = Field(default=None, alias="releaseId")
    score: float
    source: str
    locator: dict[str, Any] = Field(default_factory=dict)
    metadata: dict[str, Any] = Field(default_factory=dict)


class RetrievalTrace(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    retrieval_method: str = Field(alias="retrievalMethod")
    candidate_count: int = Field(alias="candidateCount")
    returned_count: int = Field(alias="returnedCount")


class RetrievalResponse(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    request_id: str = Field(alias="requestId")
    documents: list[RetrievedDocument]
    trace: RetrievalTrace


class Citation(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    document_id: str = Field(alias="documentId")
    source: str
    locator: dict[str, Any] = Field(default_factory=dict)


class AnswerResponse(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    request_id: str = Field(alias="requestId")
    answer: str
    citations: list[Citation]
    trace_id: str = Field(alias="traceId")
