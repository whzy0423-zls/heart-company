from html.parser import HTMLParser
from pathlib import Path, PurePosixPath
from xml.etree import ElementTree
from zipfile import ZipFile

from app.ingestion.metadata import ExtractedDocument, ExtractedSection


class _TextExtractor(HTMLParser):
    def __init__(self) -> None:
        super().__init__()
        self.parts: list[str] = []

    def handle_starttag(self, tag: str, _attrs) -> None:
        if tag in {"p", "h1", "h2", "h3", "h4", "li", "br"} and self.parts:
            self.parts.append("\n")

    def handle_data(self, data: str) -> None:
        value = data.strip()
        if value:
            self.parts.append(value)

    def text(self) -> str:
        return "\n".join(part.strip() for part in "".join(self.parts).splitlines() if part.strip())


def load_epub(path: Path) -> ExtractedDocument:
    with ZipFile(path) as archive:
        container = ElementTree.fromstring(archive.read("META-INF/container.xml"))
        rootfile = next(element for element in container.iter() if element.tag.endswith("rootfile"))
        opf_path = rootfile.attrib["full-path"]
        opf = ElementTree.fromstring(archive.read(opf_path))
        manifest = {
            element.attrib["id"]: element.attrib["href"]
            for element in opf.iter()
            if element.tag.endswith("item") and "id" in element.attrib and "href" in element.attrib
        }
        spine = [
            element.attrib["idref"]
            for element in opf.iter()
            if element.tag.endswith("itemref") and "idref" in element.attrib
        ]
        base = PurePosixPath(opf_path).parent
        sections: list[ExtractedSection] = []
        for chapter, item_id in enumerate(spine, start=1):
            href = manifest.get(item_id)
            if not href:
                continue
            parser = _TextExtractor()
            parser.feed(archive.read(str(base / href)).decode("utf-8", errors="replace"))
            text = parser.text()
            if text:
                sections.append(ExtractedSection(text=text, locator={"chapter": chapter, "href": href}))
    return ExtractedDocument(source=str(path), extractor="epub-opf-v1", sections=sections)

