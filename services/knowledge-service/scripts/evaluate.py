#!/usr/bin/env python3
import argparse
import json
import os
import time
from pathlib import Path

from app.domain.queries import KnowledgeScope
from app.embeddings.client import OpenAICompatibleEmbeddingClient
from app.evaluation.metrics import EvaluationCaseResult, summarize
from app.repositories.documents import PostgresDocumentRepository
from app.retrieval.hybrid import reciprocal_rank_fusion


def required(name: str) -> str:
    value = os.getenv(name, "").strip()
    if not value:
        raise SystemExit(f"{name} is required")
    return value


def main() -> None:
    parser = argparse.ArgumentParser(description="Evaluate hybrid retrieval against a JSONL dataset")
    parser.add_argument("dataset", type=Path)
    parser.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()
    repository = PostgresDocumentRepository(required("DATABASE_URL"))
    embedding = OpenAICompatibleEmbeddingClient(
        required("EMBEDDING_API_BASE"), required("EMBEDDING_API_KEY"), required("EMBEDDING_MODEL")
    )
    cases = [json.loads(line) for line in args.dataset.read_text(encoding="utf-8").splitlines() if line.strip()]
    results: list[EvaluationCaseResult] = []
    scope = KnowledgeScope(public=True)
    for index, case in enumerate(cases, start=1):
        started = time.perf_counter()
        vector = embedding.embed([case["query"]])[0]
        lexical = repository.lexical_search(case["query"], scope, enneagram_types=set(), max_safety_level=0, limit=20)
        semantic = repository.vector_search(vector, scope, enneagram_types=set(), max_safety_level=0, limit=20)
        retrieved = reciprocal_rank_fusion(lexical, semantic)[:10]
        elapsed_ms = (time.perf_counter() - started) * 1000
        ids = [item.id for item in retrieved]
        expected = next((item for item in retrieved if item.id == case["expected_document_id"]), None)
        citation_correct = bool(
            expected
            and expected.source == case["expected_source"]
            and expected.locator == case["expected_locator"]
        )
        results.append(EvaluationCaseResult(case["expected_document_id"], ids, elapsed_ms, False, citation_correct))
        if index % 25 == 0:
            print(f"progress={index}/{len(cases)}", flush=True)
    report = summarize(results)
    report["categories"] = {category: sum(case["category"] == category for case in cases) for category in sorted({case["category"] for case in cases})}
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print(json.dumps(report, ensure_ascii=False, sort_keys=True))


if __name__ == "__main__":
    main()
