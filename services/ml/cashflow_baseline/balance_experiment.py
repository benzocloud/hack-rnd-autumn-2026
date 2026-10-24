"""Fixed-window product experiment; all detailed outputs stay local."""

import argparse
import csv
import json
import os
from collections import Counter, defaultdict
from dataclasses import replace
from datetime import date
from pathlib import Path
from statistics import mean

from .balance_diagnostics import (
    compare_companies,
    diagnostic_row,
    grouped_metrics,
    human_sample,
    product_metrics,
    write_csv,
)
from .balance_model import (
    RECEIPT_METHODS,
    cautious_forecast,
    combine,
    expense_calendar,
    incoming_forecast,
    receipt_type,
)
from .cli import ROOT, read_config
from .data import DataError, load_companies, money, parse_money, sha256
from .evaluation import all_metrics, evaluate, first_gap
from .models import Prediction
from .report import config_dict, dump_json, write_eligibility, write_predictions

PROTOCOL_VERSION = "balance_v1_fixed_20261024"


def gates(candidate, baseline, candidate_groups, baseline_groups, counts):
    """Product constraints fixed in the plan, never derived from evaluation."""
    failures = []
    if candidate["balance_mae"] > baseline["balance_mae"] * 0.95:
        failures.append("balance_mae_less_than_5pct_improvement")
    for key in ("company_median_balance_mae", "macro_normalized_balance_mae"):
        if candidate[key] > baseline[key]:
            failures.append(key)
    for key in ("fn", "fp"):
        if candidate["new_gap"][key] > baseline["new_gap"][key]:
            failures.append("new_gap_" + key)
    if baseline["new_gap_depth_mae"] is not None and (
        candidate["new_gap_depth_mae"] is None
        or candidate["new_gap_depth_mae"] > baseline["new_gap_depth_mae"]
    ):
        failures.append("new_gap_depth_mae")
    if counts.get("improved", 0) + counts.get("equal", 0) < sum(counts.values()) / 2:
        failures.append("majority_companies_worsened")
    for kind, group in baseline_groups.items():
        if candidate_groups[kind]["balance_mae"] > group["balance_mae"] * 1.10:
            failures.append("receipt_type_" + kind)
    # Common detected cases: a candidate must not lose warning lead or date accuracy.
    old_cases = {
        (r["company_id"], r["cutoff"]): r
        for r in baseline["gap_cases"]
        if r["actual_date"] and r["predicted_date"]
    }
    new_cases = {
        (r["company_id"], r["cutoff"]): r
        for r in candidate["gap_cases"]
        if r["actual_date"] and r["predicted_date"]
    }
    for key in old_cases.keys() & new_cases.keys():
        old, new = old_cases[key], new_cases[key]
        actual = date.fromisoformat(old["actual_date"])
        if abs((date.fromisoformat(new["predicted_date"]) - actual).days) > abs(
            (date.fromisoformat(old["predicted_date"]) - actual).days
        ):
            failures.append("common_tp_date_error")
    return sorted(set(failures))


def selection(records, split):
    by_method = defaultdict(list)
    for r in records:
        if r.split == split:
            by_method[r.method].append(r)
    baseline = by_method["mean_90"]
    old = product_metrics(baseline)
    old_groups = {
        kind: product_metrics([r for r in baseline if receipt_type(r.history) == kind])
        for kind in {receipt_type(r.history) for r in baseline}
    }
    trials = []
    for method, rs in sorted(by_method.items()):
        metrics = product_metrics(rs)
        groups = {
            kind: product_metrics([r for r in rs if receipt_type(r.history) == kind])
            for kind in old_groups
        }
        counts = compare_companies(rs, baseline)
        failures = gates(metrics, old, groups, old_groups, counts)
        trials.append(
            {
                "method": method,
                "failures": failures,
                "passes": not failures,
                "company_comparison": counts,
                "metrics": metrics,
            }
        )
    passed = [r for r in trials if r["passes"]]
    winner = (
        min(
            passed,
            key=lambda r: (
                r["metrics"]["balance_mae"],
                r["metrics"]["new_gap"]["fp"],
                r["metrics"]["new_gap"]["fn"],
                r["method"],
            ),
        )["method"]
        if passed
        else "mean_90"
    )
    return {"split": split, "selected": winner, "trials": trials}


def saved_control(path, method, references):
    if not path.exists():
        return []
    rows = defaultdict(list)
    with path.open(encoding="utf-8-sig", newline="") as stream:
        for row in csv.DictReader(stream, delimiter=";"):
            if row["method"] == method:
                rows[row["company_id"], row["cutoff_date"]].append(row)
    result = []
    for r in references:
        values = rows.pop((r.company, r.cutoff.isoformat()), [])
        if len(values) != len(r.truth):
            raise DataError("Saved control windows do not match frozen baseline")
        predictions = []
        for actual, row in zip(r.truth, values):
            if row["date"] != actual.day.isoformat() or any(
                parse_money(row["actual_" + k]) != getattr(actual, k)
                for k in ("inflow", "outflow", "balance")
            ):
                raise DataError("Saved control truth does not match baseline")
            predictions.append(
                Prediction(
                    actual.day,
                    parse_money(row["predicted_inflow"]),
                    parse_money(row["predicted_outflow"]),
                    parse_money(row["predicted_balance"]),
                )
            )
        result.append(replace(r, method=method, predictions=predictions))
    if rows:
        raise DataError("Saved control has extra windows")
    return result


def explain(record, evidence):
    gap = first_gap(record.predictions)
    uncertain = [
        label
        for label, detail in evidence["channels"].items()
        if detail["mode"]
        in {"irregular_mean_fallback", "other_mean_preserved", "no_stable_slots_expected_fallback"}
    ]
    return {
        "company_id": record.company,
        "cutoff": record.cutoff.isoformat(),
        "method": record.method,
        "first_negative_date": gap.isoformat() if gap else None,
        "minimum_balance_cents": min(p.balance for p in record.predictions),
        "predicted_inflow_cents": sum(p.inflow for p in record.predictions),
        "predicted_outflow_cents": sum(p.outflow for p in record.predictions),
        "uncertain_receipt_channels": uncertain,
        "explanation": (
            f"Расчёт на {record.cutoff}: "
            + (
                f"остаток может стать отрицательным {gap}. "
                if gap
                else "в этом сценарии отрицательного остатка нет. "
            )
            + "Поступления взяты из прошлой частоты и сумм; это не подтверждённые счета к оплате. "
            + ("Нерегулярные/прочие приходы особенно неопределённы. " if uncertain else "")
            + "Календарные расходы — гипотезы по прошлым операциям, не подтверждённые обязательства."
        ),
    }


def cases(output, records, companies):
    selected = [
        r for r in records if r.company in {"C126", "C032"} and r.cutoff == date(2026, 8, 24)
    ]
    if not selected:
        return
    detail = output / "examples"
    detail.mkdir()
    write_predictions(detail, selected)
    rows = []
    for cid in ("C126", "C032"):
        r = next((r for r in selected if r.company == cid), None)
        if r is None:
            continue
        for d in r.history:
            rows.append(
                {
                    "company_id": cid,
                    "date": d.day.isoformat(),
                    "inflow": money(d.inflow),
                    "outflow": money(d.outflow),
                    "balance": money(d.balance),
                    "observed": d.observed_balance,
                }
            )
    write_csv(detail / "history.csv", rows)
    write_csv(
        detail / "operations.csv",
        [
            {
                "company_id": op.company,
                "operation_id": op.operation_id,
                "date": op.day.isoformat(),
                "amount": money(op.amount),
                "incoming": op.incoming,
                "purpose": op.purpose,
                "counterparty": op.name,
                "balance": money(op.balance),
            }
            for cid in ("C126", "C032")
            if cid in companies
            for op in companies[cid].operations
            if date(2026, 5, 27) <= op.day <= date(2026, 9, 23)
        ],
    )
    # Exportable local research figure. No network or publishing.
    os.environ.setdefault("MPLCONFIGDIR", str(detail / ".matplotlib"))
    try:
        import matplotlib

        matplotlib.use("Agg")
        import matplotlib.pyplot as plt
    except ImportError:
        return
    shown = ("mean_90", "weekday_8w", "channel_expected_both", "channel_events_both")
    for cid in ("C126", "C032"):
        rs = [r for r in selected if r.company == cid]
        if not rs:
            continue
        r = rs[0]
        fig, axes = plt.subplots(3, 1, figsize=(12, 9), sharex=True)
        for ax, stream, title in zip(
            axes, ("inflow", "outflow", "balance"), ("Поступления", "Списания", "Остаток")
        ):
            ax.plot(
                [d.day for d in r.truth],
                [getattr(d, stream) / 100000000 for d in r.truth],
                color="black",
                label="Факт",
                linewidth=2,
            )
            for row in rs:
                if row.method in shown:
                    ax.plot(
                        [p.day for p in row.predictions],
                        [getattr(p, stream) / 100000000 for p in row.predictions],
                        label=row.method,
                        alpha=0.8,
                    )
            ax.set_ylabel(title + ", млн")
            ax.grid(alpha=0.2)
        axes[-1].axhline(0, color="red", linewidth=0.7)
        axes[0].legend(fontsize=8)
        fig.suptitle(cid + ": срез 24.08.2026, локальная диагностика")
        fig.autofmt_xdate()
        fig.tight_layout()
        fig.savefig(detail / f"{cid}.png", dpi=150)
        plt.close(fig)


def report(output, metrics, choice, diagnostic, manifest):
    lines = [
        "# Дневной остаток и события",
        "",
        "Все методы используют одинаковые окна. Поздняя часть уже изучалась; это диагностика, не независимый holdout.",
        "",
        f"Выбор по validation: **{choice['selected']}**. Default сохранён: **mean_90**.",
        "",
        "| Часть | Метод | MAE остатка | Медиана MAE компаний | Дневной WAPE прихода | WAPE суммы | TP/FN/FP |",
        "| --- | --- | ---: | ---: | ---: | ---: | --- |",
    ]
    for row in metrics["by_split"]:
        if row["split"] == "development":
            continue
        gap = row["new_gap"]
        lines.append(
            f"| {row['split']} | {row['method']} | {row['balance_mae']:.2f} | {row['company_median_balance_mae']:.2f} | {row['inflow_wape']:.2%} | {row['horizon_inflow_wape']:.2%} | {gap['tp']}/{gap['fn']}/{gap['fp']} |"
        )
    lines += ["", "## Ворота выбора на validation", ""]
    for trial in choice["trials"]:
        lines.append(
            f"- {trial['method']}: "
            + ("пройдено" if trial["passes"] else ", ".join(trial["failures"]))
            + "; компании "
            + str(trial["company_comparison"])
        )
    lines += [
        "",
        "## Причины ошибок mean_90",
        "",
        "Разложение суммы/времени — арифметическая диагностика, не причинная модель и не будущие признаки. Части могут компенсироваться.",
        "",
        "```json",
        json.dumps(diagnostic, ensure_ascii=False, indent=2),
        "```",
        "",
        "## Ограничения",
        "",
        "Осторожный сценарий — чувствительность к меньшим/поздним поступлениям и верхнему прошлому уровню расходов. Его ложные тревоги считаются отдельно. Это не вероятность и не доверительный интервал.",
        "",
        "Календарь знает обычные выходные, но не праздники/переносы РФ. Расписания кредитов и подтверждённые будущие счета не предоставлены. Остаток относится к одному подключённому счёту. Свои переводы сохранены; другие счета не консолидируются.",
        "",
        "Человеческая независимая разметка ещё не выполнена: blind CSV и инструкция подготовлены, accuracy не измерена. Логистический score не калиброван; p*amount может размазывать редкие приходы, это явно измеряется и сопровождается fallback.",
        "",
        "Все попытки, метрики по компаниям/типам, события, причины fallback и предупреждения сохранены локально. Для продвижения default нужны новые временные данные.",
        "",
    ]
    (output / "comparison.md").write_text("\n".join(lines), encoding="utf-8")
    dump_json(output / "manifest.json", manifest)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data-dir", type=Path, default=ROOT / "data/cashflow/raw")
    parser.add_argument(
        "--reference-dir", type=Path, default=ROOT / "services/ml/artifacts/baseline_43d7a28"
    )
    parser.add_argument(
        "--output-dir", type=Path, default=ROOT / "services/ml/artifacts/balance_v1_verified"
    )
    args = parser.parse_args(argv)
    output, data, reference = (
        args.output_dir.resolve(),
        args.data_dir.resolve(),
        args.reference_dir.resolve(),
    )
    for protected in (data, reference):
        if output == protected or protected in output.parents or output in protected.parents:
            parser.error("Output must not overlap input/frozen reference")
    if output.exists():
        parser.error("Output exists: choose a fresh directory; previous experiments are preserved")
    source_hashes = {name: sha256(data / name) for name in ("all_operations.csv", "companies.csv")}
    implementation_hashes = {p.name: sha256(p) for p in sorted(Path(__file__).parent.glob("*.py"))}
    frozen = json.loads((reference / "manifest.json").read_text(encoding="utf-8"))
    if source_hashes != frozen["source_sha256"]:
        raise DataError("Source differs from frozen baseline")
    config = read_config(Path(__file__).parents[1] / "baseline_config.json")
    if config_dict(config) != frozen["config"]:
        raise DataError("Frozen evaluation protocol changed")
    companies = {c.company_id: c for c in load_companies(data)}
    print("Reproducing original windows and forecasts...", flush=True)
    baseline, skipped, eligibility = evaluate(list(companies.values()), config)
    output.mkdir(parents=True)
    write_predictions(output, baseline)
    if (output / "predictions.csv").read_bytes() != (
        reference / "predictions.csv"
    ).read_bytes() or all_metrics(baseline) != json.loads(
        (reference / "metrics.json").read_text(encoding="utf-8")
    ):
        raise DataError("Original baseline forecasts/metrics changed")
    write_eligibility(output, eligibility)
    refs = [r for r in baseline if r.method == "mean_90"]
    diagnostics = [diagnostic_row(r, companies[r.company]) for r in refs]
    write_csv(output / "error_decomposition.csv", diagnostics)
    summary = {}
    for split in ("development", "validation", "evaluation"):
        ds = [d for d in diagnostics if d["split"] == split]
        summary[split] = {
            "windows": len(ds),
            "companies": len({d["company_id"] for d in ds}),
            "dominant_components": dict(Counter(d["dominant_component"] for d in ds)),
            "receipt_types": dict(Counter(d["receipt_type"] for d in ds)),
            "windows_with_own_transfers": sum(
                d["future_own_transfer_cents_diagnostic_only"] > 0 for d in ds
            ),
            "windows_with_exceptional_flows": sum(
                d["future_exceptional_cents_diagnostic_only"] > 0 for d in ds
            ),
            "mean_component_errors": {
                key: mean(d[key] for d in ds)
                for key in ("inflow_amount", "inflow_timing", "outflow_amount", "outflow_timing")
            },
        }
    summary["excluded_quality_and_other_reasons"] = skipped
    dump_json(output / "diagnostics.json", summary)
    print("Stage 1: full-dataset diagnostics saved.", flush=True)
    records, explanations = list(baseline), []
    # Run early parts and make a frozen validation selection BEFORE late forecasts.
    choice = None
    for late in (False, True):
        for index, r in enumerate([r for r in refs if (r.split == "evaluation") == late]):
            if index % 100 == 0:
                print(
                    f"{'Late diagnostic' if late else 'Past/validation'} window {index}", flush=True
                )
            c = companies[r.company]
            outgoing, patterns, events = expense_calendar(c, r.history, config.horizon)
            default_in = [p.inflow for p in r.predictions]
            default_out = [p.outflow for p in r.predictions]
            expense_record = replace(
                r, method="expense_only", predictions=combine(r.history, default_in, outgoing)
            )
            records.append(expense_record)
            detail = {
                "company_id": r.company,
                "cutoff": r.cutoff.isoformat(),
                "expense_patterns": [
                    {
                        "category": p.category,
                        "counterparty": p.counterparty,
                        "confirmations": p.confirmations,
                        "date_error": p.mean_date_error,
                        "amount_cents": p.amount,
                        "matched_ids": sorted(p.matched_ids),
                        "paid_periods": sorted(p.paid_periods),
                    }
                    for p in patterns
                ],
                "expense_events": [
                    {
                        "date": e.day.isoformat(),
                        "amount_cents": e.amount,
                        "category": patterns[e.pattern_index].category,
                    }
                    for e in events
                ],
            }
            for method in RECEIPT_METHODS:
                incoming, evidence = incoming_forecast(c, r.history, config.horizon, method)
                for suffix, out in (("inflow_only", default_out), ("both", outgoing)):
                    new = replace(
                        r,
                        method=method + "_" + suffix,
                        predictions=combine(r.history, incoming, out),
                    )
                    records.append(new)
                    if method == "channel_events" and suffix == "both":
                        cautious, caution_evidence = cautious_forecast(
                            c, r.history, new.predictions
                        )
                        records.append(replace(new, method="cautious_events", predictions=cautious))
                        detail["caution"] = caution_evidence
                        detail["user_explanation"] = explain(new, evidence)
                detail[method] = evidence
            explanations.append(detail)
        if not late:
            # Cautious is a separate policy; it cannot win expected-path selection.
            choice = selection([r for r in records if r.method != "cautious_events"], "validation")
            dump_json(output / "selection_before_evaluation.json", choice)
            print("Validation choice frozen: " + choice["selected"], flush=True)
    for method, folder in (("seasonal_receipts", "inflow_v2"), ("calendar_hybrid", "recurring_v2")):
        records += saved_control(
            ROOT / "services/ml/artifacts" / folder / "predictions.csv", method, refs
        )
    # Every new/control method must have the identical full set of eligible windows.
    keys = {(r.company, r.cutoff) for r in refs}
    for method in {r.method for r in records}:
        current = [r for r in records if r.method == method]
        if len(current) != len(keys) or {(r.company, r.cutoff) for r in current} != keys:
            raise DataError("Method changed the eligible window set")
    for r in records:
        expected = combine(
            r.history, [p.inflow for p in r.predictions], [p.outflow for p in r.predictions]
        )
        if expected != r.predictions:
            raise DataError("Forecast balance arithmetic is inconsistent")
    metrics = all_metrics(records)
    metrics["by_split"] = [
        {
            "split": split,
            "method": method,
            **product_metrics([r for r in records if r.split == split and r.method == method]),
        }
        for split in ("development", "validation", "evaluation")
        for method in sorted({r.method for r in records})
    ]
    metrics["by_receipt_type"] = grouped_metrics(records)
    metrics["company_comparison"] = [
        {
            "split": split,
            "method": method,
            **compare_companies(
                [r for r in records if r.split == split and r.method == method],
                [r for r in refs if r.split == split],
            ),
        }
        for split in ("validation", "evaluation")
        for method in sorted({r.method for r in records})
    ]
    metrics["late_gates_diagnostic_only"] = selection(
        [r for r in records if r.method != "cautious_events"], "evaluation"
    )
    dump_json(output / "metrics.json", metrics)
    dump_json(output / "explanations.json", explanations)
    write_predictions(output, records)
    # Copy only each original method's exported rows to verify unchanged final export.
    exported = Counter()
    with (output / "predictions.csv").open(encoding="utf-8-sig") as stream:
        for row in csv.DictReader(stream, delimiter=";"):
            exported[row["method"]] += 1
    if any(count != len(keys) * config.horizon for count in exported.values()):
        raise DataError("Final CSV lost daily forecast rows")
    audit = human_sample(
        output,
        list(companies.values()),
        ROOT / "services/ml/artifacts/inflow_v2/manual_review.json",
    )
    cases(output, records, companies)
    final_hashes = {name: sha256(data / name) for name in source_hashes}
    if final_hashes != source_hashes:
        raise DataError("Input changed during experiment")
    if implementation_hashes != {
        p.name: sha256(p) for p in sorted(Path(__file__).parent.glob("*.py"))
    }:
        raise DataError("Implementation changed during experiment; rerun in a fresh directory")
    manifest = {
        "protocol": PROTOCOL_VERSION,
        "source_sha256": source_hashes,
        "implementation_sha256": implementation_hashes,
        "config": config_dict(config),
        "paired_windows": len(keys),
        "baseline_identical": True,
        "validation_selected": choice["selected"],
        "default": "mean_90",
        "independent_holdout": False,
        "human_audit": audit,
        "exported_rows": dict(exported),
        "probability_calibrated": False,
    }
    report(output, metrics, choice, summary, manifest)
    print(
        json.dumps(
            {
                "output": str(output),
                "selected": choice["selected"],
                "default": "mean_90",
                "windows": len(keys),
                "human_sample": audit,
            },
            ensure_ascii=True,
        ),
        flush=True,
    )
    return 0
