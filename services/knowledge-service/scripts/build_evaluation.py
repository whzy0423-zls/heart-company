#!/usr/bin/env python3
import argparse
import json
import os
from pathlib import Path

import psycopg
from psycopg.rows import dict_row

from app.evaluation.dataset import build_cases


CATEGORIES = {
    "九型人格": ["九型人格"],
    "焦虑情绪": ["情绪", "情绪急救", "神经症人格"],
    "亲密关系": ["爱的艺术", "非暴力沟通", "社会性动物"],
    "家庭成长": ["心理学原理", "心理学的故事", "高敏感", "内向者优势"],
    "习惯行动": ["拖延心理学", "掌控习惯", "突破天性", "活出生命的意义"],
}


def main() -> None:
    parser = argparse.ArgumentParser(description="Build a deterministic 500-case retrieval evaluation set")
    parser.add_argument("output", type=Path)
    parser.add_argument("--index-version", default="bge-m3-v1")
    args = parser.parse_args()
    database_url = os.getenv("DATABASE_URL", "").strip()
    if not database_url:
        raise SystemExit("DATABASE_URL is required")
    rows_by_category: dict[str, list[dict]] = {}
    with psycopg.connect(database_url, row_factory=dict_row) as connection, connection.cursor() as cursor:
        for category, source_terms in CATEGORIES.items():
            cursor.execute(
                """SELECT id,content,source,locator FROM knowledge_documents
                   WHERE index_version=%s AND source ILIKE ANY(%s::text[])
                   ORDER BY id""",
                (args.index_version, [f"%{term}%" for term in source_terms]),
            )
            rows_by_category[category] = list(cursor.fetchall())
    cases = build_cases(rows_by_category, per_category=100)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with args.output.open("w", encoding="utf-8", newline="\n") as output:
        for case in cases:
            output.write(json.dumps(case.__dict__, ensure_ascii=False, sort_keys=True) + "\n")
    print(f"cases={len(cases)} output={args.output}")


if __name__ == "__main__":
    main()
