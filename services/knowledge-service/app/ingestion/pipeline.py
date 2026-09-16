from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path

from app.ingestion.chapter_splitter import split_chapters
from app.ingestion.loaders import load_document
from app.ingestion.metadata import ExtractedSection
from app.ingestion.semantic_splitter import split_sections


@dataclass(frozen=True)
class PreparedChunk:
    id: str
    source: str
    title: str
    content: str
    content_hash: str
    locator: dict
    kind: str
    extractor: str


@dataclass(frozen=True)
class IngestionReport:
    path: str
    status: str
    chunk_count: int = 0
    error: str | None = None


def prepare_documents(catalog_path: Path, *, limit: int = 20) -> tuple[list[PreparedChunk], list[IngestionReport]]:
    rows = [json.loads(line) for line in catalog_path.read_text(encoding="utf-8").splitlines() if line.strip()]
    selected = [row for row in rows if row.get("status") == "canonical"][:limit]
    chunks: list[PreparedChunk] = []
    reports: list[IngestionReport] = []
    for row in selected:
        path = Path(row["path"])
        try:
            document = load_document(path)
            sections: list[ExtractedSection] = []
            for extracted in document.sections:
                chapter_sections = split_chapters(extracted.text)
                if not chapter_sections:
                    chapter_sections = [extracted]
                for section in chapter_sections:
                    sections.append(
                        ExtractedSection(
                            text=section.text,
                            locator={**extracted.locator, **section.locator},
                            title=section.title or extracted.title,
                            kind=section.kind,
                        )
                    )
            prepared = split_sections(sections)
            title = path.stem
            for index, chunk in enumerate(prepared):
                chunks.append(
                    PreparedChunk(
                        id=f"{row['id']}:{index}",
                        source=str(path),
                        title=chunk.title or title,
                        content=chunk.text,
                        content_hash=chunk.content_hash,
                        locator=chunk.locator,
                        kind=chunk.kind,
                        extractor=document.extractor,
                    )
                )
            reports.append(IngestionReport(str(path), "prepared", len(prepared)))
        except Exception as exc:
            reports.append(IngestionReport(str(path), "failed", error=str(exc)))
    return chunks, reports


def write_prepared_chunks(chunks: list[PreparedChunk], destination: Path) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    with destination.open("w", encoding="utf-8", newline="\n") as output:
        for chunk in chunks:
            output.write(json.dumps(chunk.__dict__, ensure_ascii=False, sort_keys=True) + "\n")
