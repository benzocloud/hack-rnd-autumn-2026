"""Explain negative-balance outcomes using facts, keeping them out of prediction."""

import csv
import json
from collections import defaultdict
from datetime import date, timedelta
from pathlib import Path

from .categories import categorize
from .data import Company, money, parse_money


def gap_cases(baseline_dir: Path, companies):
    windows = defaultdict(list)
    with (baseline_dir / "predictions.csv").open(encoding="utf-8-sig", newline="") as source:
        for row in csv.DictReader(source, delimiter=";"):
            if row["split"] == "evaluation":
                windows[(row["company_id"], row["cutoff_date"], row["method"])].append(row)
    by_id = {company.company_id: company for company in companies}
    cases = []
    for (cid, cutoff_text, method), rows in sorted(windows.items()):
        if method != "mean_90" or parse_money(rows[0]["initial_balance"]) < 0:
            continue
        negative = [row for row in rows if parse_money(row["actual_balance"]) < 0]
        if not negative:
            continue
        cutoff = date.fromisoformat(cutoff_text)
        first = date.fromisoformat(negative[0]["date"])
        company: Company = by_id[cid]
        past = [op for op in company.operations if cutoff - timedelta(days=179) <= op.day <= cutoff]
        critical = [op for op in company.operations if cutoff < op.day <= first and not op.incoming]
        critical.sort(key=lambda op: op.amount, reverse=True)
        events = []
        for operation in critical[:5]:
            category = categorize(operation).label
            history = [
                op
                for op in past
                if not op.incoming
                and op.name == operation.name
                and categorize(op).label == category
            ]
            events.append(
                {
                    "date": operation.day.isoformat(),
                    "amount": money(operation.amount),
                    "category": category,
                    "counterparty": operation.name,
                    "purpose": operation.purpose,
                    "similar_past": [
                        {"date": op.day.isoformat(), "amount": money(op.amount)}
                        for op in history[-6:]
                    ],
                    "past_count": len(history),
                }
            )
        predicted = {}
        for name in ("mean_90", "weekday_8w"):
            forecast = windows[(cid, cutoff_text, name)]
            predicted[name] = {
                "first_gap": next(
                    (row["date"] for row in forecast if parse_money(row["predicted_balance"]) < 0),
                    None,
                ),
                "minimum_balance": money(
                    min(parse_money(row["predicted_balance"]) for row in forecast)
                ),
            }
        cases.append(
            {
                "company_id": cid,
                "industry": company.industry,
                "cutoff": cutoff_text,
                "initial_balance": rows[0]["initial_balance"],
                "first_gap": first.isoformat(),
                "minimum_balance": money(min(parse_money(row["actual_balance"]) for row in rows)),
                "predicted": predicted,
                "critical_expenses": events,
                "interpretation": "История содержит похожие регулярные списания; усреднение теряет дату."
                if events and events[0]["past_count"] >= 3
                else "Крупный платёж не подтверждён повторениями в доступной истории; нужна информация об обязательстве.",
            }
        )
    return cases


def write_diagnostics(output: Path, cases):
    output.mkdir(parents=True, exist_ok=True)
    (output / "gap_cases.json").write_text(
        json.dumps(cases, ensure_ascii=False, indent=2), encoding="utf-8"
    )
    text = "# Пять новых уходов в минус: разбор baseline\n\n"
    text += "Фактические будущие операции используются только для объяснения ошибок, не как вход прогноза. Два окна C004 относятся к одному бизнесу.\n\n"
    for case in cases:
        text += f"## {case['company_id']}, срез {case['cutoff']}\n\n"
        text += f"{case['industry']}. Начальный остаток {case['initial_balance']}; первый минус {case['first_gap']}; минимум {case['minimum_balance']} денежных единиц.\n\n"
        text += case["interpretation"] + "\n\n"
        for name, prediction in case["predicted"].items():
            text += f"- {name}: прогноз первого минуса {prediction['first_gap'] or 'нет'}, минимум {prediction['minimum_balance']}.\n"
        text += "\nКрупные списания до первого минуса:\n\n"
        for event in case["critical_expenses"]:
            text += f"- {event['date']}: {event['amount']}, {event['category']}, {event['counterparty']}. {event['purpose']} Подобных операций за прошлые 180 дней: {event['past_count']}.\n"
        if "hybrid_first_gap" in case:
            text += f"\nГибрид: первый минус {case['hybrid_first_gap'] or 'не выявлен'}; минимум {case['hybrid_minimum_balance']}.\n"
            totals = case["flows_until_actual_gap"]
            text += f"\nДо фактического первого минуса поступления: факт {totals['actual_inflow']}, прогноз {totals['predicted_inflow']}; расходы: факт {totals['actual_outflow']}, прогноз {totals['predicted_outflow']}.\n"
            text += f"Разница прогноз минус факт: поступления {totals['predicted_minus_actual_inflow']}, расходы {totals['predicted_minus_actual_outflow']}. Положительная разница поступлений означает переоценку доступных денег; отрицательная разница расходов — недооценку списаний.\n"
        text += "\n"
    text += "Сезонное погашение кредита без расписания не выдаётся за известный будущий платёж. Ни одно улучшение нельзя обосновывать только этими пятью уже изученными окнами.\n"
    (output / "gap_cases.md").write_text(text, encoding="utf-8")
