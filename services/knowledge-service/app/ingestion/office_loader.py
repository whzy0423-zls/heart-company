import shutil
import subprocess
import tempfile
from pathlib import Path
from xml.etree import ElementTree
from zipfile import ZipFile

from app.ingestion.metadata import ExtractedDocument, ExtractedSection


def _load_docx(path: Path) -> ExtractedDocument:
    with ZipFile(path) as archive:
        root = ElementTree.fromstring(archive.read("word/document.xml"))
    paragraphs: list[str] = []
    for paragraph in (item for item in root.iter() if item.tag.endswith("}p")):
        text = "".join(item.text or "" for item in paragraph.iter() if item.tag.endswith("}t")).strip()
        if text:
            paragraphs.append(text)
    return ExtractedDocument(
        source=str(path), extractor="docx-xml-v1", sections=[ExtractedSection("\n".join(paragraphs), {"section": 1})]
    )


def load_office(path: Path) -> ExtractedDocument:
    if path.suffix.casefold() == ".docx":
        return _load_docx(path)
    if not shutil.which("libreoffice"):
        raise RuntimeError("legacy DOC conversion requires libreoffice")
    with tempfile.TemporaryDirectory(prefix="nx-office-") as directory:
        subprocess.run(
            ["libreoffice", "--headless", "--convert-to", "docx", "--outdir", directory, str(path)],
            check=True,
            timeout=120,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
        converted = Path(directory) / (path.stem + ".docx")
        return _load_docx(converted)

