from __future__ import annotations

import hashlib
import json
import zipfile
from dataclasses import asdict, dataclass
from pathlib import Path

from app.ingestion.deduplication import normalized_title


SUPPORTED_EXTENSIONS = {".pdf", ".epub", ".txt", ".md", ".doc", ".docx", ".mobi", ".azw3"}
INCOMPLETE_SUFFIXES = {".part", ".download", ".crdownload", ".tmp"}


@dataclass(frozen=True)
class CatalogEntry:
    id: str
    path: str
    size: int
    sha256: str
    format: str
    work_id: str
    status: str
    canonical_id: str | None = None
    reason: str | None = None


def _digest(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def _rejection_reason(path: Path) -> str | None:
    if path.suffix.casefold() in INCOMPLETE_SUFFIXES:
        return "incomplete_download"
    if path.suffix.casefold() not in SUPPORTED_EXTENSIONS:
        return "unsupported_extension"
    if path.stat().st_size == 0:
        return "empty_file"
    if path.suffix.casefold() in {".epub", ".docx"} and not zipfile.is_zipfile(path):
        return "corrupt_archive"
    return None


def build_catalog(root: Path) -> list[CatalogEntry]:
    entries: list[CatalogEntry] = []
    canonical_by_hash: dict[str, str] = {}
    canonical_by_work: dict[str, str] = {}
    for path in sorted((item for item in root.rglob("*") if item.is_file()), key=lambda p: str(p)):
        size = path.stat().st_size
        reason = _rejection_reason(path)
        sha256 = _digest(path) if size else hashlib.sha256(b"").hexdigest()
        entry_id = hashlib.sha256(str(path.resolve()).encode()).hexdigest()[:24]
        work_id = hashlib.sha256(normalized_title(path).encode()).hexdigest()[:24]
        canonical_id = canonical_by_hash.get(sha256) or canonical_by_work.get(work_id)
        status = "rejected" if reason else ("duplicate" if canonical_id else "canonical")
        if status == "canonical":
            canonical_id = entry_id
            canonical_by_hash[sha256] = entry_id
            canonical_by_work[work_id] = entry_id
        entries.append(
            CatalogEntry(
                id=entry_id,
                path=str(path.resolve()),
                size=size,
                sha256=sha256,
                format=path.suffix.casefold().lstrip("."),
                work_id=work_id,
                status=status,
                canonical_id=canonical_id if status == "duplicate" else None,
                reason=reason,
            )
        )
    return entries


def write_catalog(entries: list[CatalogEntry], destination: Path) -> None:
    destination.parent.mkdir(parents=True, exist_ok=True)
    with destination.open("w", encoding="utf-8", newline="\n") as output:
        for entry in entries:
            output.write(json.dumps(asdict(entry), ensure_ascii=False, sort_keys=True) + "\n")

