"""Versioned, snapshot-only derived labels. Never changes financial arithmetic."""

import hashlib
from collections import defaultdict
from datetime import timedelta

from .csv_calendar import CSV_CALENDAR_PREPROCESSING, CSV_CALENDAR_VERSION, csv_calendar_forecast
from .csv_forecast import CSV_PREPROCESSING, CSV_VERSION, csv_forecast
from .financial import checked
from .inference import InvalidSnapshot, UnsupportedContext, _date

ANALYSIS_VERSION = "text_categories_monthly_v1"
CSV_ANALYSIS_VERSION = "mean_90_csv_analysis_v2"
CSV_CALENDAR_ANALYSIS_VERSION = "mean_90_csv_calendar_analysis_v2"
PROFILES = {
    CSV_ANALYSIS_VERSION: ("csv_observed_operations_analysis_v2", CSV_VERSION, CSV_PREPROCESSING),
    CSV_CALENDAR_ANALYSIS_VERSION: (
        "csv_observed_operations_calendar_analysis_v2",
        CSV_CALENDAR_VERSION,
        CSV_CALENDAR_PREPROCESSING,
    ),
}


def analysis_forecast(request, pb):
    # Imports are pure helpers; no dataset loader, daily reconstruction or training.
    from cashflow_baseline.categories import categorize, normalize
    from cashflow_baseline.data import Operation
    from cashflow_baseline.recurring import EXCLUDED, fit_monthly

    profile = PROFILES.get(request.model.model_version)
    if profile is None or (
        request.model.model_name != "mean_90"
        or request.model.preprocessing_version != profile[0]
        or request.model.configuration_id != request.model.model_version
    ):
        raise UnsupportedContext("Unsupported runtime analysis context")
    for op in request.operations:
        manual = op.category_source == "user"
        if (
            op.category_code != "unknown"
            or op.category_rule_id
            or (manual and not op.HasField("category_id"))
            or (not manual and (op.category_source != "unknown" or op.HasField("category_id")))
        ):
            raise InvalidSnapshot("Analysis requires unchanged unknown or manual input labels")
    base = pb.PredictRequest()
    base.CopyFrom(request)
    base.model.model_version, base.model.configuration_id = profile[1], profile[1]
    base.model.preprocessing_version = profile[2]
    compute = csv_calendar_forecast if profile[1] == CSV_CALENDAR_VERSION else csv_forecast
    result = compute(base, pb)  # All original CSV/calendar guards run before analysis.
    result.model.CopyFrom(request.model)
    cutoff = _date(request.snapshot.cutoff_date)
    start = cutoff - timedelta(days=89)
    result.analysis_context.CopyFrom(
        pb.AnalysisContext(
            analysis_version=ANALYSIS_VERSION,
            window_from=start.isoformat(),
            window_through=cutoff.isoformat(),
        )
    )
    groups = defaultdict(list)
    versions = {}
    for factual, classified in zip(request.operations, result.classified_operations):
        op = Operation(
            request.snapshot.company_id,
            factual.operation_id,
            _date(factual.date),
            factual.inflow_minor or factual.outflow_minor,
            factual.inflow_minor > 0,
            factual.counterparty_inn,
            factual.counterparty_name,
            factual.payment_purpose,
            0,
        )
        versions[op.operation_id] = factual.version
        if factual.category_source == "user":
            continue  # A manual UUID/code is never replaced or inferred from text.
        category = categorize(op)
        classified.category_code = category.label
        classified.category_source = "unknown" if category.label == "unknown" else "rules"
        classified.category_rule_id = category.rule
        party = op.inn or normalize(op.name)
        if party and category.label not in EXCLUDED:
            groups[(op.incoming, party, category.label)].append(op)
    total_refs = 0
    for (_, _, category), operations in sorted(groups.items()):
        pattern = fit_monthly(
            sorted(operations, key=lambda op: (op.day, op.operation_id)), category, cutoff
        )
        if pattern is None:
            continue
        ids = sorted(pattern.matched_ids)
        total_refs += len(ids)
        if len(result.recurring_hypotheses) >= 1000 or total_refs > 20000:
            raise InvalidSnapshot("Runtime analysis evidence exceeds supported limits")
        identity = "\n".join([request.snapshot.snapshot_id, "monthly_fit", ANALYSIS_VERSION, *ids])
        hypothesis = result.recurring_hypotheses.add(
            pattern_id="monthly_" + hashlib.sha256(identity.encode()).hexdigest(),
            rule_id="monthly_fit",
            direction=pb.RECURRING_DIRECTION_INFLOW
            if pattern.incoming
            else pb.RECURRING_DIRECTION_OUTFLOW,
            weekend_shift={
                "none": pb.RECURRING_WEEKEND_SHIFT_NONE,
                "previous": pb.RECURRING_WEEKEND_SHIFT_PREVIOUS,
                "next": pb.RECURRING_WEEKEND_SHIFT_NEXT,
            }[pattern.weekend_shift],
            expected_amount_minor=checked(pattern.amount),
            confirmations=pattern.confirmations,
            expected_periods=pattern.expected_periods,
            first_evidence_date=pattern.first_evidence.isoformat(),
            last_evidence_date=pattern.last_evidence.isoformat(),
            category_code=category,
            rule_version=ANALYSIS_VERSION,
            status=pb.RECURRING_HYPOTHESIS_STATUS_HYPOTHESIS,
        )
        if pattern.anchor == 0:
            hypothesis.month_end.SetInParent()
        else:
            hypothesis.day_of_month = pattern.anchor
        for identifier in ids:
            hypothesis.historical_operations.add(
                operation_id=identifier, version=versions[identifier]
            )
    if result.ByteSize() > 2 * 1024 * 1024:
        raise InvalidSnapshot("Runtime analysis exceeds RPC message limit")
    return result
