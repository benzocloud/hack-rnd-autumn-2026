"""Compare false-warning mechanisms and one fixed past-performance gate."""

import argparse
import csv
import json
from collections import Counter
from dataclasses import replace
from pathlib import Path

from .balance_diagnostics import grouped_metrics, product_metrics, write_csv
from .balance_experiment import saved_control, selection
from .balance_model import combine, expense_calendar
from .cli import ROOT, read_config
from .data import DataError, load_companies, parse_money, sha256
from .evaluation import all_metrics, evaluate
from .expense_gate import gated_expenses
from .expense_warnings import classify_warnings
from .report import config_dict, dump_json, write_eligibility, write_predictions


def verify_export(output, reference):
    """Separate CSV pass: compare source rows, integer balances and exact dates."""
    old = []
    balances, keys, dates = {}, {}, {}
    rows = 0
    with (output / "predictions.csv").open(encoding="utf-8-sig", newline="") as stream:
        for row in csv.DictReader(stream, delimiter=";"):
            rows += 1
            key = row["company_id"], row["cutoff_date"], row["method"]
            before = balances.get(key, parse_money(row["initial_balance"]))
            inc, out = parse_money(row["predicted_inflow"]), parse_money(row["predicted_outflow"])
            balance = parse_money(row["predicted_balance"])
            if min(inc, out) < 0 or balance != before + inc - out:
                raise DataError("Exported balance arithmetic failed")
            balances[key] = balance
            keys.setdefault(row["method"], set()).add(key[:2])
            current = dates.setdefault(key, [])
            current.append(row["date"])
            if row["method"] in {"mean_90", "weekday_8w"}:
                old.append(row)
    with reference.open(encoding="utf-8-sig", newline="") as stream:
        expected = list(csv.DictReader(stream, delimiter=";"))
    if old != expected:
        raise DataError("Original baseline rows changed in final export")
    from datetime import date, timedelta

    for key, values in dates.items():
        cutoff = date.fromisoformat(key[1])
        if values != [(cutoff + timedelta(days=i)).isoformat() for i in range(1, 31)]:
            raise DataError("Forecast dates changed")
    original_keys = {(r["company_id"], r["cutoff_date"]) for r in expected}
    if any(value != original_keys for value in keys.values()):
        raise DataError("Methods changed the window set")
    return {
        "daily_rows": rows,
        "paired_windows": len(original_keys),
        "baseline_rows_identical": True,
        "cash_arithmetic_exact": True,
        "dates_exact": True,
    }


def write_report(output, metrics, summary, choice, evidence):
    lines = [
        "# Ложные тревоги расходов и проверка past gate",
        "",
        "Одна фиксированная гипотеза; все прежние окна. Поздняя часть изучена ранее и не является независимым holdout.",
        "",
        "## Тревоги старого expense_only относительно mean_90",
        "",
        "```json",
        json.dumps(summary, ensure_ascii=False, indent=2),
        "```",
        "",
        "## Продуктовые метрики",
        "",
        "| Часть | Метод | MAE остатка | Медиана компаний | TP/FN/FP | Ошибка даты TP, дни | Доступное время TP, дни | Ошибка глубины |",
        "| --- | --- | ---: | ---: | --- | ---: | ---: | ---: |",
    ]
    for row in metrics["by_split"]:
        if row["split"] == "development":
            continue
        gap = row["new_gap"]
        lines.append(
            f"| {row['split']} | {row['method']} | {row['balance_mae']:.2f} | {row['company_median_balance_mae']:.2f} | {gap['tp']}/{gap['fn']}/{gap['fp']} | {gap['mean_first_gap_date_error_days_tp']} | {row['mean_warning_days']} | {row['new_gap_depth_mae']} |"
        )
    used = Counter(e["reason"] for e in evidence)
    lines += [
        "",
        "Выбор validation: **" + choice["selected"] + "**. Default — mean_90.",
        "",
        "Gate: два прошлых неперекрывающихся окна по 30 дней, минимум 5% улучшения ошибки дневного остатка и без роста прошлых новых FP/FN. При неполной истории — baseline.",
        "",
        "Причины выбора/fallback: " + str(dict(used)),
        "",
        "Сопоставление календарного события с ближайшей фактической группой в пределах семи дней — только диагностика, не доказательство обязательства/неоплаты. Компоненты ошибок могут компенсироваться. Подробные случаи остаются локальными.",
        "",
        "Повторное использование validation и уже изученной поздней истории ограничивает выводы. Новая гипотеза не даёт права выбирать default по этой поздней диагностике. Нет нового календаря праздников, графиков долга или консолидации счетов.",
        "",
    ]
    (output / "comparison.md").write_text("\n".join(lines), encoding="utf-8")


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data-dir", type=Path, default=ROOT / "data/cashflow/raw")
    parser.add_argument(
        "--reference-dir", type=Path, default=ROOT / "services/ml/artifacts/baseline_43d7a28"
    )
    parser.add_argument(
        "--previous-dir", type=Path, default=ROOT / "services/ml/artifacts/balance_v1_verified"
    )
    parser.add_argument(
        "--output-dir", type=Path, default=ROOT / "services/ml/artifacts/expense_warnings_v1"
    )
    args = parser.parse_args(argv)
    data, reference, previous, output = (
        p.resolve() for p in (args.data_dir, args.reference_dir, args.previous_dir, args.output_dir)
    )
    for protected in (data, reference, previous):
        if output == protected or output in protected.parents or protected in output.parents:
            parser.error("Output overlaps input or saved experiments")
    if output.exists():
        parser.error("Choose a fresh output directory; existing results are preserved")
    hashes = {n: sha256(data / n) for n in ("companies.csv", "all_operations.csv")}
    for folder in (reference, previous):
        if (
            json.loads((folder / "manifest.json").read_text(encoding="utf-8"))["source_sha256"]
            != hashes
        ):
            raise DataError("Input differs from saved experiments")
    implementation = {p.name: sha256(p) for p in sorted(Path(__file__).parent.glob("*.py"))}
    config = read_config(Path(__file__).parents[1] / "baseline_config.json")
    frozen = json.loads((reference / "manifest.json").read_text(encoding="utf-8"))
    if config_dict(config) != frozen["config"]:
        raise DataError("Baseline configuration changed")
    companies = {c.company_id: c for c in load_companies(data)}
    print("Reproducing baseline...", flush=True)
    baseline, skipped, eligibility = evaluate(list(companies.values()), config)
    output.mkdir(parents=True)
    write_predictions(output, baseline)
    if (output / "predictions.csv").read_bytes() != (
        reference / "predictions.csv"
    ).read_bytes() or all_metrics(baseline) != json.loads(
        (reference / "metrics.json").read_text(encoding="utf-8")
    ):
        raise DataError("Baseline reproduction failed")
    refs = [r for r in baseline if r.method == "mean_90"]
    calendars = saved_control(previous / "predictions.csv", "expense_only", refs)
    if not calendars:
        raise DataError("Previous expense_only controls are required")
    old = {(r.company, r.cutoff): r for r in calendars}
    patterns, cache, evidence, candidates = {}, {}, [], []
    choice = None
    for late in (False, True):
        for index, r in enumerate([r for r in refs if (r.split == "evaluation") == late]):
            if index % 100 == 0:
                print(f"{'Late' if late else 'Past/validation'} {index}", flush=True)
            c = companies[r.company]
            key = r.company, r.cutoff
            outgoing, fits, events = expense_calendar(c, r.history, 30)
            if (
                combine(r.history, [p.inflow for p in r.predictions], outgoing)
                != old[key].predictions
            ):
                raise DataError("Original expense calendar changed")
            patterns[key] = fits
            cache[key] = outgoing
            predictions, detail = gated_expenses(c, r.history, 30, outgoing, cache)
            evidence.append(
                {
                    "company_id": r.company,
                    "cutoff": r.cutoff.isoformat(),
                    "split": r.split,
                    **detail,
                }
            )
            candidates.append(replace(r, method="expense_past_gate", predictions=predictions))
        if not late:
            choice = selection(baseline + calendars + candidates, "validation")
            dump_json(output / "selection_before_evaluation.json", choice)
            print("Validation choice: " + choice["selected"], flush=True)
    rows, summary = classify_warnings(refs, calendars, companies, patterns)
    dump_json(output / "warning_cases.json", rows)
    dump_json(output / "warning_summary.json", summary)
    write_csv(
        output / "warning_cases.csv",
        [
            {k: v for k, v in row.items() if k not in {"events", "signed_components_currency"}}
            for row in rows
        ],
    )
    records = baseline + calendars + candidates
    metrics = all_metrics(records)
    metrics["by_split"] = [
        {
            "split": split,
            "method": method,
            **product_metrics([r for r in records if r.split == split and r.method == method]),
        }
        for split in ("development", "validation", "evaluation")
        for method in ("mean_90", "weekday_8w", "expense_only", "expense_past_gate")
    ]
    metrics["by_receipt_type"] = grouped_metrics(records)
    metrics["late_gates_diagnostic_only"] = selection(records, "evaluation")
    dump_json(output / "metrics.json", metrics)
    dump_json(output / "gate_evidence.json", evidence)
    write_predictions(output, records)
    write_eligibility(output, eligibility)
    audit = verify_export(output, reference / "predictions.csv")
    if hashes != {n: sha256(data / n) for n in hashes} or implementation != {
        p.name: sha256(p) for p in sorted(Path(__file__).parent.glob("*.py"))
    }:
        raise DataError("Sources or implementation changed during experiment")
    dump_json(output / "audit.json", audit)
    dump_json(
        output / "manifest.json",
        {
            "source_sha256": hashes,
            "implementation_sha256": implementation,
            "config": config_dict(config),
            "paired_windows": len(refs),
            "default": "mean_90",
            "selection": choice["selected"],
            "evaluation_is_untouched_holdout": False,
            "previous_predictions_sha256": sha256(previous / "predictions.csv"),
            "skipped": skipped,
        },
    )
    write_report(output, metrics, summary, choice, evidence)
    print(
        json.dumps({"output": str(output), "choice": choice["selected"], "audit": audit}),
        flush=True,
    )
    return 0
