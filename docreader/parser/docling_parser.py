# -*- coding: utf-8 -*-
"""Docling parser engine.

Docling (https://github.com/DS4SD/docling) is IBM's open-source document
parsing stack with strong support for complex PDFs: it preserves tables,
reading order, figures, and (when enabled) runs a vision language model
over layout blocks for higher-fidelity extraction.

This parser is an *optional* engine — Docling is a heavy dependency
(PyTorch + transformer models), so we soft-import it and report the
engine as unavailable when the package or its models are missing. This
lets teams opt-in without forcing the dependency on every deployment.
"""

from __future__ import annotations

import io
import logging
from typing import Optional, Tuple

from docreader.models.document import Document
from docreader.parser.base_parser import BaseParser

logger = logging.getLogger(__name__)


# Soft-import Docling. We resolve the converter lazily inside __init__ so
# the cost is paid only when the engine is actually exercised.
try:  # pragma: no cover - availability depends on deployment
    from docling.document_converter import DocumentConverter  # type: ignore

    _DOCLING_IMPORT_ERROR: Optional[str] = None
except Exception as exc:  # pragma: no cover
    DocumentConverter = None  # type: ignore[assignment]
    _DOCLING_IMPORT_ERROR = f"{type(exc).__name__}: {exc}"


def docling_available(_overrides=None) -> Tuple[bool, str]:
    """Engine availability probe for the registry.

    Returns (available, reason). A missing docling install is the common
    case on lightweight deployments and is not logged as an error here —
    the registry surfaces the reason to the UI instead.
    """
    if DocumentConverter is None:
        return False, f"docling package not installed ({_DOCLING_IMPORT_ERROR})"
    return True, ""


class DoclingParser(BaseParser):
    """Parse documents via Docling's DocumentConverter into markdown.

    Supported formats (mirrors Docling's native capability):
        pdf, docx, pptx, xlsx, html, images.

    Docling emits a structured document tree that we export to markdown
    via ``export_to_markdown()``. Tables, headings, and reading order are
    preserved, which gives downstream chunking & retrieval much cleaner
    text than plain text extraction.
    """

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        if DocumentConverter is None:
            raise RuntimeError(
                "Docling engine selected but docling package is not installed. "
                "Install with: pip install docling"
            )
        # DocumentConverter caches models per-process after first call, so
        # subsequent parses are fast. Construction is cheap.
        self._converter = DocumentConverter()

    def parse_into_text(self, content: bytes) -> Document:
        ext = self.file_type or "pdf"
        if not ext.startswith("."):
            ext = "." + ext

        # Docling's DocumentConverter accepts file-like streams via the
        # DocumentStream helper. We wrap the bytes with the original file
        # name so Docling can dispatch on the extension.
        from docling.datamodel.base_models import DocumentStream  # type: ignore

        source = DocumentStream(
            name=self.file_name or f"document{ext}",
            stream=io.BytesIO(content),
        )
        logger.info(
            "Docling parsing %s (%d bytes)",
            source.name,
            len(content),
        )
        result = self._converter.convert(source)
        markdown = result.document.export_to_markdown()
        return Document(content=markdown)
