"""Independently audit exported arithmetic and receipt-date support, without fitting."""

import argparse
import csv
import json
import sys
from collections import Counter, defaultdict
from datetime import date, timedelta
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from cashflow_baseline.categories import categorize  # noqa: E402
from cashflow_baseline.data import DataError, load_companies, money, parse_money, sha256  # noqa: E402
from cashflow_baseline.report import dump_json  # noqa: E402

ROOT = Path(__file__).resolve().parents[3]


def audit(output, reference, data_dir=ROOT / "data/cashflow/raw"):
    manifest = json.loads((output / "manifest.json").read_text(encoding="utf-8"))
    if {name: sha256(data_dir / name) for name in manifest["source_sha256"]} != manifest[
        "source_sha256"
    ]:
        raise DataError("Audit source does not match experiment manifest")
    baseline_lines = []
    window_keys = defaultdict(set)
    group = {}
    with (output / "error_decomposition.csv").open(encoding="utf-8-sig") as stream:
        for r in csv.DictReader(stream, delimiter=";"):
            group[r["company_id"], r["cutoff"]] = r["receipt_type"]
    sums = defaultdict(Counter)
    balances, dates = {}, {}
    rows = 0
    with (output / "predictions.csv").open(encoding="utf-8-sig", newline="") as stream:
        reader = csv.DictReader(stream, delimiter=";")
        for r in reader:
            rows += 1
            key = r["company_id"], r["cutoff_date"], r["method"]
            kind = group[key[:2]]
            bucket = sums[r["split"], r["method"], kind]
            actual = parse_money(r["actual_inflow"])
            predicted = parse_money(r["predicted_inflow"])
            out = parse_money(r["predicted_outflow"])
            balance = parse_money(r["predicted_balance"])
            previous = balances.get(key, parse_money(r["initial_balance"]))
            if min(predicted, out) < 0 or balance != previous + predicted - out:
                raise DataError("Exported cash arithmetic failed")
            if r["date"] in dates.setdefault(key, set()):
                raise DataError("Duplicate forecast date")
            dates[key].add(r["date"])
            balances[key] = balance
            window_keys[r["method"]].add(key[:2])
            bucket["days"] += 1
            bucket["actual_positive_days"] += actual > 0
            bucket["predicted_nonzero_days"] += predicted > 0
            bucket["nonzero_support_tp"] += actual > 0 and predicted > 0
            bucket["nonzero_support_fp"] += actual == 0 and predicted > 0
            bucket["nonzero_support_fn"] += actual > 0 and predicted == 0
            bucket["predicted_on_zero_actual_cents"] += predicted if not actual else 0
            bucket["absolute_daily_error_cents"] += abs(actual - predicted)
            bucket["actual_inflow_cents"] += actual
            bucket["absolute_balance_error_cents"] += abs(
                parse_money(r["actual_balance"]) - balance
            )
            if r["method"] in {"mean_90", "weekday_8w"}:
                baseline_lines.append(r)
    with reference.open(encoding="utf-8-sig", newline="") as stream:
        original = list(csv.DictReader(stream, delimiter=";"))
    if original != baseline_lines:
        raise DataError("Final exported baseline rows differ from frozen reference")
    original_keys = {(r["company_id"], r["cutoff_date"]) for r in original}
    if any(keys != original_keys for keys in window_keys.values()):
        raise DataError("Methods use different windows")
    for key, ds in dates.items():
        cutoff = date.fromisoformat(key[1])
        if ds != {(cutoff + timedelta(days=i)).isoformat() for i in range(1, 31)}:
            raise DataError("A forecast lost or changed daily dates")
    records = []
    for (split, method, kind), stats in sorted(sums.items()):
        denominator = stats["nonzero_support_tp"] + stats["nonzero_support_fp"]
        records.append(
            {
                "split": split,
                "method": method,
                "receipt_type": kind,
                **dict(stats),
                "nonzero_support_precision": stats["nonzero_support_tp"] / denominator
                if denominator
                else None,
                "warning": "Nonzero expectation is not a receipt-date decision or calibrated probability.",
            }
        )
    # Evaluate the separate >=0.5 occurrence decision where a score is actually fit.
    # Irregular fallbacks have no such decision and are explicitly counted separately.
    with (output / "explanations.json").open(encoding="utf-8") as stream:
        explanations = json.load(stream)
    actual_channels = defaultdict(Counter)

    for company in load_companies(data_dir):
        for op in company.operations:
            if op.incoming:
                actual_channels[op.company, op.day.isoformat()][categorize(op).label] += op.amount
    decisions = defaultdict(Counter)
    for detail in explanations:
        cutoff = detail["cutoff"]
        cid = detail["company_id"]

        day = date.fromisoformat(cutoff)
        split = (
            "development"
            if day.year < 2026
            else "validation"
            if day < date(2026, 4, 1)
            else "evaluation"
        )
        for method in ("channel_expected", "channel_events", "logistic_expected"):
            for label, evidence in detail[method]["channels"].items():
                if label == "other":
                    continue
                scores = evidence.get("scores", [])
                stats = decisions[split, method]
                if not scores:
                    stats["channels_without_occurrence_decision"] += 1
                    continue
                for i, score in enumerate(scores, 1):
                    observed = (
                        actual_channels[cid, (day + timedelta(days=i)).isoformat()][label] > 0
                    )
                    predicted = score >= 0.5
                    name = (
                        ("tp" if predicted else "fn") if observed else ("fp" if predicted else "tn")
                    )
                    stats[name] += 1
                    stats["brier_score_sum"] += (score - int(observed)) ** 2
                    stats["days"] += 1
    decision_rows = []
    for (split, method), stats in sorted(decisions.items()):
        decision_rows.append(
            {
                "split": split,
                "method": method,
                **dict(stats),
                "precision": stats["tp"] / (stats["tp"] + stats["fp"])
                if stats["tp"] + stats["fp"]
                else None,
                "recall": stats["tp"] / (stats["tp"] + stats["fn"])
                if stats["tp"] + stats["fn"]
                else None,
                "brier_mean": stats["brier_score_sum"] / stats["days"] if stats["days"] else None,
                "calibrated": False,
            }
        )
    result = {
        "rows_checked": rows,
        "paired_windows": len(original_keys),
        "baseline_final_rows_identical": True,
        "cash_arithmetic_exact": True,
        "date_support_by_type": records,
        "occurrence_decisions": decision_rows,
        "predictions_sha256": sha256(output / "predictions.csv"),
    }
    dump_json(output / "independent_audit.json", result)
    examples = [
        "# Примеры объяснения ожидаемого сценария",
        "",
        "Это гипотезы по истории одного счёта, не подтверждённые обязательства и не вероятности.",
        "",
    ]
    wanted = {
        ("C124", "2026-06-25"),
        ("C004", "2026-06-25"),
        ("C032", "2026-08-24"),
        ("C126", "2026-08-24"),
    }
    for detail in explanations:
        if (detail["company_id"], detail["cutoff"]) not in wanted:
            continue
        message = detail["user_explanation"]
        examples += [
            f"## {detail['company_id']}, расчёт на {detail['cutoff']}",
            "",
            message["explanation"],
            "",
            "Причина риска в расчёте: ожидаемые списания "
            + money(message["predicted_outflow_cents"])
            + ", поступления "
            + money(message["predicted_inflow_cents"])
            + "; минимальный остаток "
            + money(message["minimum_balance_cents"])
            + ".",
            "",
            "Ключевые календарные гипотезы (по прошлым повторениям):",
            "",
        ]
        for event in sorted(detail["expense_events"], key=lambda e: -e["amount_cents"])[:4]:
            examples.append(
                f"- {event['date']}: {event['category']}, {money(event['amount_cents'])}."
            )
        if not detail["expense_events"]:
            examples.append(
                "- Устойчивые месячные обязательства не найдены; расходы представлены прошлым средним."
            )
        examples += [
            "",
            "Неопределённые поступления: "
            + (
                ", ".join(message["uncertain_receipt_channels"])
                or "точная сумма и задержки даже регулярной выручки"
            )
            + ". Они не подтверждены будущими счетами/договорами. При задержке или невнесении денег остаток может быть ниже.",
            "",
            "Осторожная траектория дана отдельно как стресс-сценарий; она не подтверждает неизбежность дефицита.",
            "",
        ]
    (output / "user_examples.md").write_text("\n".join(examples), encoding="utf-8")
    return {
        k: result[k]
        for k in (
            "rows_checked",
            "paired_windows",
            "baseline_final_rows_identical",
            "cash_arithmetic_exact",
        )
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--output-dir", type=Path, default=ROOT / "services/ml/artifacts/balance_v1_verified"
    )
    parser.add_argument(
        "--reference",
        type=Path,
        default=ROOT / "services/ml/artifacts/baseline_43d7a28/predictions.csv",
    )
    parser.add_argument("--data-dir", type=Path, default=ROOT / "data/cashflow/raw")
    args = parser.parse_args()
    print(json.dumps(audit(args.output_dir, args.reference, args.data_dir)))
