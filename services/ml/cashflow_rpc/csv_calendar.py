"""Explicit CSV + CURRENT calendar; sparse history never becomes coverage proof."""

import hashlib
from collections import defaultdict
from datetime import timedelta

from .csv_forecast import LIMITATIONS, validate_csv_history
from .financial import allocate, calendar_contributions, checked
from .inference import MODEL_NAME, InvalidSnapshot, UnsupportedContext, _date

CSV_CALENDAR_VERSION = "mean_90_csv_calendar_v1"
CSV_CALENDAR_PREPROCESSING = "csv_observed_operations_calendar_v1"
CSV_CALENDAR_LIMITATIONS = tuple(
    item for item in LIMITATIONS if item != "historical_calendar_unavailable"
) + ("current_calendar_only",)


def csv_calendar_forecast(request, pb):
    model = request.model
    if (
        model.model_name,
        model.model_version,
        model.preprocessing_version,
        model.configuration_id,
    ) != (MODEL_NAME, CSV_CALENDAR_VERSION, CSV_CALENDAR_PREPROCESSING, CSV_CALENDAR_VERSION):
        raise UnsupportedContext("Requested CSV calendar model context is unavailable")
    if model.HasField("random_seed") or model.HasField("trajectory_count"):
        raise UnsupportedContext("CSV calendar profile does not estimate trajectories")
    s, quality, calendar, cutoff, _ = validate_csv_history(request, pb, CSV_CALENDAR_LIMITATIONS)
    if (
        calendar.snapshot_id != s.snapshot_id
        or calendar.data_version != s.data_version
        or calendar.knowledge_mode != pb.CALENDAR_KNOWLEDGE_MODE_CURRENT
        or calendar.HasField("scenario_context")
        or len(calendar.groups) > 1000
        or len(calendar.payments) > 1000
    ):
        raise InvalidSnapshot("CSV calendar requires one bounded CURRENT frozen snapshot")
    available_on = _date(calendar.as_of_business_date)
    if not cutoff <= available_on <= cutoff + timedelta(days=1) or (
        available_on > cutoff and not calendar.no_post_cutoff_movements
    ):
        raise InvalidSnapshot(
            "CURRENT CSV calendar requires current/preceding EOD and no later movements"
        )
    for payment in calendar.payments:
        if payment.effective_date != payment.original_due_date or payment.source not in (
            "manual",
            "imported",
        ):
            raise InvalidSnapshot("CSV calendar preserves original dates and payment provenance")
    operations = {op.operation_id: op for op in request.operations}
    covered, known, known_components = calendar_contributions(
        calendar, operations, s.company_id, cutoff, pb
    )
    totals = [defaultdict(int), defaultdict(int)]
    source_groups = defaultdict(list)
    for identifier, op in operations.items():
        if identifier in covered:
            continue
        key = (
            op.category_code,
            op.category_source,
            op.regularity_key,
            op.category_id,
            op.category_company_id,
        )
        totals[0][key] += op.inflow_minor
        totals[1][key] += op.outflow_minor
        source_groups[key].append(identifier)
    allocated = [allocate(values) for values in totals]
    result = pb.PredictResponse(
        snapshot_id=s.snapshot_id,
        company_id=s.company_id,
        account_id=s.account_id,
        data_version=s.data_version,
        model=model,
        currency=s.currency,
        cutoff_date=s.cutoff_date,
        start_date=(cutoff + timedelta(days=1)).isoformat(),
        history_only_includes_recurring_flows=False,
        calendar_snapshot_id=calendar.snapshot_id,
        calendar_data_version=calendar.data_version,
        replaced_historical_operation_ids=sorted(covered),
        classified_operations=request.operations,
        availability_limitations=CSV_CALENDAR_LIMITATIONS,
        csv_history=quality,
        warnings=[
            "CSV_HISTORY_COMPLETENESS_UNVERIFIED",
            "CURRENT_CALENDAR_INCLUDED",
            "UNCALIBRATED_NO_PROBABILITY",
        ],
        assumptions=[
            "Residual available CSV operations are divided by 90 calendar days after explicit complete replacement mappings.",
            "Unknown historical dates remain unknown; CURRENT remaining obligations are added once on their original dates.",
            "No calibrated probability, trajectory or interval is available for this incomplete-history context.",
        ],
    )
    evidence_ids = {
        key: "history:" + hashlib.sha256(repr(key).encode()).hexdigest() for key in source_groups
    }
    for key in sorted(source_groups):
        result.history_evidence.add(
            evidence_id=evidence_ids[key],
            historical_operation_ids=sorted(source_groups[key]),
            category_source=key[1],
        )
    balance = s.initial_balance_minor
    for step in range(30):
        day = (cutoff + timedelta(days=step + 1)).isoformat()
        incoming = checked(sum(allocated[0].values()) + known[step][0])
        outgoing = checked(sum(allocated[1].values()) + known[step][1])
        balance = checked(balance + incoming - outgoing)
        point = result.days.add(
            date=day,
            inflow_minor=incoming,
            outflow_minor=outgoing,
            reference_balance_minor=balance,
        )
        point.negative_balance_probability.unavailable_reason = "not_calibrated"
        if balance < 0 and not result.HasField("first_negative_balance_date"):
            result.first_negative_balance_date = day
        for key in sorted(set(allocated[0]) | set(allocated[1])):
            incoming, outgoing = allocated[0].get(key, 0), allocated[1].get(key, 0)
            if incoming or outgoing:
                result.components.add(
                    component_id=f"residual:{step}:{evidence_ids[key]}",
                    date=day,
                    category_code=key[0],
                    inflow_minor=incoming,
                    outflow_minor=outgoing,
                    kind="mean_90_uncovered",
                    source_ids=[evidence_ids[key]],
                    regularity_key=key[2],
                    category_id=key[3] if key[3] else None,
                    category_company_id=key[4] if key[4] else None,
                )
    result.components.extend(known_components)
    result.uncertainty.method = "unavailable_incomplete_history"
    result.uncertainty.gap_probability_unavailable_reason = "not_calibrated"
    result.gap_probability.unavailable_reason = "not_calibrated"
    return result
