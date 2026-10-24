"""Compare independently recorded manual readings with category rule outputs.

The local review JSON contains operation_id, before, expected, reason. Labels
must be recorded before running this comparison, never generated from categorize().
This diagnostic assistant review is not independent human ground truth.
"""

import argparse
import csv
import json
import sys
from collections import Counter
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from cashflow_baseline.categories import categorize  # noqa: E402
from cashflow_baseline.data import load_companies, sha256  # noqa: E402


def main():
    root = Path(__file__).resolve().parents[3]
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data-dir", type=Path, default=root / "data/cashflow/raw")
    parser.add_argument(
        "--review", type=Path, default=root / "services/ml/artifacts/inflow_v2/manual_review.json"
    )
    parser.add_argument("--output-dir", type=Path, default=root / "services/ml/artifacts/inflow_v2")
    args = parser.parse_args()
    if (
        args.output_dir.resolve() == args.data_dir.resolve()
        or args.data_dir.resolve() in args.output_dir.resolve().parents
    ):
        parser.error("Output cannot overlap source data")
    source_hashes = {
        name: sha256(args.data_dir / name) for name in ("companies.csv", "all_operations.csv")
    }
    companies = load_companies(args.data_dir)
    operations = {op.operation_id: op for c in companies for op in c.operations}
    review = json.loads(args.review.read_text(encoding="utf-8"))
    labels = review["labels"]
    if len({r["operation_id"] for r in labels}) != len(labels):
        raise ValueError("Review IDs must be unique")
    rows = []
    for label in labels:
        op = operations[label["operation_id"]]
        actual = categorize(op)
        rows.append(
            {
                "company_id": op.company,
                "operation_id": op.operation_id,
                "direction": "in" if op.incoming else "out",
                "counterparty_name": op.name,
                "purpose": op.purpose,
                "before": label["before"],
                "manual_expected": label["expected"],
                "after": actual.label,
                "rule": actual.rule,
                "manual_reason": label["reason"],
                "agrees_after": actual.label == label["expected"],
            }
        )
    summary = {
        "reviewer": review["reviewer"],
        "independent_human_labels": False,
        "operations_reviewed": len(rows),
        "companies_reviewed": len({r["company_id"] for r in rows}),
        "before_disagreements": sum(r["before"] != r["manual_expected"] for r in rows),
        "after_disagreements": sum(not r["agrees_after"] for r in rows),
        "before_categories_reviewed": dict(Counter(r["before"] for r in rows)),
        "population_accuracy_measured": False,
        "selection": review["selection"],
        "review_sha256": sha256(args.review),
        "source_sha256": source_hashes,
    }
    assert all(sha256(args.data_dir / name) == value for name, value in source_hashes.items())
    args.output_dir.mkdir(parents=True, exist_ok=True)
    with (args.output_dir / "category_audit.csv").open(
        "w", encoding="utf-8-sig", newline=""
    ) as stream:
        writer = csv.DictWriter(stream, fieldnames=list(rows[0]), delimiter=";")
        writer.writeheader()
        writer.writerows(rows)
    (args.output_dir / "category_audit_summary.json").write_text(
        json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(json.dumps(summary, ensure_ascii=False))
    if summary["after_disagreements"]:
        raise SystemExit("Manual reading disagreements remain; inspect category_audit.csv")


if __name__ == "__main__":
    main()
