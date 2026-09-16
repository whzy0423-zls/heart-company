import json
from pathlib import Path

from app.ingestion.pipeline import prepare_documents


def test_pipeline_prepares_only_canonical_supported_documents_with_limit(tmp_path: Path) -> None:
    first = tmp_path / "first.txt"
    second = tmp_path / "second.txt"
    first.write_text("第一章 开始\n" + "这是第一本书。" * 100, encoding="utf-8")
    second.write_text("第二本书。" * 100, encoding="utf-8")
    catalog = tmp_path / "catalog.jsonl"
    rows = [
        {"id": "1", "path": str(first), "status": "canonical", "format": "txt", "work_id": "w1"},
        {"id": "2", "path": str(second), "status": "canonical", "format": "txt", "work_id": "w2"},
        {"id": "3", "path": str(first), "status": "duplicate", "format": "txt", "work_id": "w1"},
    ]
    catalog.write_text("".join(json.dumps(row, ensure_ascii=False) + "\n" for row in rows), encoding="utf-8")

    chunks, reports = prepare_documents(catalog, limit=1)

    assert chunks
    assert {chunk.source for chunk in chunks} == {str(first)}
    assert reports[0].status == "prepared"
    assert len(reports) == 1

