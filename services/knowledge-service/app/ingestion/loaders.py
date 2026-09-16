import shutil
import subprocess
import tempfile
from pathlib import Path

from app.ingestion.epub_loader import load_epub
from app.ingestion.metadata import ExtractedDocument, ExtractedSection
from app.ingestion.office_loader import load_office
from app.ingestion.pdf_loader import load_pdf


class UnsupportedDocumentError(ValueError):
    pass


def _load_text(path: Path) -> ExtractedDocument:
    return ExtractedDocument(
        source=str(path),
        extractor="text-v1",
        sections=[ExtractedSection(path.read_text(encoding="utf-8", errors="replace"), {"section": 1})],
    )


def _load_ebook_via_calibre(path: Path) -> ExtractedDocument:
    if not shutil.which("ebook-convert"):
        raise UnsupportedDocumentError(f"{path.suffix} requires ebook-convert")
    with tempfile.TemporaryDirectory(prefix="nx-ebook-") as directory:
        output = Path(directory) / "converted.epub"
        subprocess.run(
            ["ebook-convert", str(path), str(output)],
            check=True,
            timeout=180,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        return load_epub(output)


def load_document(path: Path) -> ExtractedDocument:
    suffix = path.suffix.casefold()
    if suffix in {".txt", ".md"}:
        return _load_text(path)
    if suffix == ".pdf":
        return load_pdf(path)
    if suffix == ".epub":
        return load_epub(path)
    if suffix in {".doc", ".docx"}:
        return load_office(path)
    if suffix in {".mobi", ".azw3"}:
        return _load_ebook_via_calibre(path)
    raise UnsupportedDocumentError(f"unsupported document format: {suffix}")
