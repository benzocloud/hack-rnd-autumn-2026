"""Reproduce frozen windows and compare receipt forecasts without dropping flows."""

import argparse
import html
import json
from collections import Counter
from dataclasses import replace
from pathlib import Path

from .cli import ROOT, read_config
from .data import DataError, load_companies, sha256
from .evaluation import aggregate, all_metrics, evaluate, first_gap
from .inflow import INFLOW_VARIANTS, adaptive_predict, inflow_predict, seasonal_predict
from .recurring_experiment import csv_rows, export_categories
from .report import config_dict, dump_json, write_eligibility, write_predictions


def records_for(references, companies, method):
    result, explanations = [], []
    for record in references:
        if method in {"adaptive_inflow", "seasonal_receipts"}:
            function = adaptive_predict if method == "adaptive_inflow" else seasonal_predict
            predictions, evidence = function(
                companies[record.company], record.history, len(record.truth)
            )
            explanations.append(
                {"company_id": record.company, "cutoff": record.cutoff.isoformat(), **evidence}
            )
        else:
            predictions = inflow_predict(record.history, len(record.truth), method)
        result.append(replace(record, method=method, predictions=predictions))
    return result, explanations


def markdown_table(headers, rows):
    return "\n".join(
        ["| " + " | ".join(headers) + " |", "| " + " | ".join(["---"] * len(headers)) + " |"]
        + ["| " + " | ".join(map(str, row)) + " |" for row in rows]
    )


def write_report(output, metrics, trials, manifest, diagnostics):
    lines = [
        "# Поступления после проверки категорий",
        "",
        f"Выбор по validation: **{manifest['selected_by_validation']}**. Все {manifest['paired_windows']} среза сохранены.",
        "",
        "## Поздняя диагностическая проверка",
        "",
    ]
    rows = []
    for row in metrics["by_split"]:
        if row["split"] == "evaluation":
            gap = row["new_gap"]
            rows.append(
                [
                    row["method"],
                    f"{row['horizon_inflow_wape']:.2%}",
                    f"{row['inflow_wape']:.2%}",
                    f"{row['balance_mae']:,.2f}",
                    f"{gap['tp']}/{gap['fn']}/{gap['fp']}",
                ]
            )
    lines += [
        markdown_table(
            [
                "Метод",
                "WAPE суммы поступлений",
                "Дневной WAPE",
                "MAE остатка, ден. ед.",
                "Новый минус TP/FN/FP",
            ],
            rows,
        ),
        "",
        "## Все статические попытки на validation",
        "",
    ]
    rows = [
        [r["method"], f"{r['horizon_inflow_wape']:.2%}", f"{r['balance_mae']:,.2f}"]
        for r in trials
        if r["split"] == "validation"
    ]
    lines += [
        markdown_table(["Метод", "WAPE суммы поступлений", "MAE остатка"], rows),
        "",
        "## Ограничения",
        "",
        "Параметры подбирались по validation; первые пять вариантов, затем три варианта 12-недельного сглаживания, локальный выбор и сезонный вариант показаны полностью. Поздний период ранее изучен и открывался между гипотезами: это исследование, не независимый holdout.",
        "",
        "Сезонный вариант сочетает операционную выручку текущего уровня и того же дня недели год назад. Финансирование, свои переводы и unknown сохраняют среднее за 90 дней. Ничто не удаляется из исходной истории и не неттируется между счетами. При отсутствии годовых данных действует fallback mean_90.",
        "",
        "Расходы всех новых методов фиксированы как mean_90. Улучшение поступлений не гарантирует улучшение остатка: оценивать обе ошибки и ложные тревоги. По умолчанию исходный run_baseline.py сохранён. Новый метод остаётся экспериментальным, вероятность риска не оценивается.",
        "",
        "Проверка категорий — ручное чтение ассистентом целевых примеров, а не независимая разметка двумя людьми. Unknown не равен ошибке. См. category_audit.csv и category_audit_summary.json; без локальной ручной разметки статистика аудита недоступна.",
        "",
        "## Диагностика относительно mean_90",
        "",
        "```json",
        json.dumps(diagnostics, ensure_ascii=False, indent=2),
        "```",
        "",
    ]
    markdown = "\n".join(lines)
    (output / "comparison.md").write_text(markdown, encoding="utf-8")
    (output / "comparison.html").write_text(
        '<!doctype html><html lang="ru"><meta charset="utf-8"><title>Прогноз поступлений</title><style>body{font:16px system-ui;max-width:1200px;margin:36px auto;padding:0 24px}pre{white-space:pre-wrap;line-height:1.55}</style><pre>'
        + html.escape(markdown)
        + "</pre></html>",
        encoding="utf-8",
    )


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data-dir", type=Path, default=ROOT / "data/cashflow/raw")
    parser.add_argument(
        "--reference-dir", type=Path, default=ROOT / "services/ml/artifacts/baseline_43d7a28"
    )
    parser.add_argument("--output-dir", type=Path, default=ROOT / "services/ml/artifacts/inflow_v2")
    args = parser.parse_args(argv)
    output, data, reference = (
        args.output_dir.resolve(),
        args.data_dir.resolve(),
        args.reference_dir.resolve(),
    )
    for protected in (data, reference):
        if output == protected or protected in output.parents or output in protected.parents:
            parser.error("Output must not overlap sources or frozen reference")
    baseline_manifest = json.loads((reference / "manifest.json").read_text(encoding="utf-8"))
    hashes = {name: sha256(data / name) for name in ("all_operations.csv", "companies.csv")}
    if hashes != baseline_manifest["source_sha256"]:
        raise DataError("Source differs from frozen baseline")
    config = read_config(Path(__file__).parents[1] / "baseline_config.json")
    if config_dict(config) != baseline_manifest["config"]:
        raise DataError("Config differs from frozen baseline")
    companies = {c.company_id: c for c in load_companies(data)}
    print("Reproducing baseline windows...", flush=True)
    old, skipped, eligibility = evaluate(list(companies.values()), config)
    output.mkdir(parents=True, exist_ok=True)
    write_predictions(output, old)
    if (output / "predictions.csv").read_bytes() != (reference / "predictions.csv").read_bytes():
        raise DataError("Frozen baseline forecasts changed")
    if all_metrics(old) != json.loads((reference / "metrics.json").read_text(encoding="utf-8")):
        raise DataError("Frozen baseline metrics changed")
    references = [r for r in old if r.method == "mean_90"]
    earlier = [r for r in references if r.split != "evaluation"]
    trials, extra, explanations = [], [], {}
    for method in INFLOW_VARIANTS:
        predicted, _ = records_for(earlier, companies, method)
        for split in ("development", "validation"):
            trials.append(
                {
                    "method": method,
                    "split": split,
                    **aggregate([r for r in predicted if r.split == split]),
                }
            )
    winner = min(
        (r for r in trials if r["split"] == "validation"),
        key=lambda r: (r["horizon_inflow_wape"], r["balance_mae"]),
    )["method"]
    for method in (winner, "adaptive_inflow", "seasonal_receipts"):
        print(f"Receipt comparison: {method}", flush=True)
        predicted, evidence = records_for(earlier, companies, method)
        extra += predicted
        explanations[method] = evidence
        if method != winner:
            for split in ("development", "validation"):
                trials.append(
                    {
                        "method": method,
                        "split": split,
                        **aggregate([r for r in predicted if r.split == split]),
                    }
                )
    options = trials + [
        {
            "method": method,
            "split": "validation",
            **aggregate([r for r in old if r.method == method and r.split == "validation"]),
        }
        for method in ("mean_90", "weekday_8w")
    ]
    selected = min(
        (r for r in options if r["split"] == "validation"),
        key=lambda r: (r["horizon_inflow_wape"], r["balance_mae"]),
    )["method"]
    dump_json(
        output / "selection.json",
        {
            "selected": selected,
            "criterion": "validation horizon_inflow_wape, then balance_mae",
            "trials": options,
        },
    )
    # Re-run late diagnostics after making a deterministic validation-only choice.
    for method in (winner, "adaptive_inflow", "seasonal_receipts"):
        predicted, evidence = records_for(
            [r for r in references if r.split == "evaluation"], companies, method
        )
        extra += predicted
        explanations[method] += evidence
    expected_keys = {(r.company, r.cutoff) for r in references}
    for method in (winner, "adaptive_inflow", "seasonal_receipts"):
        assert {(r.company, r.cutoff) for r in extra if r.method == method} == expected_keys
    records = old + extra
    metrics = all_metrics(records)
    write_predictions(output, records)
    write_eligibility(output, eligibility)
    dump_json(output / "metrics.json", metrics)
    dump_json(output / "explanations.json", explanations)
    export_categories(output, list(companies.values()))
    base = {(r.company, r.cutoff): r for r in references}
    chosen = [r for r in records if r.method == selected and r.split == "evaluation"]
    wins = Counter()
    cases = []
    for record in chosen:
        original = base[(record.company, record.cutoff)]
        actual = sum(d.inflow for d in record.truth)
        before = abs(sum(d.inflow for d in original.predictions) - actual)
        after = abs(sum(d.inflow for d in record.predictions) - actual)
        wins["improved" if after < before else "worsened" if after > before else "equal"] += 1
        if record.initial_balance >= 0 and first_gap(record.truth):
            cases.append(
                {
                    "company_id": record.company,
                    "cutoff": record.cutoff.isoformat(),
                    "actual_first_gap": first_gap(record.truth).isoformat(),
                    "predicted_first_gap": first_gap(record.predictions).isoformat()
                    if first_gap(record.predictions)
                    else "",
                    "actual_inflow_cents": actual,
                    "predicted_inflow_cents": sum(d.inflow for d in record.predictions),
                }
            )
    csv_rows(output / "positive_cases.csv", cases, list(cases[0]))
    by_company = defaultdict_metrics(chosen, [r for r in references if r.split == "evaluation"])
    diagnostics = {
        "horizon_inflow_error_windows": dict(wins),
        "company_horizon_inflow_wape": by_company,
        "positive_cases": cases,
    }
    dump_json(output / "diagnostics.json", diagnostics)
    manifest = {
        "source_sha256": hashes,
        "config": config_dict(config),
        "config_sha256": sha256(Path(__file__).parents[1] / "baseline_config.json"),
        "implementation_sha256": {p.name: sha256(p) for p in Path(__file__).parent.glob("*.py")},
        "paired_windows": len(references),
        "skipped": skipped,
        "selected_by_validation": selected,
        "baseline_predictions_identical": True,
        "baseline_metrics_identical": True,
        "evaluation_is_untouched_holdout": False,
        "probability_estimated": False,
        "category_accuracy_measured": False,
        "forecast_record_count": len(records),
    }
    assert all(sha256(data / name) == value for name, value in hashes.items())
    dump_json(output / "manifest.json", manifest)
    write_report(output, metrics, options, manifest, diagnostics)
    print(
        json.dumps(
            {
                "output": str(output),
                "selected": selected,
                "evaluation": [r for r in metrics["by_split"] if r["split"] == "evaluation"],
            },
            ensure_ascii=False,
        )
    )
    return 0


def defaultdict_metrics(chosen, baseline):
    results = Counter()
    for company_id in sorted({r.company for r in chosen}):
        new = aggregate([r for r in chosen if r.company == company_id])
        old = aggregate([r for r in baseline if r.company == company_id])
        a, b = new["horizon_inflow_wape"], old["horizon_inflow_wape"]
        if a is None or b is None:
            results["undefined_zero_actual"] += 1
        else:
            results["improved" if a < b else "worsened" if a > b else "equal"] += 1
    return dict(results)
