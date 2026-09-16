import shutil
import subprocess
import tempfile
from pathlib import Path


def ocr_pdf_pages(path: Path, pages: list[int]) -> dict[int, str]:
    """OCR selected 1-based pages with installed pdftoppm/tesseract tools."""
    if not shutil.which("pdftoppm") or not shutil.which("tesseract"):
        raise RuntimeError("OCR requires pdftoppm and tesseract")
    output: dict[int, str] = {}
    with tempfile.TemporaryDirectory(prefix="nx-ocr-") as directory:
        for page in pages:
            image_prefix = Path(directory) / f"page-{page}"
            subprocess.run(
                ["pdftoppm", "-f", str(page), "-singlefile", "-png", str(path), str(image_prefix)],
                check=True,
                timeout=60,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.PIPE,
            )
            result = subprocess.run(
                ["tesseract", str(image_prefix) + ".png", "stdout", "-l", "chi_sim+eng"],
                check=True,
                timeout=120,
                capture_output=True,
                text=True,
            )
            output[page] = result.stdout.strip()
    return output
