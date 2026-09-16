import hashlib

from app.ingestion.metadata import DocumentChunk, ExtractedSection


def split_sections(
    sections: list[ExtractedSection],
    *,
    target_chars: int = 900,
    max_chars: int = 1200,
    overlap_chars: int = 120,
) -> list[DocumentChunk]:
    if target_chars <= overlap_chars or max_chars < target_chars:
        raise ValueError("chunk sizes must satisfy max >= target > overlap")
    chunks: list[DocumentChunk] = []
    for section in sections:
        text = section.text.strip()
        start = 0
        while start < len(text):
            end = min(start + target_chars, len(text))
            if end < len(text):
                boundary = max(text.rfind(mark, start, min(start + max_chars, len(text))) for mark in ("。", "！", "？", "\n"))
                if boundary >= start + target_chars // 2:
                    end = boundary + 1
            value = text[start:end].strip()
            if value:
                chunks.append(
                    DocumentChunk(
                        text=value,
                        content_hash=hashlib.sha256(value.encode()).hexdigest(),
                        locator=dict(section.locator),
                        title=section.title,
                        kind=section.kind,
                    )
                )
            if end >= len(text):
                break
            start = max(start + 1, end - overlap_chars)
    return chunks

