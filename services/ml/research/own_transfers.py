"""Read-only own-account transfer research; no production model integration.

Usage: python services/ml/research/own_transfers.py
Money is represented in integer minor units. Source files are never rewritten.
The exported labels are rule outcomes, not manual ground truth or probabilities.
"""

from __future__ import annotations

import argparse
import csv
import hashlib
import json
import re
import unicodedata
from collections import Counter, defaultdict
from datetime import date
from decimal import Decimal
from pathlib import Path

CONFIRMED = "подтверждено по доступным признакам"
PROBABLE = "вероятный кандидат"
INSUFFICIENT = "недостаточно данных"
ALTERNATIVE = "признаки другого назначения"
NO_SIGNAL = "нет выделяющего сигнала"
CANDIDATE_TIERS = {CONFIRMED, PROBABLE}
RULE_VERSION = "own-transfers-research-v1"


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read_csv(path: Path) -> tuple[list[str], list[dict]]:
    with path.open(encoding="utf-8-sig", newline="") as handle:
        reader = csv.DictReader(handle, delimiter=";")
        return list(reader.fieldnames or []), list(reader)


def norm(value: str) -> str:
    return " ".join(unicodedata.normalize("NFKC", value).casefold().replace("ё", "е").split())


def name_key(value: str) -> str:
    # Keep legal form and all words; do not fuzzy-merge different entities.
    return " ".join(re.sub(r"[^\w\s-]", " ", norm(value)).split())


def cents(value: str) -> int:
    scaled = Decimal(value) * 100
    if not scaled.is_finite() or scaled != scaled.to_integral_value():
        raise ValueError(f"Invalid minor-unit amount: {value!r}")
    return int(scaled)


def money(value: int) -> str:
    sign = "-" if value < 0 else ""
    value = abs(value)
    return f"{sign}{value // 100}.{value % 100:02d}"


def features(operation: dict, company: dict) -> dict:
    purpose = norm(operation["payment_purpose"])
    own = bool(
        re.search(r"перевод собственных средств|между (?:своими |собственными )?счет", purpose)
    )
    business_account = bool(re.search(r"расчетн\w* счет|\bр/с\b", purpose))
    bank_mention = bool(re.search(r"\bбанк[а-я0-9-]*\b", purpose))
    personal = bool(re.search(r"личн.{0,20}(?:счет|карт)|счет физическ", purpose))
    cash = "взнос наличных" in purpose and "торговая выручка" in purpose
    topup = "пополнение счета собственными средствами" in purpose
    loan = bool(re.search(r"займ|заем|кредит|овердрафт", purpose))
    owner = bool(re.search(r"учредител|уставн\w* капитал", purpose))
    closure = "перечисление остатка денежных средств" in purpose and "закрытием счета" in purpose
    direction_matches = (
        operation["direction"] == "Списание" and "на расчетный счет" in purpose
    ) or (operation["direction"] == "Поступление" and "с расчетного счета" in purpose)
    same_inn = bool(operation["counterparty_inn"]) and (
        operation["counterparty_inn"] == company["company_inn"]
    )
    same_name = bool(operation["counterparty_name"]) and (
        name_key(operation["counterparty_name"]) == name_key(company["company_name"])
    )
    return {
        "inn_equal": same_inn,
        "name_equal": same_name,
        "own_transfer_wording": own,
        "own_funds_any_wording": "собственн" in purpose or own,
        "business_account_wording": business_account,
        "bank_mention": bank_mention,
        "closure_transfer": closure,
        "direction_matches_text": direction_matches,
        "personal_use": personal,
        "cash_revenue": cash,
        "ambiguous_self_topup": topup,
        "loan_wording": loan,
        "owner_capital_wording": owner,
        "purpose_missing": not purpose,
        "counterparty_inn_missing": not operation["counterparty_inn"],
        "account_number_in_text": bool(re.search(r"(?<!\d)\d{20}(?!\d)", purpose)),
        "bank_labels_in_text": ", ".join(sorted(set(re.findall(r"банк-\d+", purpose)))),
    }


def decide(operation: dict, company: dict) -> dict:
    f = features(operation, company)
    signal = (
        f["inn_equal"] or f["name_equal"] or f["own_funds_any_wording"] or f["closure_transfer"]
    )
    evidence = [key for key, value in f.items() if value and isinstance(value, bool)]
    tier, rule, alternative = (
        NO_SIGNAL,
        "R0_NO_SIGNAL",
        "Нет положительного признака; это не доказательство отсутствия перевода.",
    )
    if signal:
        if f["cash_revenue"]:
            tier, rule = ALTERNATIVE, "R1_CASH_REVENUE"
            alternative = "Внесение наличной торговой выручки; совпадение ИНН не доказывает перевод между банками."
        elif f["personal_use"]:
            tier, rule = ALTERNATIVE, "R2_PERSONAL_USE"
            alternative = (
                "Вывод владельцу/на личный счет ИП; принадлежность деловому контуру не установлена."
            )
        elif f["loan_wording"] or f["owner_capital_wording"]:
            tier, rule = ALTERNATIVE, "R3_FINANCING"
            alternative = (
                "Заем, кредит или взнос владельца; не считать переводом только по слову счет."
            )
        elif (
            f["inn_equal"]
            and f["name_equal"]
            and f["own_transfer_wording"]
            and f["business_account_wording"]
            and f["bank_mention"]
            and f["direction_matches_text"]
        ):
            tier, rule = CONFIRMED, "R4_SELF_INN_EXPLICIT_INTERBANK"
            alternative = "Текст/реквизиты могут быть ошибочны; конкретный второй счет и связь двух проводок не подтверждены."
        elif (
            f["inn_equal"]
            and f["name_equal"]
            and f["closure_transfer"]
            and f["bank_mention"]
            and operation["direction"] == "Списание"
        ):
            tier, rule = PROBABLE, "R5_CLOSURE_TO_SAME_ENTITY"
            alternative = "Похоже на перенос остатка, но нет номера/типа счета назначения и явной формулировки собственных средств."
        elif (
            f["counterparty_inn_missing"]
            and f["name_equal"]
            and f["own_transfer_wording"]
            and f["business_account_wording"]
            and f["bank_mention"]
            and f["direction_matches_text"]
        ):
            tier, rule = PROBABLE, "R6_NAME_AND_TEXT_WITHOUT_INN"
            alternative = "Одноименная организация или неверные реквизиты; требуется подтверждение принадлежности."
        else:
            tier, rule = INSUFFICIENT, "R7_SELF_SIGNAL_AMBIGUOUS"
            alternative = (
                "Пополнение личными средствами владельца либо перевод с другого делового счета; источник не указан."
                if f["ambiguous_self_topup"]
                else "Внесение наличных, поддержка владельца, личный перевод или межбанковский перевод; назначение не раскрывает источник."
            )
    return {
        **operation,
        "company_inn": company["company_inn"],
        "company_name": company["company_name"],
        "legal_form": company["legal_form"],
        **f,
        "screen_signal": bool(signal),
        "tier": tier,
        "rule": rule,
        "reasons": ", ".join(evidence),
        "alternative_explanation": alternative,
    }


def summarize(rows: list[dict]) -> dict:
    return {
        "operations": len(rows),
        "companies": len({r["company_id"] for r in rows}),
        "company_ids": sorted({r["company_id"] for r in rows}),
        "incoming_operations": sum(r["direction"] == "Поступление" for r in rows),
        "outgoing_operations": sum(r["direction"] == "Списание" for r in rows),
        "incoming_amount": money(
            sum(cents(r["amount"]) for r in rows if r["direction"] == "Поступление")
        ),
        "outgoing_amount": money(
            sum(cents(r["amount"]) for r in rows if r["direction"] == "Списание")
        ),
    }


def write_csv(path: Path, rows: list[dict], fields: list[str] | None = None) -> None:
    with path.open("w", encoding="utf-8-sig", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=fields or list(rows[0]), delimiter=";")
        writer.writeheader()
        writer.writerows(rows)


def pair_diagnostics(rows: list[dict]) -> tuple[dict, list[dict]]:
    grouped = defaultdict(list)
    for row in rows:
        grouped[row["company_id"], cents(row["amount"])].append(row)
    pairs = []
    for group in grouped.values():
        incoming = [r for r in group if r["direction"] == "Поступление"]
        outgoing = [r for r in group if r["direction"] == "Списание"]
        for a in incoming:
            for b in outgoing:
                lag = abs(
                    (
                        date.fromisoformat(a["operation_date"])
                        - date.fromisoformat(b["operation_date"])
                    ).days
                )
                if lag <= 7:
                    pairs.append(
                        {
                            "company_id": a["company_id"],
                            "incoming_operation_id": a["operation_id"],
                            "outgoing_operation_id": b["operation_id"],
                            "incoming_date": a["operation_date"],
                            "outgoing_date": b["operation_date"],
                            "amount": a["amount"],
                            "calendar_day_distance": lag,
                            "incoming_purpose": a["payment_purpose"],
                            "outgoing_purpose": b["payment_purpose"],
                            "same_counterparty_inn": bool(a["counterparty_inn"])
                            and a["counterparty_inn"] == b["counterparty_inn"],
                            "incoming_candidate": a["tier"] in CANDIDATE_TIERS,
                            "outgoing_candidate": b["tier"] in CANDIDATE_TIERS,
                        }
                    )
    summary = {}
    for window in (0, 3, 7):
        selected = [p for p in pairs if p["calendar_day_distance"] <= window]
        summary[str(window)] = {
            "possible_pairs": len(selected),
            "companies": len({p["company_id"] for p in selected}),
            "pairs_with_any_transfer_candidate": sum(
                p["incoming_candidate"] or p["outgoing_candidate"] for p in selected
            ),
            "pairs_with_both_transfer_candidates": sum(
                p["incoming_candidate"] and p["outgoing_candidate"] for p in selected
            ),
        }
    return summary, pairs


def annotation_sample(rows: list[dict]) -> tuple[list[dict], list[dict]]:
    # Fixed, disjoint strata; labels remain blank for independent human reviewers.
    # Retain rule outputs in a separate CSV, not in this blind annotation sample.
    strata = defaultdict(list)
    for row in rows:
        if row["tier"] in {CONFIRMED, PROBABLE, INSUFFICIENT}:
            key = row["tier"]
        elif row["personal_use"]:
            key = "personal_control"
        elif row["cash_revenue"]:
            key = "cash_revenue_control"
        elif row["loan_wording"] or row["owner_capital_wording"]:
            key = "financing_control"
        elif row["purpose_missing"]:
            key = "missing_purpose_control"
        else:
            key = "other_control"
        strata[key].append(row)
    limits = {CONFIRMED: 100, PROBABLE: 100, INSUFFICIENT: 200}
    sample = []
    design = []
    for key, pool in sorted(strata.items()):
        # Round robin across companies, with hash ordering independent of label.
        by_company = defaultdict(list)
        for row in sorted(
            pool, key=lambda r: hashlib.sha256(r["operation_id"].encode()).hexdigest()
        ):
            by_company[row["company_id"]].append(row)
        chosen = []
        while by_company and len(chosen) < limits.get(key, 40):
            for company_id in sorted(list(by_company)):
                if len(chosen) >= limits.get(key, 40):
                    break
                chosen.append(by_company[company_id].pop())
                if not by_company[company_id]:
                    del by_company[company_id]
        for row in chosen:
            sample.append(
                {
                    **{
                        k: row[k]
                        for k in (
                            "company_id",
                            "operation_id",
                            "operation_date",
                            "amount",
                            "direction",
                            "company_inn",
                            "company_name",
                            "counterparty_inn",
                            "counterparty_name",
                            "payment_purpose",
                        )
                    },
                    "reviewer_1_label": "",
                    "reviewer_2_label": "",
                    "adjudicated_label": "",
                    "account_ownership_verified": "",
                    "review_notes": "",
                }
            )
            design.append(
                {
                    "company_id": row["company_id"],
                    "operation_id": row["operation_id"],
                    "sampling_stratum": key,
                    "stratum_population": len(pool),
                    "stratum_sample_size": len(chosen),
                }
            )
    # Do not reveal sampling strata/rule predictions through CSV row ordering.
    sample.sort(key=lambda r: hashlib.sha256(r["operation_id"].encode()).hexdigest())
    return sample, design


def main() -> None:
    repo = Path(__file__).resolve().parents[3]
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data", type=Path)
    parser.add_argument("--output", type=Path, default=repo / "services/ml/artifacts/own_transfers")
    args = parser.parse_args()
    source = args.data or (
        repo / "dataset"
        if (repo / "dataset/companies.csv").is_file()
        else repo / "data/cashflow/raw"
    )
    source = source.resolve()
    output = args.output.resolve()
    if output == source or source in output.parents:
        raise ValueError("Output must be outside the source dataset directory")
    source_files = sorted(p for p in source.rglob("*") if p.is_file())
    hashes_before = {str(p.relative_to(source)): digest(p) for p in source_files}
    _, company_rows = read_csv(source / "companies.csv")
    companies = {r["company_id"]: r for r in company_rows}
    assert len(companies) == len(company_rows)
    fields, operations = read_csv(source / "all_operations.csv")
    assert len({r["operation_id"] for r in operations}) == len(operations)
    by_company = defaultdict(list)
    for row in operations:
        assert row["company_id"] in companies
        assert row["direction"] in {"Поступление", "Списание"}
        assert cents(row["amount"]) > 0
        cents(row["balance_eod"])
        date.fromisoformat(row["operation_date"])
        by_company[row["company_id"]].append(row)
    statement_rows = 0
    for company_id, company in companies.items():
        statement_fields, statement = read_csv(source / "statements" / f"{company_id}.csv")
        assert statement_fields == fields
        assert Counter(tuple(r[f] for f in fields) for r in statement) == Counter(
            tuple(r[f] for f in fields) for r in by_company[company_id]
        )
        assert int(company["operations_count"]) == len(statement)
        statement_rows += len(statement)
    assert statement_rows == len(operations)
    labeled = [decide(r, companies[r["company_id"]]) for r in operations]
    candidates = [r for r in labeled if r["tier"] in CANDIDATE_TIERS]
    screened = [r for r in labeled if r["screen_signal"]]
    tiers = {
        tier: summarize([r for r in labeled if r["tier"] == tier])
        for tier in (CONFIRMED, PROBABLE, INSUFFICIENT, ALTERNATIVE, NO_SIGNAL)
    }
    assert sum(t["operations"] for t in tiers.values()) == len(operations)
    pair_summary, pairs = pair_diagnostics(labeled)
    feature_summary = {
        f: summarize([r for r in labeled if r[f]])
        for f, value in labeled[0].items()
        if isinstance(value, bool)
    }
    company_inns = {r["company_inn"]: r["company_id"] for r in company_rows}
    cross_company_matches = [
        r
        for r in labeled
        if r["counterparty_inn"] in company_inns
        and company_inns[r["counterparty_inn"]] != r["company_id"]
    ]
    summary = {
        "rule_version": RULE_VERSION,
        "source_directory": str(source),
        "operations": len(operations),
        "companies": len(companies),
        "date_min": min(r["operation_date"] for r in operations),
        "date_max": max(r["operation_date"] for r in operations),
        "identical_statement_rows_checked_not_added": statement_rows,
        "unique_company_inns": len(company_inns),
        "cross_company_inn_matches": len(cross_company_matches),
        "screened": summarize(screened),
        "candidates": summarize(candidates),
        "candidates_and_unresolved": summarize(
            [r for r in labeled if r["tier"] in CANDIDATE_TIERS | {INSUFFICIENT}]
        ),
        "tiers": tiers,
        "features": feature_summary,
        "rule_counts": dict(Counter(r["rule"] for r in labeled)),
        "same_amount_opposite_direction_diagnostics": pair_summary,
        "source_sha256": hashes_before,
        "script_sha256": digest(Path(__file__)),
        "manual_labels_available": False,
        "measured_accuracy": None,
        "account_pairs_confirmed": 0,
    }
    output.mkdir(parents=True, exist_ok=True)
    write_csv(output / "screened_operations.csv", screened)
    write_csv(output / "candidates.csv", candidates)
    write_csv(output / "amount_date_comparisons.csv", pairs)
    sample, design = annotation_sample(labeled)
    write_csv(output / "manual_annotation.csv", sample)
    write_csv(output / "annotation_design.csv", design)
    summary["manual_annotation_sample"] = dict(Counter(r["sampling_stratum"] for r in design))
    company_coverage = []
    for company_id in sorted(companies):
        company_coverage.append(
            {
                "company_id": company_id,
                "company_inn": companies[company_id]["company_inn"],
                "company_name": companies[company_id]["company_name"],
                "total_operations": len(by_company[company_id]),
                **{
                    tier: sum(r["company_id"] == company_id and r["tier"] == tier for r in screened)
                    for tier in (CONFIRMED, PROBABLE, INSUFFICIENT, ALTERNATIVE)
                },
                "bank_labels_from_text_not_account_ids": ", ".join(
                    sorted(
                        {
                            r["bank_labels_in_text"]
                            for r in candidates
                            if r["company_id"] == company_id and r["bank_labels_in_text"]
                        }
                    )
                ),
            }
        )
    write_csv(output / "company_coverage.csv", company_coverage)
    hashes_after = {str(p.relative_to(source)): digest(p) for p in source_files}
    assert hashes_after == hashes_before, "Source files changed during research"
    summary["source_hashes_unchanged"] = True
    (output / "summary.json").write_text(
        json.dumps(summary, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    print(
        json.dumps(
            {
                "output": str(output),
                "candidates": summary["candidates"],
                "tiers": {k: (v["operations"], v["companies"]) for k, v in tiers.items()},
                "manual_sample": len(sample),
                "sources_unchanged": True,
            },
            ensure_ascii=False,
        )
    )


if __name__ == "__main__":
    main()
