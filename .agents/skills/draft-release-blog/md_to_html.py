# /// script
# requires-python = ">=3.10"
# dependencies = [
#     "markdown>=3.5",
# ]
# ///
"""Convert a release blog markdown file to HTML for the Wordpress CMS.

Strips the leading YAML frontmatter block (between two `---` lines) so the
CMS doesn't render it as content. Emits the rendered HTML to stdout.

Usage:
    uv run md_to_html.py /tmp/release-blog-vX.Y.0.md > /tmp/release-blog-vX.Y.0.html
"""
from __future__ import annotations

import sys
from pathlib import Path

import markdown


def strip_frontmatter(text: str) -> str:
    if not text.startswith("---\n"):
        return text
    end = text.find("\n---\n", 4)
    if end == -1:
        return text
    return text[end + len("\n---\n"):]


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: md_to_html.py <path-to-md-file>", file=sys.stderr)
        return 1
    src = Path(sys.argv[1]).read_text(encoding="utf-8")
    body = strip_frontmatter(src)
    html = markdown.markdown(
        body,
        extensions=["extra", "sane_lists"],
        output_format="html5",
    )
    sys.stdout.write(html)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
