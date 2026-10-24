"""Small, fixed inflow family; every historical receipt remains in its sample."""

from collections import defaultdict
from datetime import timedelta
from fractions import Fraction

from .categories import categorize
from .data import DataError, daily_as_of, history_at
from .models import Prediction, predict, rounded_mean

INFLOW_VARIANTS = (
    "weekday_4w",
    "blend_4w_8w",
    "mean_28",
    "robust_8w",
    "robust_trend_8w",
    "robust_12w_25",
    "robust_12w_50",
    "robust_12w_75",
)
SELECTED_VARIANT = "robust_12w_50"


def round_fraction(value):
    if value < 0:
        raise DataError("Incoming flow must be nonnegative")
    return (2 * value.numerator + value.denominator) // (2 * value.denominator)


def inflow_predict(history, horizon, variant):
    # Reuse the baseline's quality, continuity and observed-cutoff validation.
    reference = predict(history, horizon, "mean_90")
    if variant not in INFLOW_VARIANTS:
        raise DataError("Unknown inflow variant")
    recent = history[-56:]
    weekly = [sum(d.inflow for d in recent[i : i + 7]) for i in range(0, 56, 7)]
    average = Fraction(sum(weekly), 8)
    ordered = sorted(weekly)
    center = Fraction(ordered[3] + ordered[4], 2)
    robust = (average + center) / 2
    scale = robust / average if average else Fraction(1)
    if variant == "robust_trend_8w":
        previous, current = sum(weekly[:4]), sum(weekly[4:])
        ratio = Fraction(current, previous) if previous else Fraction(1)
        ratio = max(Fraction(1, 2), min(Fraction(3, 2), ratio))
        scale *= (1 + ratio) / 2
    result, balance = [], history[-1].balance
    for step, original in enumerate(reference, 1):
        day = history[-1].day + timedelta(days=step)
        eight = [d.inflow for d in recent if d.day.weekday() == day.weekday()]
        four = [d.inflow for d in recent[-28:] if d.day.weekday() == day.weekday()]
        if variant.startswith("robust_12w_"):
            twelve = history[-84:]
            totals = sorted(sum(d.inflow for d in twelve[i : i + 7]) for i in range(0, 84, 7))
            weight = Fraction(int(variant.rsplit("_", 1)[1]), 100)
            level = (1 - weight) * Fraction(sum(totals), 12) + weight * Fraction(
                totals[5] + totals[6], 2
            )
            incoming = round_fraction(level / 7)
        elif variant == "weekday_4w":
            incoming = rounded_mean(four)
        elif variant == "blend_4w_8w":
            incoming = round_fraction((Fraction(sum(four), 4) + Fraction(sum(eight), 8)) / 2)
        elif variant == "mean_28":
            incoming = rounded_mean([d.inflow for d in recent[-28:]])
        else:
            incoming = round_fraction(Fraction(sum(eight), 8) * scale)
        balance += incoming - original.outflow
        result.append(Prediction(day, incoming, original.outflow, balance))
    return result


def adaptive_predict(company, history, horizon):
    """Select a level on at most three fully past 28-day rolling checks.

    Every candidate uses the same usable inner folds. Without two folds, or
    without a 10% reduction of total absolute horizon error, keep mean_90.
    This selector never receives the outer forecast's actual future.
    """
    cutoff = history[-1].day
    known = daily_as_of(company, cutoff)
    candidates = ("mean_90", "mean_28", "robust_12w_50")
    errors = {name: 0 for name in candidates}
    folds = []
    for step in (3, 2, 1):
        inner_cutoff = cutoff - timedelta(days=28 * step)
        past, reason = history_at(company, inner_cutoff, 90)
        actual = [known.get(inner_cutoff + timedelta(days=i)) for i in range(1, 29)]
        if reason or any(d is None or d.gap or d.duplicates for d in actual):
            continue
        actual_total = sum(d.inflow for d in actual)
        for name in candidates:
            predictions = (
                predict(past, 28, "mean_90")
                if name == "mean_90"
                else inflow_predict(past, 28, name)
            )
            errors[name] += abs(sum(p.inflow for p in predictions) - actual_total)
        folds.append(inner_cutoff.isoformat())
    selected = min(candidates, key=lambda name: (errors[name], candidates.index(name)))
    if len(folds) < 2 or errors[selected] * 100 >= errors["mean_90"] * 90:
        selected = "mean_90"
    predictions = (
        predict(history, horizon, "mean_90")
        if selected == "mean_90"
        else inflow_predict(history, horizon, selected)
    )
    return predictions, {
        "selected": selected,
        "fold_cutoffs": folds,
        "absolute_horizon_errors_cents": errors,
        "selection_available_as_of": cutoff.isoformat(),
    }


def seasonal_predict(company, history, horizon):
    """Blend annual operating receipts with recent levels; retain other receipts.

    Same-weekday prior-year evidence uses a 364-day lag and +/- two weeks.
    Financing, own transfers and unknowns keep their full 90-day mean component;
    they are not interpreted as recurring annual sales or silently removed.
    """
    baseline = predict(history, horizon, "mean_90")
    cutoff = history[-1].day
    known = daily_as_of(company, cutoff)
    operating = defaultdict(int)
    revenue = {
        "acquiring_revenue",
        "cash_revenue",
        "customer_payment",
        "marketplace_revenue",
        "agency_revenue",
    }
    for operation in company.operations:
        if (
            operation.day <= cutoff
            and operation.incoming
            and categorize(operation).label in revenue
        ):
            operating[operation.day] += operation.amount
    recent_operating = sum(operating[d.day] for d in history[-90:])
    recent_other = sum(d.inflow for d in history[-90:]) - recent_operating
    prior = [d.day - timedelta(days=364) for d in history[-90:]]

    def usable(day):
        return day in known and not known[day].gap and not known[day].duplicates

    prior_usable = all(usable(day) for day in prior)
    prior_sum = sum(operating[day] for day in prior) if prior_usable else 0
    ratio = Fraction(recent_operating, prior_sum) if prior_sum else Fraction(1)
    growth = max(Fraction(1, 2), min(Fraction(2), ratio))
    result, available = [], 0
    balance = history[-1].balance
    for original in baseline:
        old_day = original.day - timedelta(days=364)
        sample = [old_day + timedelta(days=7 * offset) for offset in (-2, -1, 0, 1, 2)]
        if prior_usable and prior_sum > 0 and all(usable(day) for day in sample):
            seasonal = Fraction(sum(operating[day] for day in sample), 5) * growth
            level = (Fraction(recent_operating, 90) + seasonal) / 2 + Fraction(recent_other, 90)
            incoming = round_fraction(level)
            available += 1
        else:
            incoming = original.inflow
        balance += incoming - original.outflow
        result.append(Prediction(original.day, incoming, original.outflow, balance))
    return result, {
        "annual_evidence_days": available,
        "other_receipts_90d_cents": recent_other,
        "growth_numerator": growth.numerator,
        "growth_denominator": growth.denominator,
        "selection_available_as_of": cutoff.isoformat(),
    }
