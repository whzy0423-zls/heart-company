from collections.abc import AsyncIterator, Awaitable, Callable
from hmac import compare_digest

from fastapi import Depends, HTTPException, status
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

from app.config import Settings, get_settings
from app.domain.documents import RetrievedDocument
from app.domain.queries import AnswerQuery, RetrievalQuery


Retriever = Callable[[RetrievalQuery], Awaitable[list[RetrievedDocument]]]
AnswerGenerator = Callable[[AnswerQuery, list[RetrievedDocument]], AsyncIterator[str]]
bearer = HTTPBearer(auto_error=False)


async def fixture_retriever(_query: RetrievalQuery) -> list[RetrievedDocument]:
    return []


def get_retriever() -> Retriever:
    return fixture_retriever


async def fixture_answer_generator(
    query: AnswerQuery, documents: list[RetrievedDocument]
) -> AsyncIterator[str]:
    if documents:
        yield documents[0].content
    else:
        yield f"我已收到你的问题：{query.query}"


def get_answer_generator() -> AnswerGenerator:
    return fixture_answer_generator


def require_service_token(
    credentials: HTTPAuthorizationCredentials | None = Depends(bearer),
    settings: Settings = Depends(get_settings),
) -> None:
    expected = settings.service_token.get_secret_value()
    if (
        credentials is None
        or credentials.scheme.lower() != "bearer"
        or not expected
        or not compare_digest(credentials.credentials, expected)
    ):
        raise HTTPException(
            status_code=status.HTTP_401_UNAUTHORIZED,
            detail="invalid service token",
            headers={"WWW-Authenticate": "Bearer"},
        )
