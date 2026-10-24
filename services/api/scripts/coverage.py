#!/usr/bin/env python3
"""Gate own-code coverage, including commands and test Auth, excluding only generated bindings."""

import sys
from pathlib import Path


def measure(profile):
    blocks = {}
    for row in profile.read_text().splitlines()[1:]:
        position, statements, count = row.split()
        if "/internal/gen/" in position:
            continue
        size, previous = blocks.get(position, (int(statements), 0))
        assert size == int(statements)
        blocks[position] = size, max(previous, int(count))
    # Cross-package coverage profiles can contain the same instrumented block
    # multiple times; union counts, never count repeated zero records as new code.
    own = profile.with_name("coverage-own.out")
    own.write_text(
        "mode: atomic\n" + "".join(f"{p} {n} {c}\n" for p, (n, c) in sorted(blocks.items()))
    )
    total = sum(n for n, _ in blocks.values())
    covered = sum(n for n, c in blocks.values() if c)
    lines = {}
    for position, (statements, count) in blocks.items():
        if not statements:
            continue
        file, location = position.rsplit(":", 1)
        local = Path(file.split("/services/api/", 1)[1])
        source = local.read_text().splitlines()
        start, end = (int(part.split(".")[0]) for part in location.split(","))
        for line in range(start, end + 1):
            text = source[line - 1].strip()
            if not text or text.startswith("//") or text in {"{", "}", "})", ")"}:
                continue
            key = file, line
            lines[key] = lines.get(key, False) or count > 0
    statement_coverage = 100 * covered / total
    line_coverage = 100 * sum(lines.values()) / len(lines)
    print(f"Own statements: {covered}/{total} = {statement_coverage:.2f}%")
    print(f"Own executable block lines: {sum(lines.values())}/{len(lines)} = {line_coverage:.2f}%")
    print("Excluded: internal/gen only. Included: cmd, production adapters, test Auth/PKI.")
    assert statement_coverage >= 85 and line_coverage >= 85, "coverage gate below 85%"


if __name__ == "__main__":
    measure(Path(sys.argv[1] if len(sys.argv) > 1 else "coverage.out"))
