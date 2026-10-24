"""Retrospective causes of false warnings; never used to create forecasts."""

from collections import Counter, defaultdict
from datetime import timedelta
from fractions import Fraction

from .categories import categorize, normalize
from .data import DataError
from .evaluation import first_gap
from .recurring import future_events


def is_false_warning(record):
    return (
        record.initial_balance >= 0
        and first_gap(record.predictions) is not None
        and first_gap(record.truth) is None
    )


def event_diagnostics(company, record, patterns):
    """Nearest observed group within seven days is a diagnostic, not a liability.

    Future operations are read here ONLY after the forecast was fixed. Missing
    matches are not assertions of nonpayment; the horizon can censor matches.
    """
    operations = {op.operation_id: op for op in company.operations}
    seen = set()
    group_keys = {}
    for index, pattern in enumerate(patterns):
        if seen & pattern.matched_ids:
            raise DataError("Historical expense double counting")
        seen.update(pattern.matched_ids)
        sample = operations[next(iter(pattern.matched_ids))]
        group_keys[index] = sample.inn or normalize(sample.name)
    result, used = [], set()
    end = record.truth[-1].day
    for event in future_events(patterns, record.cutoff, len(record.truth)):
        pattern = patterns[event.pattern_index]
        candidates = defaultdict(list)
        for op in company.operations:
            if (
                not op.incoming
                and record.cutoff < op.day <= end
                and (op.inn or normalize(op.name)) == group_keys[event.pattern_index]
                and categorize(op).label == pattern.category
                and abs((op.day - event.day).days) <= 7
            ):
                candidates[op.day].append(op)
        choices = [
            day for day, ops in candidates.items() if all(op.operation_id not in used for op in ops)
        ]
        observed = (
            min(choices, key=lambda day: (abs((day - event.day).days), day)) if choices else None
        )
        amount = sum(op.amount for op in candidates[observed]) if observed else None
        if observed:
            used.update(op.operation_id for op in candidates[observed])
        result.append(
            {
                "category": pattern.category,
                "counterparty": pattern.counterparty,
                "predicted_date": event.day.isoformat(),
                "predicted_amount_cents": event.amount,
                "observed_nearby_date": observed.isoformat() if observed else None,
                "observed_nearby_amount_cents": amount,
                "observed_minus_predicted_days": (observed - event.day).days if observed else None,
                "predicted_minus_observed_amount_cents": event.amount - amount
                if amount is not None
                else None,
                "right_censored_match": observed is None and end - event.day < timedelta(days=7),
                "matching_is_diagnostic_not_ground_truth": True,
            }
        )
    return result


def warning_detail(record, baseline, company, patterns, status):
    warning = first_gap(record.predictions)
    horizon, step = len(record.truth), (warning - record.cutoff).days
    signals = {}
    for stream, orientation in (("inflow", -1), ("outflow", 1)):
        # Positive values raise actual balance relative to the too-low prediction.
        total = orientation * sum(
            getattr(p, stream) - getattr(a, stream)
            for a, p in zip(record.truth, record.predictions)
        )
        prefix = orientation * sum(
            getattr(p, stream) - getattr(a, stream)
            for a, p in zip(record.truth[:step], record.predictions[:step])
        )
        amount = Fraction(total * step, horizon)
        signals[stream + "_amount"] = amount
        signals[stream + "_timing"] = prefix - amount
    nonnegative = {key: max(Fraction(0), value) for key, value in signals.items()}
    actual = record.truth[step - 1]
    predicted = record.predictions[step - 1]
    actual_from_flows = record.initial_balance + sum(
        d.inflow - d.outflow for d in record.truth[:step]
    )
    reconciliation = actual.balance - actual_from_flows
    if actual.balance - predicted.balance != sum(signals.values()) + reconciliation:
        raise DataError("Warning decomposition does not reconcile")
    own_in, own_out, exceptional_in = 0, 0, 0
    for op in company.operations:
        if record.cutoff < op.day <= warning:
            label = categorize(op).label
            if label in {"own_transfer", "own_transfer_candidate"}:
                if op.incoming:
                    own_in += op.amount
                else:
                    own_out += op.amount
            if op.incoming and label in {
                "loan_receipt",
                "owner_financing",
                "owner_capital",
                "grant_receipt",
                "insurance_receipt",
            }:
                exceptional_in += op.amount
    events = event_diagnostics(company, record, patterns)
    before = [e for e in events if e["predicted_date"] <= warning.isoformat()]
    return {
        "company_id": record.company,
        "cutoff": record.cutoff.isoformat(),
        "split": record.split,
        "status": status,
        "method": record.method,
        "warning_date": warning.isoformat(),
        "actual_balance_cents": actual.balance,
        "predicted_balance_cents": predicted.balance,
        "calendar_balance_delta_cents": baseline.predictions[step - 1].balance - predicted.balance,
        "dominant_component": max(nonnegative, key=nonnegative.get),
        "signed_components_currency": {key: float(value / 100) for key, value in signals.items()},
        "inflow_underestimate_cents": sum(
            d.inflow - p.inflow for d, p in zip(record.truth[:step], record.predictions[:step])
        ),
        "outflow_overestimate_cents": sum(
            p.outflow - d.outflow for d, p in zip(record.truth[:step], record.predictions[:step])
        ),
        "own_transfer_in_cents_diagnostic_only": own_in,
        "own_transfer_out_cents_diagnostic_only": own_out,
        "exceptional_in_cents_diagnostic_only": exceptional_in,
        "actual_warning_day_observed": actual.observed_balance,
        "one_cent_reconciliation_cents": reconciliation,
        "historical_double_counted_operations": 0,
        "scheduled_events_before_warning": len(before),
        "events_observed_later": sum((e["observed_minus_predicted_days"] or 0) > 0 for e in before),
        "events_amount_overestimated": sum(
            (e["predicted_minus_observed_amount_cents"] or 0) > 0 for e in before
        ),
        "unmatched_uncensored_events": sum(
            e["observed_nearby_date"] is None and not e["right_censored_match"] for e in before
        ),
        "events": events,
        "limitation": "Arithmetic explanation, not a causal proof or a confirmed future liability.",
    }


def classify_warnings(baselines, calendars, companies, patterns):
    old = {(r.company, r.cutoff): r for r in baselines}
    new = {(r.company, r.cutoff): r for r in calendars}
    rows, summaries = [], {}
    for split in ("development", "validation", "evaluation"):
        a = {key for key, r in old.items() if r.split == split and is_false_warning(r)}
        b = {key for key, r in new.items() if r.split == split and is_false_warning(r)}
        summaries[split] = {"common": len(a & b), "added": len(b - a), "removed": len(a - b)}
        for key in sorted(a | b):
            status = "common" if key in a & b else "added" if key in b else "removed"
            r = old[key] if status == "removed" else new[key]
            rows.append(warning_detail(r, old[key], companies[r.company], patterns[key], status))
        current = [r for r in rows if r["split"] == split and r["status"] != "removed"]
        added = [r for r in current if r["status"] == "added"]
        summaries[split].update(
            {
                "dominant_components_all_calendar_fp": dict(
                    Counter(r["dominant_component"] for r in current)
                ),
                "dominant_components_added_fp": dict(
                    Counter(r["dominant_component"] for r in added)
                ),
                "windows_with_later_expense_matches": sum(
                    r["events_observed_later"] > 0 for r in current
                ),
                "windows_with_overestimated_matched_expense": sum(
                    r["events_amount_overestimated"] > 0 for r in current
                ),
                "windows_with_unmatched_events": sum(
                    r["unmatched_uncensored_events"] > 0 for r in current
                ),
                "windows_with_own_inflow_before_warning": sum(
                    r["own_transfer_in_cents_diagnostic_only"] > 0 for r in current
                ),
                "windows_with_exceptional_inflow_before_warning": sum(
                    r["exceptional_in_cents_diagnostic_only"] > 0 for r in current
                ),
                "warnings_on_inferred_actual_balance": sum(
                    not r["actual_warning_day_observed"] for r in current
                ),
                "historical_double_counted_operations": sum(
                    r["historical_double_counted_operations"] for r in current
                ),
            }
        )
    return rows, summaries
