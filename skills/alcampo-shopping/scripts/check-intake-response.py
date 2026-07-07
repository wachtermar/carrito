#!/usr/bin/env python3
"""Check that a captured Hermes response asked required Alcampo intake questions."""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path


REQUIRED = {
    "meal-plan": {
        "people": [r"\bpeople\b", r"\bpersonas\b", r"how many"],
        "diet_safety": [r"\bdiet\b", r"allerg", r"avoid", r"foods? to avoid", r"dislikes?"],
        "budget": [r"\bbudget\b", r"presupuesto", r"grocery budget", r"ignore budget"],
        "pantry": [r"pantry", r"fridge", r"freezer", r"starting from zero", r"start from zero"],
        "cooking": [r"cooking", r"quick", r"batch", r"variety", r"effort", r"healthy"],
    },
    "shopping": {
        "people": [r"\bpeople\b", r"\bpersonas\b", r"how many"],
        "diet_safety": [r"\bdiet\b", r"allerg", r"avoid", r"foods? to avoid", r"dislikes?"],
        "budget": [r"\bbudget\b", r"presupuesto", r"max", r"maximum spend"],
        "policy": [r"cheapest", r"balanced", r"quality"],
        "pantry": [r"pantry", r"fridge", r"freezer", r"starting from zero", r"start from zero"],
        "cart_intent": [r"basket", r"cart", r"review", r"maximum spend", r"max"],
    },
}

FORBIDDEN = [
    r"selected product count",
    r"basket path:",
    r"meal count:",
    r"generated and verified",
    r"done\.",
]


def has_any(text: str, patterns: list[str]) -> bool:
    return any(re.search(pattern, text, flags=re.IGNORECASE) for pattern in patterns)


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--mode", choices=sorted(REQUIRED), required=True)
    parser.add_argument("response", type=Path)
    args = parser.parse_args()

    text = args.response.read_text(encoding="utf-8", errors="replace")
    lowered = text.lower()
    failures: list[str] = []
    for name, patterns in REQUIRED[args.mode].items():
        if not has_any(lowered, patterns):
            failures.append(f"missing intake topic: {name}")
    for pattern in FORBIDDEN:
        if re.search(pattern, lowered, flags=re.IGNORECASE):
            failures.append(f"appears to have acted before intake: {pattern}")
    if failures:
        for failure in failures:
            print(f"FAIL: {failure}", file=sys.stderr)
        return 1
    print(f"ok: {args.mode} intake response covers required topics")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
