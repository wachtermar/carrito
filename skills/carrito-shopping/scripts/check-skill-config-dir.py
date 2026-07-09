#!/usr/bin/env python3
"""Check that the Hermes skill does not hardcode the default carrito state dir."""

from __future__ import annotations

import re
import sys
from pathlib import Path


CONFIG_BLOCK = re.compile(
    r"- key:\s*carrito\.config_dir(?P<body>.*?)(?:\n\s*- key:|\n\s{2}\w+:|\n---)",
    re.DOTALL,
)
DEFAULT_CARRITO = re.compile(r"default:\s*['\"]?~/.carrito['\"]?")
HARDCODED_DEFAULT_EXPORT = re.compile(
    r"export\s+CARRITO_CONFIG_DIR\s*=\s*['\"]?(?:~|\$HOME|/Users/[^'\"\s]*)/\.carrito['\"]?",
    re.IGNORECASE,
)


def check(path: Path) -> list[str]:
    text = path.read_text(encoding="utf-8", errors="replace")
    failures: list[str] = []

    match = CONFIG_BLOCK.search(text)
    if not match:
        failures.append("missing carrito.config_dir metadata block")
    else:
        block = match.group("body")
        if DEFAULT_CARRITO.search(block):
            failures.append("carrito.config_dir metadata must not default to ~/.carrito")
        if not re.search(r"default:\s*(['\"]{2})", block):
            failures.append("carrito.config_dir metadata should default to an empty string")

    if path.name == "SKILL.md" and "skills/carrito-shopping" in path.as_posix():
        required = [
            "Preserve any existing `CARRITO_CONFIG_DIR`",
            "Do not overwrite an existing `CARRITO_CONFIG_DIR`",
            "never export `CARRITO_CONFIG_DIR` to `~/.carrito`",
        ]
        for phrase in required:
            if phrase not in text:
                failures.append(f"missing instruction: {phrase}")
        if HARDCODED_DEFAULT_EXPORT.search(text):
            failures.append("skill text contains a hardcoded export to the default ~/.carrito state dir")

    return failures


def main() -> int:
    paths = [Path(arg) for arg in sys.argv[1:]]
    if not paths:
        paths = [Path("SKILL.md"), Path("skills/carrito-shopping/SKILL.md")]

    failed = False
    for path in paths:
        failures = check(path)
        if failures:
            failed = True
            for failure in failures:
                print(f"FAIL {path}: {failure}", file=sys.stderr)
        else:
            print(f"ok: {path}")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
