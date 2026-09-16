import zipfile
from pathlib import Path

import pytest

from app.ingestion.loaders import UnsupportedDocumentError, load_document
from app.ingestion.pdf_loader import load_pdf


def test_epub_loader_follows_opf_spine_and_preserves_chapter_locator(tmp_path: Path) -> None:
    path = tmp_path / "book.epub"
    with zipfile.ZipFile(path, "w") as archive:
        archive.writestr("META-INF/container.xml", """<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OPS/content.opf"/></rootfiles></container>""")
        archive.writestr("OPS/content.opf", """<package xmlns="http://www.idpf.org/2007/opf"><manifest><item id="c2" href="c2.xhtml"/><item id="c1" href="c1.xhtml"/></manifest><spine><itemref idref="c1"/><itemref idref="c2"/></spine></package>""")
        archive.writestr("OPS/c1.xhtml", "<html><body><h1>第一章</h1><p>正文一</p></body></html>")
        archive.writestr("OPS/c2.xhtml", "<html><body><h1>第二章</h1><p>正文二</p></body></html>")

    document = load_document(path)

    assert [section.text for section in document.sections] == ["第一章\n正文一", "第二章\n正文二"]
    assert document.sections[1].locator == {"chapter": 2, "href": "c2.xhtml"}


def test_pdf_loader_routes_low_density_pages_to_ocr(tmp_path: Path) -> None:
    path = tmp_path / "scan.pdf"
    path.write_bytes(b"%PDF fixture")
    calls: list[list[int]] = []

    def extract(_path: Path) -> list[str]:
        return ["", "足够长的文本" * 30]

    def ocr(_path: Path, pages: list[int]) -> dict[int, str]:
        calls.append(pages)
        return {1: "OCR 第一页"}

    document = load_pdf(path, text_extractor=extract, ocr_extractor=ocr, minimum_page_chars=40)

    assert calls == [[1]]
    assert document.sections[0].text == "OCR 第一页"
    assert document.sections[0].locator == {"page": 1}
    assert document.extractor.startswith("pdf+")


def test_loader_rejects_unsupported_file_without_executing_it(tmp_path: Path) -> None:
    path = tmp_path / "instructions.sh"
    path.write_text("touch SHOULD_NOT_EXIST", encoding="utf-8")

    with pytest.raises(UnsupportedDocumentError):
        load_document(path)

    assert not (tmp_path / "SHOULD_NOT_EXIST").exists()
