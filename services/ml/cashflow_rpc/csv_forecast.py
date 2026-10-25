"""Limited CSV projection; unknown dates never become observed zero-flow facts."""

from datetime import timedelta

from .financial import checked, validate_category
from .inference import MODEL_NAME, InvalidSnapshot, UnsupportedContext, _date, _uuid

CSV_VERSION = "mean_90_csv_v1"
CSV_PREPROCESSING = "csv_observed_operations_v1"
QUALITY_VERSION = "csv_history_quality_v1"
LIMITATIONS = (
    "csv_history_completeness_unverified",
    "unobserved_days_unknown",
    "observed_operations_mean_per_calendar_day",
    "historical_calendar_unavailable",
    "historical_availability_unverified",
    "not_calibrated",
)


def csv_forecast(request, pb):
    model = request.model
    if (
        model.model_name,
        model.model_version,
        model.preprocessing_version,
        model.configuration_id,
    ) != (MODEL_NAME, CSV_VERSION, CSV_PREPROCESSING, CSV_VERSION):
        raise UnsupportedContext("Requested CSV model context is unavailable")
    if model.HasField("random_seed") or model.HasField("trajectory_count"):
        raise UnsupportedContext("CSV profile does not estimate trajectories")
    s, quality, calendar, cutoff, totals = validate_csv_history(request, pb, LIMITATIONS)
    validate_unavailable_calendar(s, calendar, pb)
    # This is an estimate from available operations divided by calendar span.
    # No unknown historical day or balance is stored as a known zero.
    incoming = checked((sum(v[0] for v in totals.values()) + 45) // 90)
    outgoing = checked((sum(v[1] for v in totals.values()) + 45) // 90)
    result = pb.PredictResponse(
        snapshot_id=s.snapshot_id,
        company_id=s.company_id,
        account_id=s.account_id,
        data_version=s.data_version,
        model=model,
        currency=s.currency,
        cutoff_date=s.cutoff_date,
        start_date=(cutoff + timedelta(days=1)).isoformat(),
        history_only_includes_recurring_flows=True,
        calendar_snapshot_id=calendar.snapshot_id,
        calendar_data_version=calendar.data_version,
        classified_operations=request.operations,
        availability_limitations=LIMITATIONS,
        csv_history=quality,
        warnings=[
            "CSV_HISTORY_COMPLETENESS_UNVERIFIED",
            "HISTORY_ONLY_NO_CALENDAR",
            "UNCALIBRATED_NO_PROBABILITY",
        ],
        assumptions=[
            "Forecast uses available imported inflows/outflows divided by 90 calendar days.",
            "Missing operations can understate either flow; unknown dates are not verified zero-activity days.",
            "The projection excludes unknown future obligations and has no calibrated probability or interval.",
        ],
    )
    result.uncertainty.method = "unavailable_incomplete_history"
    result.uncertainty.gap_probability_unavailable_reason = "not_calibrated"
    result.gap_probability.unavailable_reason = "not_calibrated"
    balance = s.initial_balance_minor
    for n in range(30):
        balance = checked(balance + incoming - outgoing)
        point = result.days.add(
            date=(cutoff + timedelta(days=n + 1)).isoformat(),
            inflow_minor=incoming,
            outflow_minor=outgoing,
            reference_balance_minor=balance,
        )
        point.negative_balance_probability.unavailable_reason = "not_calibrated"
        if balance < 0 and not result.HasField("first_negative_balance_date"):
            result.first_negative_balance_date = point.date
    return result


def validate_csv_history(request, pb, limitations):
    """Validate original supplied CSV facts/EOD without filling unknown days."""
    if not all(request.HasField(field) for field in ("snapshot", "calendar", "csv_history")):
        raise InvalidSnapshot("CSV snapshot, calendar exclusion and quality context are required")
    s, quality, calendar = request.snapshot, request.csv_history, request.calendar
    for identifier in (s.snapshot_id, s.company_id, s.account_id):
        _uuid(identifier)
    cutoff = _date(s.cutoff_date)
    try:
        start = cutoff - timedelta(days=89)
        cutoff + timedelta(days=30)
    except OverflowError as exc:
        raise InvalidSnapshot("CSV date range exceeds supported calendar") from exc
    if (
        request.horizon_days != 30
        or s.currency != "RUB"
        or s.data_version < 1
        or not s.HasField("initial_balance_minor")
        or s.known_payments
        or len(s.history) != 90
        or quality.rule_version != QUALITY_VERSION
        or quality.from_date != start.isoformat()
        or quality.through_date != s.cutoff_date
        or quality.data_version != s.data_version
        or _date(quality.first_operation_date) > start
        or tuple(request.availability_limitations) != limitations
    ):
        raise InvalidSnapshot("CSV input does not match the limited 90-day quality profile")
    checked(s.initial_balance_minor)
    if not 1 <= len(request.operations) <= 20000:
        raise InvalidSnapshot("CSV operation count is outside the supported limit")
    totals, seen = {}, set()
    for op in request.operations:
        _uuid(op.operation_id)
        validate_category(op, s.company_id)
        day = _date(op.date)
        if (
            not start <= day <= cutoff
            or op.operation_id in seen
            or op.version < 1
            or op.factual_source_kind not in ("imported", "manual")
            or (
                op.factual_source_kind == "imported"
                and (not op.source_namespace or not op.external_operation_id)
            )
            or (
                op.factual_source_kind == "manual"
                and (op.source_namespace or op.external_operation_id)
            )
            or not op.category_code
            or not op.category_source
            or op.category_rule_id
            or not (
                (op.inflow_minor > 0 and op.outflow_minor == 0)
                or (op.outflow_minor > 0 and op.inflow_minor == 0)
            )
        ):
            raise InvalidSnapshot("CSV facts have invalid identity, provenance or amounts")
        seen.add(op.operation_id)
        values = totals.setdefault(op.date, [0, 0])
        values[0] = checked(values[0] + op.inflow_minor)
        values[1] = checked(values[1] + op.outflow_minor)
    previous_balance = None
    net_since_balance = 0
    for index, raw in enumerate(s.history):
        expected_date = (start + timedelta(days=index)).isoformat()
        flows = totals.get(expected_date)
        has_flows = raw.HasField("inflow_minor") and raw.HasField("outflow_minor")
        has_balance = raw.HasField("closing_balance_minor")
        if (
            raw.date != expected_date
            or raw.quality_codes
            or raw.HasField("inflow_minor") != raw.HasField("outflow_minor")
            or has_flows != (flows is not None)
            or (has_flows and flows != [raw.inflow_minor, raw.outflow_minor])
            or raw.source
            != (pb.DAY_SOURCE_PARTIAL if has_flows or has_balance else pb.DAY_SOURCE_UNAVAILABLE)
        ):
            raise InvalidSnapshot("CSV history must preserve observed flows and unknown dates")
        if flows:
            net_since_balance += flows[0] - flows[1]
        if has_balance:
            checked(raw.closing_balance_minor)
            if (
                previous_balance is not None
                and previous_balance + net_since_balance != raw.closing_balance_minor
            ):
                raise InvalidSnapshot("CSV EOD observations do not reconcile with supplied facts")
            previous_balance = raw.closing_balance_minor
            net_since_balance = 0
    if (
        not s.history[-1].HasField("closing_balance_minor")
        or s.history[-1].closing_balance_minor != s.initial_balance_minor
    ):
        raise InvalidSnapshot("CSV cutoff requires an accepted EOD observation")
    return s, quality, calendar, cutoff, totals


def validate_unavailable_calendar(s, calendar, pb):
    if (
        calendar.snapshot_id != s.snapshot_id
        or calendar.data_version != s.data_version
        or calendar.knowledge_mode != pb.CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE
        or calendar.as_of_business_date
        or calendar.groups
        or calendar.payments
        or calendar.HasField("scenario_context")
        or calendar.no_post_cutoff_movements
    ):
        raise InvalidSnapshot("CSV profile requires an explicitly unavailable calendar")
