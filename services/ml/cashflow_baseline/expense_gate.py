"""One predeclared, past-only gate for the experimental expense calendar."""

from datetime import timedelta

from .balance_model import combine, expense_calendar
from .data import DataError, daily_as_of, history_at
from .models import predict


def new_gap_errors(initial, truth, predictions):
    if initial < 0:
        return {"fp": 0, "fn": 0}
    actual = any(d.balance < 0 for d in truth)
    predicted = any(d.balance < 0 for d in predictions)
    return {"fp": int(predicted and not actual), "fn": int(actual and not predicted)}


def gated_expenses(company, history, horizon=30, outgoing=None, cache=None):
    """Require >=5% lower past balance error, with no additional past FP/FN.

    Two nonoverlapping 30-day outcomes end at outer-30 and outer. Mandatory
    history quality follows the baseline; unusable additional folds fall back.
    The optional cache is local to one immutable experiment, never global.
    """
    if horizon != 30:
        raise DataError("The fixed expense gate requires a 30-day horizon")
    baseline = predict(history, horizon, "mean_90")
    cutoff = history[-1].day
    evidence = {"as_of": cutoff.isoformat(), "folds": [], "used_calendar": False}
    if outgoing is None:
        outgoing = expense_calendar(company, history, horizon)[0]
    if outgoing == [p.outflow for p in baseline]:
        evidence["reason"] = "calendar_identical"
        return baseline, evidence
    known = daily_as_of(company, cutoff)
    scores = {"baseline": 0, "calendar": 0}
    gaps = {name: {"fp": 0, "fn": 0} for name in scores}
    local = {} if cache is None else cache
    for lag in (60, 30):
        inner = cutoff - timedelta(days=lag)
        past, reason = history_at(company, inner, 90)
        if reason:
            evidence["reason"] = "inner_" + reason
            return baseline, evidence
        truth = [known.get(inner + timedelta(days=i)) for i in range(1, 31)]
        if any(d is None or d.balance is None or d.gap or d.duplicates for d in truth):
            evidence["reason"] = "inner_target_quality"
            return baseline, evidence
        control = predict(past, 30, "mean_90")
        key = company.company_id, inner
        if key not in local:
            local[key] = expense_calendar(company, past, 30)[0]
        candidate = combine(past, [p.inflow for p in control], local[key])
        fold = {"cutoff": inner.isoformat(), "target_end": truth[-1].day.isoformat()}
        for name, predictions in (("baseline", control), ("calendar", candidate)):
            error = sum(abs(a.balance - p.balance) for a, p in zip(truth, predictions))
            scores[name] += error
            current = new_gap_errors(past[-1].balance, truth, predictions)
            for kind in current:
                gaps[name][kind] += current[kind]
            fold[name + "_absolute_balance_error_cents"] = error
            fold[name + "_gap_errors"] = current
        evidence["folds"].append(fold)
    improves = scores["baseline"] > 0 and scores["calendar"] * 100 <= scores["baseline"] * 95
    safer = all(gaps["calendar"][k] <= gaps["baseline"][k] for k in ("fp", "fn"))
    evidence.update(absolute_balance_errors_cents=scores, past_gap_errors=gaps)
    if improves and safer:
        evidence.update(used_calendar=True, reason="past_balance_improvement")
        return combine(history, [p.inflow for p in baseline], outgoing), evidence
    evidence["reason"] = "past_gap_errors_worse" if not safer else "past_balance_gain_below_5pct"
    return baseline, evidence
