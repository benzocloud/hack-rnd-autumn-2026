"""Exact CSV loading and date-aware reconstruction of bank statement history."""

import csv
import hashlib
from dataclasses import dataclass
from datetime import date, timedelta
from decimal import Decimal, InvalidOperation
from pathlib import Path
from typing import Dict, List, Optional, Tuple


class DataError(ValueError):
    """Input data cannot safely be used for the requested operation."""


def parse_money(value: str) -> int:
    try:
        number = Decimal(value)
        if not number.is_finite() or number * 100 != (number * 100).to_integral_value():
            raise DataError("Money must be finite and have at most two decimal places")
        return int(number * 100)
    except InvalidOperation as exc:
        raise DataError("Invalid monetary value") from exc


def money(value: int) -> str:
    return f"{'-' if value < 0 else ''}{abs(value) // 100}.{abs(value) % 100:02d}"


@dataclass(frozen=True)
class Operation:
    company: str
    operation_id: str
    day: date
    amount: int
    incoming: bool
    inn: str
    name: str
    purpose: str
    balance: int

    def content_key(self) -> tuple:
        return (
            self.company,
            self.day,
            self.amount,
            self.incoming,
            self.inn,
            self.name,
            self.purpose,
            self.balance,
        )


@dataclass(frozen=True)
class Daily:
    day: date
    inflow: int
    outflow: int
    balance: Optional[int]
    observed_balance: bool
    gap: bool
    duplicates: int
    reconciliation: Optional[int] = None


@dataclass
class Company:
    company_id: str
    industry: str
    operations: List[Operation]

    @property
    def first_date(self) -> date:
        return min(op.day for op in self.operations)

    @property
    def last_date(self) -> date:
        return max(op.day for op in self.operations)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def load_companies(data_dir: Path) -> List[Company]:
    with (data_dir / "companies.csv").open(encoding="utf-8-sig", newline="") as source:
        metadata = list(csv.DictReader(source, delimiter=";"))
    industries = {row["company_id"]: row["industry"] for row in metadata}
    if len(industries) != len(metadata):
        raise DataError("Duplicate company ID in companies.csv")
    grouped: Dict[str, List[Operation]] = {cid: [] for cid in industries}
    identifiers = set()
    with (data_dir / "all_operations.csv").open(encoding="utf-8-sig", newline="") as source:
        for row in csv.DictReader(source, delimiter=";"):
            cid = row["company_id"]
            if cid not in grouped:
                raise DataError("Operation references an unknown company")
            key = (cid, row["operation_id"])
            if key in identifiers:
                raise DataError("Duplicate operation ID; no automatic overwrite allowed")
            identifiers.add(key)
            if row["direction"] not in {"Поступление", "Списание"}:
                raise DataError("Unknown operation direction")
            amount = parse_money(row["amount"])
            if amount <= 0:
                raise DataError("Operation amount must be positive")
            grouped[cid].append(
                Operation(
                    cid,
                    row["operation_id"],
                    date.fromisoformat(row["operation_date"]),
                    amount,
                    row["direction"] == "Поступление",
                    row["counterparty_inn"],
                    row["counterparty_name"],
                    row["payment_purpose"],
                    parse_money(row["balance_eod"]),
                )
            )
    result = []
    for cid, operations in sorted(grouped.items()):
        if not operations:
            raise DataError("Company has no operations")
        operations.sort(key=lambda op: (op.day, op.operation_id))
        result.append(Company(cid, industries[cid], operations))
    return result


def daily_as_of(company: Company, as_of: date) -> Dict[date, Daily]:
    """Later operations cannot affect values or quality flags in this snapshot.

    Missing rows between observed dates are assumed to mean zero operations.
    A newly observed inconsistent balance invalidates its preceding interval.
    This assumption cannot detect missing movements with net zero effect.
    """
    grouped: Dict[date, list] = {}
    seen = set()
    for op in company.operations:
        if op.day > as_of:
            continue
        values = grouped.setdefault(op.day, [0, 0, op.balance, 0])
        if values[2] != op.balance:
            raise DataError("Conflicting end-of-day balances on the same date")
        values[0 if op.incoming else 1] += op.amount
        key = op.content_key()
        values[3] += int(key in seen)
        seen.add(key)
    if not grouped:
        return {}
    observed = sorted(grouped)
    gaps = set()
    residuals = {}
    for previous, current in zip(observed, observed[1:]):
        incoming, outgoing, closing, _ = grouped[current]
        delta = closing - grouped[previous][2] - incoming + outgoing
        residuals[current] = delta
        if abs(delta) > 1:
            cursor = previous + timedelta(days=1)
            while cursor <= current:
                gaps.add(cursor)
                cursor += timedelta(days=1)
    result = {}
    cursor = observed[0]
    balance = None
    while cursor <= as_of:
        values = grouped.get(cursor)
        if values is not None:
            incoming, outgoing, balance, duplicates = values
        else:
            incoming, outgoing, duplicates = 0, 0, 0
        result[cursor] = Daily(
            cursor,
            incoming,
            outgoing,
            None if cursor in gaps and values is None else balance,
            values is not None,
            cursor in gaps,
            duplicates,
            residuals.get(cursor),
        )
        cursor += timedelta(days=1)
    return result


def history_at(company: Company, cutoff: date, lookback: int) -> Tuple[List[Daily], str]:
    if cutoff > company.last_date or cutoff < company.first_date:
        return [], "cutoff_outside_observed_range"
    if (cutoff - company.first_date).days + 1 < lookback:
        return [], "short_history"
    # Crucially, this is reconstructed as of cutoff, not filtered from a full-history export.
    snapshot = daily_as_of(company, cutoff)
    if cutoff not in snapshot or not snapshot[cutoff].observed_balance:
        return [], "cutoff_balance_not_observed"
    start = cutoff - timedelta(days=lookback - 1)
    history = [snapshot[start + timedelta(days=i)] for i in range(lookback)]
    if any(day.gap for day in history):
        return [], "history_reconciliation_gap"
    if any(day.duplicates for day in history):
        return [], "history_duplicate_candidate"
    return history, ""
