"""Monthly hypotheses learned from past operations, not known future liabilities."""

import calendar
from collections import defaultdict
from dataclasses import dataclass, replace
from datetime import date, timedelta
from statistics import mean, median
from typing import List, Tuple

from .categories import categorize, normalize
from .data import Company, Daily, DataError, Operation, daily_as_of
from .models import Prediction, predict

REGULAR_EXPENSES = {
    "salary",
    "salary_advance",
    "salary_final",
    "tax",
    "rent",
    "equipment_rent",
    "bank_fee",
    "utilities",
    "subscription",
    "loan_payment",
    "overdraft_interest",
}
EXCLUDED = {
    "self_funding_unknown",
    "owner_capital",
    "own_transfer_candidate",
    "insurance_receipt",
    "grant_receipt",
    "enforcement_payment",
    "retention_receipt",
    "marketplace_revenue",
    "unknown",
    "owner_financing",
    "owner_repayment",
    "owner_withdrawal",
    "own_transfer",
    "refund_in",
    "refund_out",
    "acquiring_revenue",
    "cash_revenue",
    "loan_receipt",
}
TOLERANCE = 4


@dataclass(frozen=True)
class RecurringPattern:
    category: str
    counterparty: str
    incoming: bool
    anchor: int
    weekend_shift: str
    amount: int
    confirmations: int
    expected_periods: int
    mean_date_error: float
    relative_amount_mad: float
    matched_ids: frozenset
    paid_periods: frozenset
    first_evidence: date
    last_evidence: date


@dataclass(frozen=True)
class ExpectedEvent:
    day: date
    amount: int
    incoming: bool
    pattern_index: int


def scheduled_date(year: int, month: int, anchor: int, weekend_shift: str) -> date:
    last = calendar.monthrange(year, month)[1]
    target = date(year, month, last if anchor == 0 else min(anchor, last))
    if weekend_shift not in {"none", "previous", "next"}:
        raise DataError("Unknown weekend shift")
    while target.weekday() >= 5 and weekend_shift != "none":
        target += timedelta(days=-1 if weekend_shift == "previous" else 1)
    return target


def months_between(first: date, last: date):
    year, month = first.year, first.month
    while (year, month) <= (last.year, last.month):
        yield year, month
        year, month = (year + 1, 1) if month == 12 else (year, month + 1)


def fit_monthly(operations: List[Operation], category: str, cutoff: date):
    grouped = defaultdict(list)
    for operation in operations:
        grouped[operation.day].append(operation)
    events = sorted(
        (day, sum(op.amount for op in values), values) for day, values in grouped.items()
    )
    minimum = 3 if category in REGULAR_EXPENSES and not operations[0].incoming else 4
    if len(events) < minimum:
        return None
    # Ignore tiny penalties when fitting the main schedule; keep them in residual flows.
    monthly_max = defaultdict(int)
    for day, amount, _ in events:
        monthly_max[(day.year, day.month)] = max(monthly_max[(day.year, day.month)], amount)
    floor = median(monthly_max.values()) * 0.25
    events = [event for event in events if event[1] >= floor]
    if len(events) < minimum or (cutoff - events[-1][0]).days > 45:
        return None
    start = events[0][0] - timedelta(days=TOLERANCE)
    periods = list(months_between(start, cutoff + timedelta(days=TOLERANCE)))
    best = None
    for anchor in [0] + list(range(1, 32)):
        for shift in ["previous", "next", "none"]:
            used, matched = set(), []
            expected = 0
            for year, month in periods:
                due = scheduled_date(year, month, anchor, shift)
                if due < start or due > cutoff + timedelta(days=TOLERANCE):
                    continue
                expected += int(due <= cutoff)
                candidates = [
                    (abs((event[0] - due).days), index)
                    for index, event in enumerate(events)
                    if index not in used and abs((event[0] - due).days) <= TOLERANCE
                ]
                if candidates:
                    distance, index = min(candidates)
                    used.add(index)
                    matched.append((due, (year, month), events[index], distance))
            confirmed = [match for match in matched if match[0] <= cutoff]
            if len(confirmed) < minimum or not expected or len(confirmed) / expected < 0.75:
                continue
            amounts = [match[2][1] for match in confirmed]
            center = median(amounts)
            mad = median(abs(amount - center) for amount in amounts) / center
            if category not in REGULAR_EXPENSES and mad > 0.3:
                continue
            date_error = mean(match[3] for match in confirmed)
            score = (1 - len(confirmed) / expected, date_error, -len(confirmed), mad)
            if best is None or score < best[0]:
                recent = sorted(matched, key=lambda match: match[2][0])[-3:]
                amount = int(median(match[2][1] for match in recent))
                pattern = RecurringPattern(
                    category,
                    operations[0].name,
                    operations[0].incoming,
                    anchor,
                    shift,
                    amount,
                    len(confirmed),
                    expected,
                    date_error,
                    mad,
                    frozenset(op.operation_id for match in matched for op in match[2][2]),
                    frozenset(match[1] for match in matched),
                    min(match[2][0] for match in matched),
                    max(match[2][0] for match in matched),
                )
                best = score, pattern
    return best[1] if best else None


def infer_patterns(company: Company, cutoff: date, max_history_days: int = 180):
    snapshot = daily_as_of(company, cutoff)
    first = cutoff - timedelta(days=max_history_days - 1)
    groups = defaultdict(list)
    for operation in company.operations:
        if not first <= operation.day <= cutoff:
            continue
        quality = snapshot[operation.day]
        if quality.gap or quality.duplicates:
            continue
        category = categorize(operation).label
        if category in EXCLUDED:
            continue
        counterparty = operation.inn or normalize(operation.name)
        groups[(operation.incoming, counterparty, category)].append(operation)
    patterns = []
    for (_, _, category), operations in sorted(groups.items()):
        pattern = fit_monthly(operations, category, cutoff)
        if pattern is not None:
            patterns.append(pattern)
    return patterns


def future_events(patterns, cutoff: date, horizon: int):
    end = cutoff + timedelta(days=horizon)
    # Due dates shifted over a month boundary may belong to an adjacent nominal month.
    first_period = cutoff.replace(day=1) - timedelta(days=1)
    last_period = end + timedelta(days=7)
    result = []
    for index, pattern in enumerate(patterns):
        for year, month in months_between(first_period, last_period):
            day = scheduled_date(year, month, pattern.anchor, pattern.weekend_shift)
            if cutoff < day <= end and (year, month) not in pattern.paid_periods:
                result.append(ExpectedEvent(day, pattern.amount, pattern.incoming, index))
    return sorted(result, key=lambda event: (event.day, event.pattern_index))


def hybrid_predict(company: Company, history: List[Daily], horizon: int) -> Tuple[list, list, list]:
    cutoff = history[-1].day
    patterns = infer_patterns(company, cutoff)
    matched_ids = set()
    for pattern in patterns:
        if matched_ids & pattern.matched_ids:
            raise DataError("A historical operation belongs to more than one pattern")
        matched_ids.update(pattern.matched_ids)
    removed = defaultdict(lambda: [0, 0])
    for operation in company.operations:
        if operation.day <= cutoff and operation.operation_id in matched_ids:
            removed[operation.day][0 if operation.incoming else 1] += operation.amount
    residual = []
    for day in history:
        incoming = day.inflow - removed[day.day][0]
        outgoing = day.outflow - removed[day.day][1]
        if incoming < 0 or outgoing < 0:
            raise DataError("Recurring-event subtraction exceeded observed flows")
        residual.append(replace(day, inflow=incoming, outflow=outgoing))
    forecasts = predict(residual, horizon, "weekday_8w")
    events = future_events(patterns, cutoff, horizon)
    additions = defaultdict(lambda: [0, 0])
    for event in events:
        additions[event.day][0 if event.incoming else 1] += event.amount
    result, balance = [], history[-1].balance
    for prediction in forecasts:
        incoming = prediction.inflow + additions[prediction.day][0]
        outgoing = prediction.outflow + additions[prediction.day][1]
        balance += incoming - outgoing
        result.append(Prediction(prediction.day, incoming, outgoing, balance))
    return result, patterns, events
