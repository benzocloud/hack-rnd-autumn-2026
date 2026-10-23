"""Compare one calendar hybrid with an immutable baseline reference."""

import argparse
import base64
import csv
import html
import json
import os
from collections import Counter
from dataclasses import replace
from pathlib import Path

from .categories import categorize
from .cli import ROOT, read_config
from .data import DataError, load_companies, money, sha256
from .diagnostics import gap_cases, write_diagnostics
from .evaluation import all_metrics, evaluate, first_gap
from .recurring import hybrid_predict
from .report import config_dict, dump_json, number, percent, write_eligibility, write_predictions

LABELS = {
    "mean_90": "Среднее 90 дней",
    "weekday_8w": "Недельный профиль",
    "calendar_hybrid": "Категории + регулярность",
}


def csv_rows(path, rows, fields):
    with path.open("w", encoding="utf-8-sig", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=fields, delimiter=";")
        writer.writeheader()
        writer.writerows(rows)


def export_categories(output, companies):
    counts, amounts = Counter(), Counter()
    fields = ["company_id", "operation_id", "date", "category", "rule", "rule_confidence"]
    with (output / "categorized_operations.csv").open(
        "w", encoding="utf-8-sig", newline=""
    ) as stream:
        writer = csv.DictWriter(stream, fieldnames=fields, delimiter=";")
        writer.writeheader()
        for company in companies:
            for operation in company.operations:
                category = categorize(operation)
                counts[category.label] += 1
                amounts[category.label] += operation.amount
                writer.writerow(
                    {
                        "company_id": company.company_id,
                        "operation_id": operation.operation_id,
                        "date": operation.day.isoformat(),
                        "category": category.label,
                        "rule": category.rule,
                        "rule_confidence": category.confidence,
                    }
                )
    summary = {
        "operations": sum(counts.values()),
        "unknown": counts["unknown"],
        "accuracy_measured": False,
        "confidence_is_calibrated": False,
        "categories": [
            {"label": key, "count": count, "amount": money(amounts[key])}
            for key, count in counts.most_common()
        ],
    }
    dump_json(output / "category_summary.json", summary)
    return summary


def compare_and_extend(companies, records):
    by_id = {company.company_id: company for company in companies}
    result, patterns_rows, event_rows = [], [], []
    references = [record for record in records if record.method == "weekday_8w"]
    for index, record in enumerate(references, 1):
        predictions, patterns, events = hybrid_predict(
            by_id[record.company], record.history, len(record.truth)
        )
        result.append(replace(record, method="calendar_hybrid", predictions=predictions))
        for pattern_index, pattern in enumerate(patterns):
            patterns_rows.append(
                {
                    "company_id": record.company,
                    "cutoff_date": record.cutoff.isoformat(),
                    "pattern_index": pattern_index,
                    "category": pattern.category,
                    "counterparty": pattern.counterparty,
                    "direction": "in" if pattern.incoming else "out",
                    "anchor": "month_end" if pattern.anchor == 0 else pattern.anchor,
                    "weekend_shift": pattern.weekend_shift,
                    "amount": money(pattern.amount),
                    "confirmations": pattern.confirmations,
                    "expected_periods": pattern.expected_periods,
                    "mean_date_error": pattern.mean_date_error,
                    "relative_amount_mad": pattern.relative_amount_mad,
                    "first_evidence": pattern.first_evidence.isoformat(),
                    "last_evidence": pattern.last_evidence.isoformat(),
                }
            )
        for event in events:
            event_rows.append(
                {
                    "company_id": record.company,
                    "cutoff_date": record.cutoff.isoformat(),
                    "pattern_index": event.pattern_index,
                    "date": event.day.isoformat(),
                    "amount": money(event.amount),
                    "direction": "in" if event.incoming else "out",
                    "category": patterns[event.pattern_index].category,
                }
            )
        if index % 100 == 0 or index == len(references):
            print(f"Calendar hybrid: {index}/{len(references)} windows", flush=True)
    return result, patterns_rows, event_rows


def plots(output, records, cases):
    os.environ.setdefault("MPLCONFIGDIR", str((output / ".matplotlib").resolve()))
    import matplotlib

    matplotlib.use("Agg")
    import matplotlib.dates as mdates
    import matplotlib.pyplot as plt
    from matplotlib.ticker import FuncFormatter

    images = []
    for index, case in enumerate(cases, 1):
        chosen = [
            r
            for r in records
            if r.company == case["company_id"] and r.cutoff.isoformat() == case["cutoff"]
        ]
        reference = chosen[0]
        dates = [reference.cutoff] + [day.day for day in reference.truth]
        fig, axis = plt.subplots(figsize=(11, 4.8), layout="constrained")
        axis.plot(
            dates,
            [reference.initial_balance / 100] + [d.balance / 100 for d in reference.truth],
            color="#111827",
            linewidth=2.5,
            label="Факт / перенос без операций",
        )
        for record, color in zip(chosen, ["#2563eb", "#d97706", "#059669"]):
            axis.plot(
                dates,
                [record.initial_balance / 100] + [d.balance / 100 for d in record.predictions],
                color=color,
                linestyle="--",
                label=LABELS[record.method],
            )
        axis.axhline(0, color="#dc2626", linewidth=1)
        axis.set_title(
            f"{reference.company} · срез {reference.cutoff.isoformat()}\n{reference.industry}",
            loc="left",
        )
        axis.set_ylabel("Остаток, денежные единицы")
        axis.xaxis.set_major_locator(mdates.WeekdayLocator(interval=1))
        axis.xaxis.set_major_formatter(mdates.DateFormatter("%d.%m"))
        axis.yaxis.set_major_formatter(FuncFormatter(lambda x, _: f"{x:,.0f}".replace(",", " ")))
        axis.legend(fontsize=8, loc="best")
        axis.grid(axis="y", alpha=0.15)
        axis.spines[["top", "right"]].set_visible(False)
        filename = f"gap_case_{index}.png"
        fig.savefig(output / filename, dpi=140)
        plt.close(fig)
        images.append(filename)
    return images


def report(output, metrics, cases, categories, manifest, images):
    header = [
        "Метод",
        "WAPE суммы входа",
        "WAPE суммы выхода",
        "MAE остатка",
        "Новый минус TP/FN/FP",
    ]
    rows = []
    for item in metrics["by_split"]:
        if item["split"] != "evaluation":
            continue
        risk = item["new_gap"]
        rows.append(
            [
                LABELS[item["method"]],
                percent(item["horizon_inflow_wape"]),
                percent(item["horizon_outflow_wape"]),
                number(item["balance_mae"]),
                f"{risk['tp']} / {risk['fn']} / {risk['fp']}",
            ]
        )
    notes = [
        f"Старые прогнозы воспроизведены в точности; конфигурация, источники и все {manifest['paired_windows']} среза совпадают с исходным baseline. Полная поздняя часть: 316 окон у 129 компаний; пять новых отрицательных окон у четырёх компаний.",
        "Текстовые категории — прозрачные правила, без размеченного ground truth. Coverage не является accuracy; rule_confidence — эвристика, не калиброванная вероятность.",
        "Регулярность ищется только по прошлым 180 дням. Кандидат требует минимум 3/4 подтверждения и покрытия 75%, допускается сдвиг 4 дня. Обычные выходные учитываются; производственный календарь РФ и праздники не подключены.",
        "Гибрид вычитает только распознанные исторические операции из потоков, применяет к остатку старый weekday_8w и добавляет ожидаемые события один раз. Регулярность — гипотеза, не подтверждённое обязательство.",
        "Пять случаев уже использованы для анализа ошибок; сравнение исследовательское, не независимый финальный тест. Параметры нового метода зафиксированы до полного сравнения, перебор под эти пять целей не выполнялся.",
        "Крупное погашение сезонного кредита без графика не восстанавливается из будущей операции. Отрицательный EOD одного счёта не доказывает неплатёжеспособность; вероятность риска не оценивается.",
    ]

    def table(headers, body):
        return "\n".join(
            ["| " + " | ".join(headers) + " |", "| " + " | ".join(["---"] * len(headers)) + " |"]
            + ["| " + " | ".join(row) + " |" for row in body]
        )

    markdown = "# Категоризация и регулярность: сравнение с baseline\n\n" + table(header, rows)
    markdown += "\n\n" + "\n\n".join(notes)
    markdown += f"\n\nКатегоризировано операций: {categories['operations']}; unknown: {categories['unknown']}. Измеренной точности классификации нет.\n"
    markdown += "\n## Пять случаев\n\n" + table(
        [
            "Компания / срез",
            "Факт: первый минус",
            "Гибрид: первый минус",
            "Минимум факта",
            "Минимум гибрида",
        ],
        [
            [
                c["company_id"] + " / " + c["cutoff"],
                c["first_gap"],
                c["hybrid_first_gap"] or "нет",
                c["minimum_balance"],
                c["hybrid_minimum_balance"],
            ]
            for c in cases
        ],
    )
    markdown += "\n\nПодробный причинный разбор: gap_cases.md. Это объяснение последовательности денежных движений, не причинная оценка модели.\n"
    for filename in images:
        markdown += f"\n![Сравнение прогнозов]({filename})\n"
    (output / "comparison.md").write_text(markdown, encoding="utf-8")

    def html_table(headers, body):
        return (
            "<table><tr>"
            + "".join(f"<th>{html.escape(x)}</th>" for x in headers)
            + "</tr>"
            + "".join(
                "<tr>" + "".join(f"<td>{html.escape(x)}</td>" for x in row) + "</tr>"
                for row in body
            )
            + "</table>"
        )

    document = "<!doctype html><html lang='ru'><meta charset='utf-8'><title>Регулярные платежи</title><style>body{font:16px/1.6 system-ui;max-width:1200px;margin:40px auto;padding:0 20px;color:#172033}table{border-collapse:collapse;width:100%}td,th{border:1px solid #d9dfe8;padding:9px;text-align:left}th{background:#edf2fa}img{max-width:100%}</style><h1>Категоризация и регулярные платежи</h1>"
    document += html_table(header, rows) + "".join(f"<p>{html.escape(note)}</p>" for note in notes)
    for case in cases:
        document += f"<h2>{case['company_id']} · {case['cutoff']}</h2><p>{html.escape(case['interpretation'])}</p>"
        document += f"<p>Первый минус: факт {case['first_gap']}; гибрид {case['hybrid_first_gap'] or 'не выявлен'}. Минимумы: {case['minimum_balance']} / {case['hybrid_minimum_balance']}.</p><ul>"
        for event in case["critical_expenses"]:
            document += f"<li>{event['date']}: {event['amount']} — {html.escape(event['category'])}; {html.escape(event['purpose'])}. Похожих операций в прошлом: {event['past_count']}.</li>"
        document += "</ul>"
    for filename in images:
        encoded = base64.b64encode((output / filename).read_bytes()).decode("ascii")
        document += f"<img alt='Сравнение прогнозов' src='data:image/png;base64,{encoded}'>"
    (output / "comparison.html").write_text(document + "</html>", encoding="utf-8")


def main(argv=None):
    parser = argparse.ArgumentParser(description="Categorization and recurring-payment experiment")
    parser.add_argument("--data-dir", type=Path, default=ROOT / "data/cashflow/raw")
    parser.add_argument(
        "--reference-dir", type=Path, default=ROOT / "services/ml/artifacts/baseline_43d7a28"
    )
    parser.add_argument(
        "--output-dir", type=Path, default=ROOT / "services/ml/artifacts/recurring_v2"
    )
    parser.add_argument("--no-plots", action="store_true")
    args = parser.parse_args(argv)
    output, reference = args.output_dir.resolve(), args.reference_dir.resolve()
    data_dir = args.data_dir.resolve()
    for protected in (reference, data_dir):
        if output == protected or protected in output.parents or output in protected.parents:
            parser.error("Output must not overlap reference or source data")
    baseline_manifest = json.loads((reference / "manifest.json").read_text(encoding="utf-8"))
    for filename, expected in baseline_manifest["source_sha256"].items():
        if sha256(data_dir / filename) != expected:
            raise DataError("Source differs from frozen baseline")
    config = read_config(Path(__file__).parents[1] / "baseline_config.json")
    if config_dict(config) != baseline_manifest["config"]:
        raise DataError("Config differs from frozen baseline")
    companies = load_companies(data_dir)
    output.mkdir(parents=True, exist_ok=True)
    print("Reproducing fixed baseline windows...", flush=True)
    old_records, skipped, eligibility = evaluate(companies, config)
    write_predictions(output, old_records)
    if (output / "predictions.csv").read_bytes() != (reference / "predictions.csv").read_bytes():
        raise DataError("Original baseline predictions changed")
    if all_metrics(old_records) != json.loads(
        (reference / "metrics.json").read_text(encoding="utf-8")
    ):
        raise DataError("Original baseline metrics changed")
    categories = export_categories(output, companies)
    cases = gap_cases(reference, companies)
    new_records, pattern_rows, event_rows = compare_and_extend(companies, old_records)
    records = old_records + new_records
    # Three methods have the same keys; the new model cannot improve by dropping cases.
    assert {(r.company, r.cutoff) for r in new_records} == {
        (r.company, r.cutoff) for r in old_records
    }
    metrics = all_metrics(records)
    write_predictions(output, records)
    write_eligibility(output, eligibility)
    dump_json(output / "metrics.json", metrics)
    csv_rows(
        output / "recurring_patterns.csv",
        pattern_rows,
        [
            "company_id",
            "cutoff_date",
            "pattern_index",
            "category",
            "counterparty",
            "direction",
            "anchor",
            "weekend_shift",
            "amount",
            "confirmations",
            "expected_periods",
            "mean_date_error",
            "relative_amount_mad",
            "first_evidence",
            "last_evidence",
        ],
    )
    csv_rows(
        output / "expected_events.csv",
        event_rows,
        ["company_id", "cutoff_date", "pattern_index", "date", "amount", "direction", "category"],
    )
    lookup = {(r.company, r.cutoff.isoformat()): r for r in new_records}
    for case in cases:
        record = lookup[(case["company_id"], case["cutoff"])]
        day = first_gap(record.predictions)
        case["hybrid_first_gap"] = day.isoformat() if day else None
        case["hybrid_minimum_balance"] = money(min(d.balance for d in record.predictions))
        before_gap = [
            (actual, predicted)
            for actual, predicted in zip(record.truth, record.predictions)
            if actual.day.isoformat() <= case["first_gap"]
        ]
        case["flows_until_actual_gap"] = {
            "actual_inflow": money(sum(actual.inflow for actual, _ in before_gap)),
            "predicted_inflow": money(sum(predicted.inflow for _, predicted in before_gap)),
            "actual_outflow": money(sum(actual.outflow for actual, _ in before_gap)),
            "predicted_outflow": money(sum(predicted.outflow for _, predicted in before_gap)),
            "predicted_minus_actual_inflow": money(
                sum(predicted.inflow - actual.inflow for actual, predicted in before_gap)
            ),
            "predicted_minus_actual_outflow": money(
                sum(predicted.outflow - actual.outflow for actual, predicted in before_gap)
            ),
        }
    write_diagnostics(output, cases)
    manifest = {
        "reference_head": "43d7a285da809f9704b40e5773fd727557caeae6",
        "source_sha256": baseline_manifest["source_sha256"],
        "config": config_dict(config),
        "paired_windows": len(new_records),
        "skipped": skipped,
        "baseline_predictions_identical": True,
        "baseline_metrics_identical": True,
        "probability_estimated": False,
        "category_accuracy_measured": False,
        "implementation_sha256": {p.name: sha256(p) for p in Path(__file__).parent.glob("*.py")},
        "pattern_count": len(pattern_rows),
        "expected_event_count": len(event_rows),
        "recurring_parameters": {
            "history_days": 180,
            "min_regular_confirmations": 3,
            "min_other_confirmations": 4,
            "min_coverage": 0.75,
            "date_tolerance_days": 4,
            "residual_method": "weekday_8w",
        },
    }
    images = [] if args.no_plots else plots(output, records, cases)
    dump_json(output / "manifest.json", manifest)
    report(output, metrics, cases, categories, manifest, images)
    print(
        json.dumps(
            {
                "output": str(output),
                "evaluation": [m for m in metrics["by_split"] if m["split"] == "evaluation"],
                "cases": [
                    {k: c[k] for k in ["company_id", "cutoff", "first_gap", "hybrid_first_gap"]}
                    for c in cases
                ],
            },
            ensure_ascii=True,
            indent=2,
        )
    )
    return 0
