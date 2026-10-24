"""Local CSV/JSON results, readable reports and diagnostic matplotlib plots."""

import base64
import csv
import html
import json
import os
from dataclasses import asdict
from pathlib import Path

from .data import money
from .evaluation import first_gap

LABELS = {"mean_90": "Среднее за 90 дней", "weekday_8w": "Дни недели за 8 недель"}


def dump_json(path, value):
    path.write_text(
        json.dumps(value, ensure_ascii=False, indent=2, allow_nan=False), encoding="utf-8"
    )


def percent(value):
    return "н/д" if value is None else f"{value * 100:.1f}%"


def number(value):
    return "н/д" if value is None else f"{value:,.2f}".replace(",", " ")


def write_predictions(output: Path, records):
    fields = [
        "company_id",
        "cutoff_date",
        "split",
        "method",
        "date",
        "initial_balance",
        "actual_inflow",
        "predicted_inflow",
        "actual_outflow",
        "predicted_outflow",
        "actual_balance",
        "predicted_balance",
        "actual_balance_source",
    ]
    with (output / "predictions.csv").open("w", encoding="utf-8-sig", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=fields, delimiter=";")
        writer.writeheader()
        for record in records:
            for actual, predicted in zip(record.truth, record.predictions):
                writer.writerow(
                    {
                        "company_id": record.company,
                        "cutoff_date": record.cutoff.isoformat(),
                        "split": record.split,
                        "method": record.method,
                        "date": actual.day.isoformat(),
                        "initial_balance": money(record.initial_balance),
                        "actual_inflow": money(actual.inflow),
                        "predicted_inflow": money(predicted.inflow),
                        "actual_outflow": money(actual.outflow),
                        "predicted_outflow": money(predicted.outflow),
                        "actual_balance": money(actual.balance),
                        "predicted_balance": money(predicted.balance),
                        "actual_balance_source": "observed"
                        if actual.observed_balance
                        else "assumed_no_operations",
                    }
                )


def write_eligibility(output, eligibility):
    with (output / "eligibility.csv").open("w", encoding="utf-8-sig", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=list(eligibility[0]), delimiter=";")
        writer.writeheader()
        writer.writerows(eligibility)


def example_pairs(records):
    grouped = {}
    for record in records:
        if record.split == "evaluation":
            grouped.setdefault((record.company, record.cutoff), []).append(record)
    pairs = list(grouped.values())
    if not pairs:
        return []
    # Examples are selected by actual outcomes, never the quality of either method.
    positive = [pair for pair in pairs if pair[0].initial_balance >= 0 and first_gap(pair[0].truth)]
    negative = [pair for pair in pairs if pair[0].initial_balance < 0]
    stable = [
        pair for pair in pairs if pair[0].initial_balance >= 0 and not first_gap(pair[0].truth)
    ]
    selected = []
    if positive:
        selected.append(min(positive, key=lambda pair: min(day.balance for day in pair[0].truth)))
    if stable:
        stable.sort(key=lambda pair: sum(day.inflow for day in pair[0].truth))
        selected.append(stable[len(stable) // 2])
    if negative:
        selected.append(sorted(negative, key=lambda pair: (pair[0].company, pair[0].cutoff))[0])
    return selected


def render_examples(output, records):
    # Keep the plotting cache in the ignored artifact directory, not the user's home.
    os.environ.setdefault("MPLCONFIGDIR", str((output / ".matplotlib").resolve()))
    import matplotlib

    matplotlib.use("Agg")
    import matplotlib.dates as mdates
    import matplotlib.pyplot as plt
    from matplotlib.ticker import FuncFormatter

    images = []
    for index, pair in enumerate(example_pairs(records), 1):
        record = pair[0]
        fig, (balance_ax, flow_ax) = plt.subplots(
            2,
            1,
            figsize=(12, 7),
            sharex=True,
            gridspec_kw={"height_ratios": [2, 1]},
            layout="constrained",
        )
        past = record.history[-30:]
        balance_ax.plot(
            [d.day for d in past], [d.balance / 100 for d in past], color="#526075", label="История"
        )
        dates = [record.cutoff] + [d.day for d in record.truth]
        balance_ax.plot(
            dates,
            [record.initial_balance / 100] + [d.balance / 100 for d in record.truth],
            color="#111827",
            linewidth=2.4,
            label="Факт / перенос при отсутствии строк",
        )
        for forecast, color in zip(pair, ["#2563eb", "#d97706"]):
            balance_ax.plot(
                dates,
                [record.initial_balance / 100] + [d.balance / 100 for d in forecast.predictions],
                color=color,
                linestyle="--",
                label=LABELS[forecast.method],
            )
            flow_ax.plot(
                [d.day for d in forecast.predictions],
                [(d.inflow - d.outflow) / 100 for d in forecast.predictions],
                color=color,
                linewidth=1.6,
                label=LABELS[forecast.method],
            )
        balance_ax.axhline(0, color="#dc2626", linewidth=1)
        balance_ax.axvline(record.cutoff, color="#64748b", linestyle=":")
        balance_ax.set_title(
            f"{record.company} · {record.industry}\nСрез {record.cutoff.isoformat()}, следующие {len(record.truth)} дней",
            loc="left",
            fontsize=13,
        )
        balance_ax.set_ylabel("Остаток, ден. ед.")
        balance_ax.legend(loc="best", fontsize=8)
        flow_ax.bar(
            [d.day for d in record.truth],
            [(d.inflow - d.outflow) / 100 for d in record.truth],
            color="#9ca3af",
            alpha=0.6,
            label="Фактическое чистое движение",
        )
        flow_ax.axhline(0, color="#64748b", linewidth=0.8)
        flow_ax.set_ylabel("Поток за день, ден. ед.")
        flow_ax.xaxis.set_major_locator(mdates.WeekdayLocator(interval=1))
        flow_ax.xaxis.set_major_formatter(mdates.DateFormatter("%d.%m"))
        for axis in (balance_ax, flow_ax):
            axis.grid(axis="y", alpha=0.15)
            axis.yaxis.set_major_formatter(
                FuncFormatter(lambda x, _: f"{x:,.0f}".replace(",", " "))
            )
            axis.spines[["top", "right"]].set_visible(False)
        name = f"example_{index}.png"
        fig.savefig(output / name, dpi=150)
        plt.close(fig)
        images.append(
            {"file": name, "company": record.company, "cutoff": record.cutoff.isoformat()}
        )
    return images


def write_reports(output, metrics, skipped, manifest, images):
    headers = [
        "Срез",
        "Метод",
        "Окон",
        "Компаний",
        "WAPE вход",
        "WAPE выход",
        "MAE остатка",
        "Медиана MAE по компаниям",
    ]
    rows = [
        [
            m["split"],
            LABELS[m["method"]],
            str(m["windows"]),
            str(m["companies"]),
            percent(m["inflow_wape"]),
            percent(m["outflow_wape"]),
            number(m["balance_mae"]),
            number(m["company_median_balance_mae"]),
        ]
        for m in metrics["by_split"]
    ]
    horizon_headers = ["Срез", "Метод", "WAPE суммы поступлений", "WAPE суммы расходов"]
    horizon_rows = [
        [
            m["split"],
            LABELS[m["method"]],
            percent(m["horizon_inflow_wape"]),
            percent(m["horizon_outflow_wape"]),
        ]
        for m in metrics["by_split"]
    ]
    risk_headers = [
        "Срез",
        "Метод",
        "Окон с новым минусом",
        "TP",
        "FP",
        "FN",
        "Precision",
        "Recall",
    ]
    risk_rows = []
    for item in metrics["by_split"]:
        risk = item["new_gap"]
        risk_rows.append(
            [
                item["split"],
                LABELS[item["method"]],
                str(risk["actual_positive_windows"]),
                str(risk["tp"]),
                str(risk["fp"]),
                str(risk["fn"]),
                percent(risk["precision"]),
                percent(risk["recall"]),
            ]
        )
    notes = [
        "Два фиксированных baseline без обучения и подбора параметров: средний дневной поток за 90 дней и средний поток соответствующего дня недели за 8 недель.",
        "Оба метода получают одинаковые срезы и наблюдаемый EOD на дату расчёта. Прогноз начинается на следующий календарный день. День без строк считается нулевым движением как явное допущение, не доказанный факт.",
        "История восстанавливается только по операциям, доступным на cutoff. Диагностика полного набора применяется отдельно для отбора достоверных будущих целей, не как вход модели.",
        "Пропуски с расхождением более 1 копейки и кандидаты в дубли исключают затронутые окна. Дубли не удаляются. Копеечные расхождения сохраняются как предупреждения.",
        "WAPE = сумма абсолютных ошибок / сумма фактических потоков. Она сильнее взвешивает крупные компании. MAE остатка — средняя абсолютная дневная ошибка в денежных единицах набора; валюта в CSV не задана.",
        "Дневная WAPE сравнивает каждый отдельный день; WAPE суммы за горизонт сначала складывает потоки за все 30 дней. Ошибка времени платежа может быть велика по дням и мала по месячной сумме. Эти метрики нельзя подменять друг другом.",
        "Риск означает отрицательный EOD одного счёта. Новые случаи при неотрицательном начальном остатке и продолжение уже отрицательного состояния считаются отдельно. Вероятность этими методами не оценивается: gap_probability=null.",
        "Шаг срезов не меньше горизонта: будущие окна одной компании не перекрываются. Горизонты, пересекающие границу временных частей, исключены. Истории разных прогнозов могут перекрываться; компании не являются независимыми строками.",
        "Поздняя часть датасета уже изучалась при EDA. Evaluation — диагностическая временная проверка, не нетронутый финальный holdout. Достоверность результата на новых компаниях пока не проверена.",
        "Нулевая вероятность, уверенные интервалы, календарь будущих обязательств, категоризация, налоговый расчёт и API этим экспериментом не реализованы.",
        "Графики выбраны по фактическим исходам: самый глубокий новый минус, обычное окно со средним объёмом поступлений, первый продолжающийся минус. Это иллюстрации, не доказательство качества вместо общих метрик.",
    ]

    def md_table(header, body):
        return "\n".join(
            ["| " + " | ".join(header) + " |", "| " + " | ".join(["---"] * len(header)) + " |"]
            + ["| " + " | ".join(row) + " |" for row in body]
        )

    markdown = "# Локальное сравнение cash-flow baseline\n\n"
    markdown += f"Исходные операции: {manifest['operations']}; компаний: {manifest['companies_loaded']}. Горизонт: {manifest['config']['horizon']} дней.\n\n"
    markdown += "## Ошибки прогноза\n\n" + md_table(headers, rows)
    markdown += "\n\n## Суммы потоков за горизонт\n\n" + md_table(horizon_headers, horizon_rows)
    markdown += "\n\n## Новые отрицательные остатки\n\n" + md_table(risk_headers, risk_rows)
    markdown += "\n\n## Ограничения и методика\n\n" + "\n\n".join(notes)
    markdown += "\n\n## Исключённые срезы\n\n" + "\n".join(
        f"- {name}: {count}" for name, count in sorted(skipped.items())
    )
    markdown += "\n\nПодробности: metrics.json, predictions.csv, eligibility.csv, manifest.json.\n"
    for image in images:
        markdown += f"\n![{image['company']}, срез {image['cutoff']}]({image['file']})\n"
    (output / "report.md").write_text(markdown, encoding="utf-8")

    def table(header, body):
        return (
            "<table><thead><tr>"
            + "".join(f"<th>{html.escape(cell)}</th>" for cell in header)
            + "</tr></thead><tbody>"
            + "".join(
                "<tr>" + "".join(f"<td>{html.escape(cell)}</td>" for cell in row) + "</tr>"
                for row in body
            )
            + "</tbody></table>"
        )

    document = "<!doctype html><html lang='ru'><meta charset='utf-8'><title>Cashflow baseline</title><style>body{font:16px/1.55 system-ui;margin:40px auto;max-width:1250px;padding:0 24px;color:#172033;background:#fafbfc}table{border-collapse:collapse;width:100%;font-size:14px;background:white}th,td{border:1px solid #d9dfe8;padding:9px;text-align:left}th{background:#eaf0f8}img{max-width:100%;height:auto}p{max-width:1000px}h2{margin-top:32px}</style><h1>Локальное сравнение baseline</h1>"
    document += (
        "<p>Проверка на исторических срезах. Вероятность риска не оценивается.</p><h2>Ошибки прогноза</h2>"
        + table(headers, rows)
    )
    document += "<h2>Новые отрицательные остатки</h2>" + table(risk_headers, risk_rows)
    document += "<h2>Суммы потоков за горизонт</h2>" + table(horizon_headers, horizon_rows)
    document += "<h2>Методика и ограничения</h2>" + "".join(
        f"<p>{html.escape(note)}</p>" for note in notes
    )
    document += (
        "<h2>Исключённые срезы</h2><ul>"
        + "".join(
            f"<li>{html.escape(name)}: {count}</li>" for name, count in sorted(skipped.items())
        )
        + "</ul>"
    )
    for image in images:
        encoded = base64.b64encode((output / image["file"]).read_bytes()).decode("ascii")
        document += f"<h2>{image['company']} · {image['cutoff']}</h2><img alt='Факт и прогноз' src='data:image/png;base64,{encoded}'>"
    (output / "report.html").write_text(document + "</html>", encoding="utf-8")


def config_dict(config):
    return {
        name: value.isoformat() if hasattr(value, "isoformat") else value
        for name, value in asdict(config).items()
    }
