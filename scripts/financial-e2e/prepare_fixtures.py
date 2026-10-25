#!/usr/bin/env python3
"""Build deterministic CSV fixtures from the local gitignored dataset."""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import sys
from datetime import date, timedelta
from pathlib import Path

ROOT = Path(__file__).resolve().parent
DEFAULT_OUT = ROOT / "fixtures"


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def resolve_dataset_dir(explicit: str | None) -> Path:
    if explicit:
        return Path(explicit).expanduser().resolve()
    env = Path(__import__("os").environ.get("SALDO_DATASET_DIR", "")).expanduser()
    if env.parts and env.exists():
        return env.resolve()
    candidates = [
        ROOT.parents[1] / "dataset",
        ROOT.parents[1].parent / "hack-rnd-autumn-2026" / "dataset",
        Path("/Users/globalarray/hack-rnd-autumn-2026/dataset"),
    ]
    for candidate in candidates:
        if candidate.exists():
            return candidate.resolve()
    raise SystemExit(
        "dataset directory not found; set SALDO_DATASET_DIR to the local "
        "gitignored dataset root containing statements/C001.csv"
    )


def load_c001(dataset_dir: Path) -> tuple[list[str], list[dict[str, str]], str]:
    path = dataset_dir / "statements" / "C001.csv"
    if not path.is_file():
        raise SystemExit(f"missing {path}")
    source_hash = sha256_file(path)
    with path.open(encoding="utf-8-sig", newline="") as handle:
        reader = csv.DictReader(handle, delimiter=";")
        if reader.fieldnames is None:
            raise SystemExit(f"empty CSV header in {path}")
        rows = list(reader)
        fieldnames = list(reader.fieldnames)
    return fieldnames, rows, source_hash


def write_csv(path: Path, fieldnames: list[str], rows: list[dict[str, str]]) -> str:
    path.parent.mkdir(parents=True, exist_ok=True)
    # UTF-8 BOM + ';' matches dataset_v1 / importer expectations.
    with path.open("w", encoding="utf-8-sig", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames, delimiter=";")
        writer.writeheader()
        writer.writerows(rows)
    return sha256_file(path)


def ordinary_90d(rows: list[dict[str, str]]) -> tuple[list[dict[str, str]], date, date]:
    """Keep 90 calendar days ending on the last day that has an EOD observation."""
    by_date: dict[date, list[dict[str, str]]] = {}
    for row in rows:
        day = date.fromisoformat(row["operation_date"])
        by_date.setdefault(day, []).append(row)
    if not by_date:
        raise SystemExit("C001 has no rows")
    cutoff = max(by_date)
    start = cutoff - timedelta(days=89)
    selected: list[dict[str, str]] = []
    for day in sorted(by_date):
        if start <= day <= cutoff:
            selected.extend(by_date[day])
    if not selected:
        raise SystemExit(f"no rows in window {start}..{cutoff}")
    # Contract: cutoff day must carry EOD (present on every op of that day in dataset).
    cutoff_eods = {row["balance_eod"] for row in by_date[cutoff]}
    if not cutoff_eods or "" in cutoff_eods:
        raise SystemExit(f"cutoff {cutoff} lacks balance_eod")
    return selected, start, cutoff


def short_history(rows: list[dict[str, str]], days: int = 30) -> list[dict[str, str]]:
    by_date: dict[date, list[dict[str, str]]] = {}
    for row in rows:
        day = date.fromisoformat(row["operation_date"])
        by_date.setdefault(day, []).append(row)
    cutoff = max(by_date)
    start = cutoff - timedelta(days=days - 1)
    return [
        row for day, items in sorted(by_date.items()) if start <= day <= cutoff for row in items
    ]


def eod_conflict(rows_90: list[dict[str, str]]) -> list[dict[str, str]]:
    """Create an intra-day EOD disagreement (blocking INVALID_ROW on balance_eod).

    Flipping the last row alone is not enough when the cutoff day has a single
    operation: that only yields BALANCE_DISCONTINUITY (warning). Diagnostics
    block when the same operation_date carries distinct balance_minor values.
    """
    if not rows_90:
        raise SystemExit("ordinary slice empty")
    conflicted = [dict(row) for row in rows_90]
    by_date: dict[str, list[int]] = {}
    for idx, row in enumerate(conflicted):
        by_date.setdefault(row["operation_date"], []).append(idx)
    target_idxs: list[int] | None = None
    for day in sorted(by_date, reverse=True):
        idxs = by_date[day]
        if len(idxs) >= 2:
            target_idxs = idxs
            break
    if not target_idxs:
        raise SystemExit("ordinary slice has no multi-op day for EOD conflict")
    victim = conflicted[target_idxs[-1]]
    try:
        value = float(victim["balance_eod"])
    except ValueError as exc:
        raise SystemExit(f"bad balance_eod: {victim['balance_eod']}") from exc
    victim["balance_eod"] = f"{value + 1.0:.2f}"
    return conflicted


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dataset-dir", default=None)
    parser.add_argument("--out", type=Path, default=DEFAULT_OUT)
    args = parser.parse_args()

    dataset_dir = resolve_dataset_dir(args.dataset_dir)
    fieldnames, rows, source_hash = load_c001(dataset_dir)
    ordinary, start, cutoff = ordinary_90d(rows)

    manifest = {
        "source_c001_sha256": source_hash,
        "dataset_dir": str(dataset_dir),
        "ordinary_window": {
            "start": start.isoformat(),
            "cutoff": cutoff.isoformat(),
            "span_calendar_days": (cutoff - start).days + 1,
            "operation_count": len(ordinary),
            "unique_days": len({row["operation_date"] for row in ordinary}),
        },
        "files": {},
    }

    mapping = {
        "ordinary_90d_c001.csv": ordinary,
        "short_history.csv": short_history(rows, days=30),
        "eod_conflict.csv": eod_conflict(ordinary),
    }
    for name, payload in mapping.items():
        path = args.out / name
        digest = write_csv(path, fieldnames, payload)
        manifest["files"][name] = {
            "path": str(path.relative_to(ROOT)) if path.is_relative_to(ROOT) else str(path),
            "sha256": digest,
            "rows": len(payload),
        }

    manifest_path = args.out / "manifest.json"
    manifest_path.write_text(
        json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(
        json.dumps(
            {"ok": True, "manifest": str(manifest_path), **manifest["ordinary_window"]},
            ensure_ascii=False,
        )
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
