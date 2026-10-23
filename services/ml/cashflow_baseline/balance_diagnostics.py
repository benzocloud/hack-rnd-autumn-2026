"""Product diagnostics, exact error decomposition and blind human audit export."""

import csv
import json
import random
from collections import Counter, defaultdict
from fractions import Fraction
from statistics import mean, median

from .balance_model import receipt_channels, receipt_type
from .categories import categorize
from .data import DataError, money
from .evaluation import aggregate, first_gap
from .report import dump_json


def product_metrics(records):
    result = aggregate(records)
    if not records:
        return result
    by_company = defaultdict(list)
    leads, offsets, depths, details = [], [], [], []
    for r in records:
        errors = [abs(a.balance - p.balance) for a, p in zip(r.truth, r.predictions)]
        scale = max(
            1, Fraction(sum(d.inflow + d.outflow for d in r.history), len(r.history)) * len(r.truth)
        )
        by_company[r.company].append(float(Fraction(sum(errors), len(errors)) / scale))
        actual, predicted = first_gap(r.truth), first_gap(r.predictions)
        actual_depth = max(0, -min(d.balance for d in r.truth))
        predicted_depth = max(0, -min(d.balance for d in r.predictions))
        if r.initial_balance >= 0:
            if actual:
                depths.append(abs(actual_depth - predicted_depth) / 100)
            if actual and predicted:
                leads.append((actual - r.cutoff).days)
                offsets.append((predicted - actual).days)
            if actual or predicted:
                details.append(
                    {
                        "company_id": r.company,
                        "cutoff": r.cutoff.isoformat(),
                        "actual_date": actual.isoformat() if actual else None,
                        "predicted_date": predicted.isoformat() if predicted else None,
                        "available_warning_days": (actual - r.cutoff).days
                        if actual and predicted
                        else None,
                        "predicted_horizon_days": (predicted - r.cutoff).days
                        if predicted
                        else None,
                        "actual_depth_cents": actual_depth,
                        "predicted_depth_cents": predicted_depth,
                    }
                )
    result.update(
        macro_normalized_balance_mae=mean(mean(v) for v in by_company.values()),
        median_normalized_balance_mae=median(mean(v) for v in by_company.values()),
        new_gap_depth_mae=mean(depths) if depths else None,
        mean_warning_days=mean(leads) if leads else None,
        mean_signed_gap_date_error=mean(offsets) if offsets else None,
        gap_cases=details,
    )
    return result


def compare_companies(candidate, baseline):
    old = defaultdict(list)
    new = defaultdict(list)
    for r in baseline:
        old[r.company].append(r)
    for r in candidate:
        new[r.company].append(r)
    counts = Counter()
    for company, rs in old.items():
        a = aggregate(rs)["balance_mae"]
        b = aggregate(new[company])["balance_mae"]
        counts["improved" if b < a else "worsened" if b > a else "equal"] += 1
    return dict(counts)


def grouped_metrics(records):
    groups = defaultdict(list)
    for r in records:
        groups[r.split, r.method, receipt_type(r.history)].append(r)
    return [
        {"split": split, "method": method, "receipt_type": kind, **product_metrics(rs)}
        for (split, method, kind), rs in sorted(groups.items())
    ]


def diagnostic_row(record, company):
    """Separate horizon amount from timing, without calling either an oracle forecast.

    cumulative error(t) = (horizon total error * t/H) + timing(t).
    Balance identity also includes the actual reconciliation residual (<=1 cent).
    """
    result = {
        "company_id": record.company,
        "cutoff": record.cutoff.isoformat(),
        "split": record.split,
        "receipt_type": receipt_type(record.history),
    }
    h = len(record.truth)
    parts = {}
    signed = {}
    for stream in ("inflow", "outflow"):
        delta = sum(
            getattr(p, stream) - getattr(a, stream)
            for a, p in zip(record.truth, record.predictions)
        )
        accumulated, amount, timing = 0, [], []
        for i, (a, p) in enumerate(zip(record.truth, record.predictions), 1):
            accumulated += getattr(p, stream) - getattr(a, stream)
            level = Fraction(delta * i, h)
            amount.append(abs(level))
            timing.append(abs(accumulated - level))
        parts[stream + "_amount"] = float(sum(amount) / h / 100)
        parts[stream + "_timing"] = float(sum(timing) / h / 100)
        signed[stream] = delta
        result[stream + "_horizon_error_cents"] = delta
    balance = record.initial_balance
    for a in record.truth:
        balance += a.inflow - a.outflow
    residual = record.truth[-1].balance - balance
    if (
        record.predictions[-1].balance - record.truth[-1].balance
        != signed["inflow"] - signed["outflow"] - residual
    ):
        raise DataError("Signed cash-flow decomposition failed")
    channels, counterparties = receipt_channels(company, record.history)
    total = sum(d.inflow for d in record.history)
    result.update(parts)
    result.update(
        dominant_component=max(parts, key=parts.get),
        inflow_positive_days=sum(d.inflow > 0 for d in record.history),
        top3_day_share=sum(sorted((d.inflow for d in record.history), reverse=True)[:3]) / total
        if total
        else None,
        top_counterparty_share=max(counterparties.values()) / total if total else None,
        nonoperating_receipt_share=sum(channels.get("other", {}).values()) / total
        if total
        else None,
        inferred_history_days=sum(not d.observed_balance for d in record.history),
        one_cent_warnings=sum(abs(d.reconciliation or 0) == 1 for d in record.history),
    )
    # Realized exceptional flows diagnose errors, never become forecast features.
    future_categories = Counter()
    for op in company.operations:
        if record.cutoff < op.day <= record.truth[-1].day:
            future_categories[categorize(op).label] += op.amount
    result["future_own_transfer_cents_diagnostic_only"] = sum(
        future_categories[c] for c in ("own_transfer", "own_transfer_candidate")
    )
    result["future_exceptional_cents_diagnostic_only"] = sum(
        future_categories[c]
        for c in (
            "grant_receipt",
            "insurance_receipt",
            "loan_receipt",
            "owner_financing",
            "owner_capital",
        )
    )
    return result


def write_csv(path, rows):
    if not rows:
        return
    with path.open("w", encoding="utf-8-sig", newline="") as stream:
        writer = csv.DictWriter(stream, fieldnames=list(rows[0]), delimiter=";")
        writer.writeheader()
        writer.writerows(rows)


def human_sample(output, companies, prior_review=None):
    """Independent from prior assistant-selected examples, still awaiting humans."""
    reviewed = set()
    if prior_review and prior_review.exists():
        payload = json.loads(prior_review.read_text(encoding="utf-8"))

        def visit(value):
            if isinstance(value, dict):
                if "operation_id" in value:
                    reviewed.add(value["operation_id"])
                for item in value.values():
                    visit(item)
            elif isinstance(value, list):
                for item in value:
                    visit(item)

        visit(payload)
    groups = defaultdict(list)
    for company in companies:
        for op in company.operations:
            label = categorize(op).label
            groups[label, op.incoming].append(op)
    rng = random.Random(20261024)
    rows, key, strata = [], [], []
    for (label, incoming), ops in sorted(groups.items()):
        pool = sorted(
            (op for op in ops if op.operation_id not in reviewed),
            key=lambda op: (op.company, op.operation_id),
        )
        # Up to eight distinct companies before taking further examples.
        rng.shuffle(pool)
        selected, used_companies = [], set()
        for op in pool:
            if op.company not in used_companies:
                selected.append(op)
                used_companies.add(op.company)
            if len(selected) == 8:
                break
        if len(selected) < 8:
            selected_ids = {op.operation_id for op in selected}
            selected += [op for op in pool if op.operation_id not in selected_ids][
                : 8 - len(selected)
            ]
        strata.append(
            {
                "rule_stratum": label,
                "incoming": incoming,
                "population": len(ops),
                "eligible_after_prior_review": len(pool),
                "sampled": len(selected),
            }
        )
        for op in selected:
            sample_id = f"H{len(rows) + 1:04d}"
            rows.append(
                {
                    "sample_id": sample_id,
                    "company_id": op.company,
                    "operation_id": op.operation_id,
                    "date": op.day.isoformat(),
                    "direction": "Поступление" if incoming else "Списание",
                    "amount": money(op.amount),
                    "counterparty": op.name,
                    "purpose": op.purpose,
                    "human_category": "",
                    "alternative": "",
                    "reason": "",
                    "uncertain": "",
                }
            )
            key.append(
                {
                    "sample_id": sample_id,
                    "rule_category": label,
                    "rule": categorize(op).rule,
                    "prior_assistant_reviewed": False,
                }
            )
    audit = output / "human_audit"
    audit.mkdir()
    for annotator in ("A", "B"):
        shuffled = list(rows)
        rng.shuffle(shuffled)
        write_csv(audit / f"blind_{annotator}.csv", shuffled)
    private = audit / "coordinator_only"
    private.mkdir()
    write_csv(private / "rule_key.csv", key)
    dump_json(
        audit / "sampling.json",
        {
            "seed": 20261024,
            "strata": strata,
            "sample_size": len(rows),
            "excluded_prior_review": len(reviewed),
            "human_labels_completed": False,
            "accuracy": None,
        },
    )
    labels = sorted({label for label, _ in groups})
    (audit / "instructions.md").write_text(
        "# Независимая разметка двумя людьми\n\n"
        "Каждый разметчик работает самостоятельно со своим blind CSV. Ключ правил не открывать. "
        "Сначала заполнить категорию, основание, альтернативу и uncertain. Несогласия обсуждать "
        "только после сдачи обоих файлов; сохранить исходные ответы и итог арбитража.\n\n"
        "Назначение важнее имени банка: один банк проводит эквайринг, зарплату и комиссии. "
        "Не считать НДС налоговым платежом. Пустой текст — unknown. Совпадение ИНН/имени "
        "само по себе не доказывает принадлежность другого счёта. Собственные средства без "
        "источника могут быть self_funding_unknown, заем учредителя — owner_financing, "
        "взнос в капитал — owner_capital. Возврат может быть refund_in или retention_receipt; "
        "перечисление агента — agency_revenue, marketplace_revenue или unknown: требуется контекст. "
        "Не придумывать неизвестный контекст; указывать альтернативы.\n\n"
        "Категории: " + ", ".join(labels) + ".\n\n"
        "Выборка стратифицирована по правилам, до 8 примеров на категорию/направление, "
        "с разнообразием компаний и исключением прошлой ручной проверки. Это новая выборка, "
        "но независимость разметки появится только после работы людей. Без неё accuracy нет. "
        "Из-за неравных вероятностей отбора и исключений простая доля совпадений не является "
        "точностью всего набора. Сначала показать матрицу ошибок и согласие по стратам.\n",
        encoding="utf-8",
    )
    return {"sample_size": len(rows), "strata": len(strata), "human_labels_completed": False}
