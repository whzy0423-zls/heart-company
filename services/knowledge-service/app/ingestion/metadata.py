from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


@dataclass(frozen=True)
class ExtractedSection:
    text: str
    locator: dict[str, Any]
    title: str | None = None
    kind: str = "body"


@dataclass(frozen=True)
class ExtractedDocument:
    source: str
    extractor: str
    sections: list[ExtractedSection] = field(default_factory=list)


@dataclass(frozen=True)
class DocumentChunk:
    text: str
    content_hash: str
    locator: dict[str, Any]
    title: str | None = None
    kind: str = "body"
