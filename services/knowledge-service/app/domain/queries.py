from typing import Any, Literal

from pydantic import BaseModel, ConfigDict, Field


class APIModel(BaseModel):
    model_config = ConfigDict(populate_by_name=True)


class KnowledgeScope(APIModel):
    public: bool = True
    theory_release_ids: list[int] = Field(default_factory=list, alias="theoryReleaseIds")
    enneagram_release_ids: list[int] = Field(
        default_factory=list, alias="enneagramReleaseIds"
    )
    skill_release_id: int | None = Field(default=None, alias="skillReleaseId")


class Profile(APIModel):
    main_type: int | None = Field(default=None, ge=1, le=9, alias="mainType")
    wing_type: int | None = Field(default=None, ge=1, le=9, alias="wingType")


class RetrievalOptions(APIModel):
    top_k: int = Field(default=8, ge=1, le=100, alias="topK")
    vector_k: int = Field(default=20, ge=1, le=100, alias="vectorK")
    lexical_k: int = Field(default=20, ge=1, le=100, alias="lexicalK")
    rerank_k: int = Field(default=8, ge=1, le=100, alias="rerankK")
    max_context_runes: int = Field(default=8000, ge=1, le=100_000, alias="maxContextRunes")


class RetrievalQuery(APIModel):
    request_id: str = Field(min_length=1, max_length=128, alias="requestId")
    query: str = Field(min_length=1, max_length=20_000)
    scene: Literal[
        "app_chat",
        "skill_chat",
        "xinzhili",
        "compatibility",
        "daily_quiz",
    ]
    scope: KnowledgeScope
    profile: Profile = Field(default_factory=Profile)
    retrieval: RetrievalOptions = Field(default_factory=RetrievalOptions)
    metadata: dict[str, Any] = Field(default_factory=dict)

