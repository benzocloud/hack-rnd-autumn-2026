"""Past-only receipt channels, explicit event timing and expense ablations.

Occurrence scores are empirical/model scores, not calibrated probabilities.
Every source flow is assigned once; own transfers stay in the other-flow level.
"""

import math
from collections import defaultdict
from dataclasses import replace
from datetime import timedelta
from fractions import Fraction
from statistics import median

from .categories import categorize, normalize
from .data import DataError
from .inflow import round_fraction
from .models import Prediction, predict
from .recurring import REGULAR_EXPENSES, future_events, infer_patterns

REVENUE = frozenset(
    {
        "acquiring_revenue",
        "cash_revenue",
        "customer_payment",
        "marketplace_revenue",
        "agency_revenue",
    }
)
RECEIPT_METHODS = ("channel_expected", "channel_events", "logistic_expected")


def rounded_median(values):
    ordered = sorted(values)
    n = len(ordered)
    if not n:
        return 0
    return round_fraction(Fraction(ordered[(n - 1) // 2] + ordered[n // 2], 2))


def receipt_type(history):
    count = sum(day.inflow > 0 for day in history[-90:])
    return "frequent" if count >= 45 else "sparse" if count <= 18 else "mixed"


def feature_row(values, index, day):
    """All covariates precede the target day; values contain integer amounts."""
    past = values[:index]
    recent = past[-28:]
    last = next((i for i, value in enumerate(reversed(past), 1) if value), 29)
    return [
        1.0,
        *[float(day.weekday() == k) for k in range(6)],
        sum(v > 0 for v in past[-7:]) / 7,
        sum(v > 0 for v in recent) / 28,
        min(last, 28) / 28,
    ]


def sigmoid(value):
    return 1 / (1 + math.exp(-max(-30, min(30, value))))


def fit_occurrence(values, days):
    """Ten coefficients, fixed regularization/iterations; no future/global fit."""
    if len(values) < 84 or sum(v > 0 for v in values[28:]) < 12:
        return None
    rows = [feature_row(values, i, days[i]) for i in range(28, len(values))]
    labels = [int(v > 0) for v in values[28:]]
    weights = [0.0] * len(rows[0])
    for _ in range(60):
        gradient = [0.0] * len(weights)
        for row, label in zip(rows, labels):
            error = sigmoid(sum(a * b for a, b in zip(weights, row))) - label
            for j, value in enumerate(row):
                gradient[j] += error * value
        for j in range(len(weights)):
            weights[j] -= 0.6 * (gradient[j] / len(rows) + (0.02 * weights[j] if j else 0))
    return weights


def receipt_channels(company, history):
    cutoff = history[-1].day
    first = history[0].day
    channels = defaultdict(lambda: defaultdict(int))
    counterparties = defaultdict(int)
    for operation in company.operations:
        if first <= operation.day <= cutoff and operation.incoming:
            label = categorize(operation).label
            channels[label if label in REVENUE else "other"][operation.day] += operation.amount
            counterparties[operation.inn or operation.name] += operation.amount
    if sum(sum(days.values()) for days in channels.values()) != sum(d.inflow for d in history):
        raise DataError("Receipt channels must conserve every historical incoming unit")
    return channels, counterparties


def channel_forecast(values, days, horizon, method):
    cutoff = days[-1]
    future = [cutoff + timedelta(days=i) for i in range(1, horizon + 1)]
    positive = [(day, value) for day, value in zip(days, values) if value]
    flat = round_fraction(Fraction(sum(values), len(values)))
    evidence = {"positive_days": len(positive), "mode": "weekday", "scores": []}
    if len(positive) < 8 or len(positive) * 3 < len(values):
        evidence["mode"] = "irregular_mean_fallback"
        if method == "channel_events" and len(positive) >= 5:
            intervals = [(b[0] - a[0]).days for a, b in zip(positive, positive[1:])]
            interval = rounded_median(intervals)
            mad = median(abs(i - interval) for i in intervals)
            if (
                interval >= 2
                and mad <= interval / 4
                and (cutoff - positive[-1][0]).days <= interval
            ):
                next_day = positive[-1][0] + timedelta(days=interval)
                event_days = set()
                while next_day <= future[-1]:
                    if next_day > cutoff:
                        event_days.add(next_day)
                    next_day += timedelta(days=interval)
                evidence.update(mode="renewal_events", interval_days=interval)
                amount = rounded_median([amount for _, amount in positive[-6:]])
                return [amount if day in event_days else 0 for day in future], evidence
        return [flat] * horizon, evidence

    weights = fit_occurrence(values, days) if method == "logistic_expected" else None
    evidence["mode"] = "logistic_expected" if weights is not None else "weekday"
    evidence["fitted_coefficients"] = weights
    simulated = list(values)
    expected, scores, amounts = [], [], []
    for day in future:
        sample = [v for d, v in zip(days[-56:], values[-56:]) if d.weekday() == day.weekday()]
        nonzero = [v for v in sample if v]
        conditional = Fraction(sum(nonzero), len(nonzero)) if nonzero else Fraction(0)
        empirical = Fraction(len(nonzero), len(sample))
        if weights is not None:
            row = feature_row(simulated, len(simulated), day)
            # Fixed-point score makes subsequent money operations exact.
            score = Fraction(
                round(sigmoid(sum(a * b for a, b in zip(weights, row))) * 1000000), 1000000
            )
        else:
            score = empirical
        incoming = round_fraction(score * conditional)
        expected.append(incoming)
        scores.append(score)
        amounts.append(conditional)
        # Recursive features use predictions, never actual future observations.
        simulated.append(round_fraction(conditional) if score >= Fraction(1, 2) else 0)
    evidence["scores"] = [float(p) for p in scores]
    if method != "channel_events":
        return expected, evidence
    slots = [i for i, p in enumerate(scores) if p >= Fraction(1, 2) and amounts[i]]
    if not slots:
        evidence["mode"] = "no_stable_slots_expected_fallback"
        return expected, evidence
    # Keep the expected horizon budget, allocate it to explicit dates exactly.
    budget = sum(expected)
    total_weight = sum(amounts[i] for i in slots)
    result, allocated, weight = [0] * horizon, 0, Fraction(0)
    for i in slots:
        weight += amounts[i]
        cumulative = round_fraction(Fraction(budget) * weight / total_weight)
        result[i] = cumulative - allocated
        allocated = cumulative
    evidence["mode"] = "weekday_events"
    return result, evidence


def incoming_forecast(company, history, horizon, method):
    if method not in RECEIPT_METHODS:
        raise DataError("Unknown receipt method")
    predict(history, horizon, "mean_90")  # Same mandatory quality/cutoff checks.
    channels, counterparties = receipt_channels(company, history)
    days = [d.day for d in history]
    result, evidence = [0] * horizon, {}
    for label, series in sorted(channels.items()):
        values = [series[d] for d in days]
        if label == "other":
            forecast = [round_fraction(Fraction(sum(values), len(values)))] * horizon
            detail = {"mode": "other_mean_preserved", "history_total_cents": sum(values)}
        else:
            forecast, detail = channel_forecast(values, days, horizon, method)
        evidence[label] = detail
        result = [a + b for a, b in zip(result, forecast)]
    total = sum(counterparties.values())
    return result, {
        "channels": evidence,
        "receipt_type": receipt_type(history),
        "top_counterparty_share": max(counterparties.values()) / total if total else None,
        "as_of": history[-1].day.isoformat(),
    }


def expense_calendar(company, history, horizon):
    """Strict expense-only patterns; subtract matched flows, add events once."""
    cutoff = history[-1].day
    # infer_patterns ignores past gap/duplicate days. No future liability is supplied.
    patterns = [
        p
        for p in infer_patterns(company, cutoff)
        if not p.incoming
        and p.category in REGULAR_EXPENSES
        and p.confirmations >= 4
        and p.confirmations * 5 >= p.expected_periods * 4
        and p.mean_date_error <= 1
        and p.relative_amount_mad <= 0.15
    ]
    # A monthly subset of a daily fee stream is not evidence of a monthly bill.
    accepted = []
    known = [op for op in company.operations if cutoff - timedelta(days=179) <= op.day <= cutoff]
    for pattern in patterns:
        matches = [op for op in known if op.operation_id in pattern.matched_ids]
        key = matches[0].inn or normalize(matches[0].name)
        group_days = {
            op.day
            for op in known
            if not op.incoming
            and (op.inn or normalize(op.name)) == key
            and categorize(op).label == pattern.category
        }
        matched_days = {op.day for op in matches}
        if len(matched_days) * 4 < len(group_days) * 3:
            continue
        by_day = defaultdict(int)
        for op in matches:
            by_day[op.day] += op.amount
        exact_amount = rounded_median([by_day[d] for d in sorted(by_day)[-3:]])
        accepted.append(replace(pattern, amount=exact_amount))
    patterns = accepted
    matched = set()
    for pattern in patterns:
        if matched & pattern.matched_ids:
            raise DataError("Expense operation matched more than once")
        matched.update(pattern.matched_ids)
    removed = defaultdict(int)
    for op in company.operations:
        if op.day <= cutoff and op.operation_id in matched:
            removed[op.day] += op.amount
    residual = []
    for day in history:
        if removed[day.day] > day.outflow:
            raise DataError("Expense subtraction exceeded historical outflow")
        residual.append(replace(day, outflow=day.outflow - removed[day.day]))
    base = predict(residual, horizon, "mean_90")
    events = future_events(patterns, cutoff, horizon)
    additions = defaultdict(int)
    for event in events:
        additions[event.day] += event.amount
    return [p.outflow + additions[p.day] for p in base], patterns, events


def combine(history, incoming, outgoing):
    if len(incoming) != len(outgoing):
        raise DataError("Forecast horizons must match")
    balance = history[-1].balance
    result = []
    for i, (inc, out) in enumerate(zip(incoming, outgoing), 1):
        if not isinstance(inc, int) or not isinstance(out, int) or min(inc, out) < 0:
            raise DataError("Forecast flows must be nonnegative integer minor units")
        balance += inc - out
        result.append(Prediction(history[-1].day + timedelta(days=i), inc, out, balance))
    return result


def cautious_forecast(company, history, expected):
    """Sensitivity scenario, deliberately not a confidence/coverage claim.

    Frequent operating receipts -10%; sparse/mixed and other receipts -25%;
    defer the reduction-adjusted uncertain flow by seven days. Expense floor is
    the upper historical weekly quartile, alongside unchanged expected events.
    """
    channels, _ = receipt_channels(company, history)
    other = round_fraction(Fraction(sum(channels.get("other", {}).values()), len(history)))
    frequent = receipt_type(history) == "frequent"
    weekly = sorted(sum(d.outflow for d in history[i : i + 7]) for i in range(0, 84, 7))
    expense_floor = round_fraction(Fraction(weekly[8], 7))
    uncertain = []
    incoming = []
    for p in expected:
        other_component = min(other, p.inflow)
        operating = p.inflow - other_component
        stable = round_fraction(Fraction(operating * 9, 10)) if frequent else 0
        unstable = other_component + (0 if frequent else operating)
        uncertain.append(round_fraction(Fraction(unstable * 3, 4)))
        incoming.append(stable)
    for i in range(len(incoming)):
        if i >= 7:
            incoming[i] += uncertain[i - 7]
    outgoing = [max(p.outflow, expense_floor) for p in expected]
    return combine(history, incoming, outgoing), {
        "kind": "sensitivity_not_probability",
        "operating_frequent_factor": "9/10",
        "uncertain_factor": "3/4",
        "uncertain_delay_days": 7,
        "expense_floor_cents": expense_floor,
        "as_of": history[-1].day.isoformat(),
        "limitation": "Past quantile is not a future obligation; no calibrated coverage.",
    }
