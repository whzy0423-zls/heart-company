from __future__ import annotations

import hashlib
from dataclasses import dataclass
from typing import Any


@dataclass(frozen=True)
class EvaluationCase:
    id: str
    category: str
    query: str
    expected_document_id: str
    expected_source: str
    expected_locator: dict[str, Any]


def query_excerpt(content: str, *, length: int = 48) -> str:
    return content.strip()[:length]


def build_cases(rows_by_category: dict[str, list[dict]], *, per_category: int = 100) -> list[EvaluationCase]:
    cases: list[EvaluationCase] = []
    for category in sorted(rows_by_category):
        usable = [row for row in rows_by_category[category] if len(query_excerpt(row["content"])) >= 12]
        if len(usable) < per_category:
            raise ValueError(f"category {category} has only {len(usable)} usable documents")
        ordered = sorted(usable, key=lambda row: hashlib.sha256(row["id"].encode()).hexdigest())
        for index, row in enumerate(ordered[:per_category], start=1):
            cases.append(
                EvaluationCase(
                    id=f"{category}-{index:03d}",
                    category=category,
                    query=query_excerpt(row["content"]),
                    expected_document_id=row["id"],
                    expected_source=row["source"],
                    expected_locator=row.get("locator") or {},
                )
            )
    return cases
