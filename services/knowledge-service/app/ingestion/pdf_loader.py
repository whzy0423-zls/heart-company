from collections.abc import Callable
from pathlib import Path

from app.ingestion.metadata import ExtractedDocument, ExtractedSection
from app.ingestion.ocr_loader import ocr_pdf_pages


def _extract_pdf_text(path: Path) -> list[str]:
    from pypdf import PdfReader

    return [(page.extract_text() or "").strip() for page in PdfReader(path).pages]


def load_pdf(
    path: Path,
    *,
    text_extractor: Callable[[Path], list[str]] = _extract_pdf_text,
    ocr_extractor: Callable[[Path, list[int]], dict[int, str]] = ocr_pdf_pages,
    minimum_page_chars: int = 80,
) -> ExtractedDocument:
    pages = text_extractor(path)
    low_density = [index for index, text in enumerate(pages, start=1) if len(text.strip()) < minimum_page_chars]
    ocr_text = ocr_extractor(path, low_density) if low_density else {}
    sections = [
        ExtractedSection(text=ocr_text.get(page, text).strip(), locator={"page": page})
        for page, text in enumerate(pages, start=1)
        if ocr_text.get(page, text).strip()
    ]
    extractor = "pdf+ocr-v1" if low_density else "pypdf-v1"
    return ExtractedDocument(source=str(path), extractor=extractor, sections=sections)
