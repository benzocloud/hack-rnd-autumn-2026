"""Validate a normalized snapshot and run the fixed deterministic baseline."""

from datetime import date, timedelta
from uuid import UUID

from cashflow_baseline.data import Daily
from cashflow_baseline.models import predict

MODEL_NAME = "mean_90"
MODEL_VERSION = "mean_90_v1"
PREPROCESSING_VERSION = "daily_snapshot_v1"
MIN_INT64 = -(2**63)
MAX_INT64 = 2**63 - 1


class InvalidSnapshot(ValueError):
    """The caller supplied an invalid or ambiguous snapshot."""


class UnsupportedContext(ValueError):
    """The requested model or calendar cannot be computed faithfully."""


def _uuid(value):
    try:
        parsed = UUID(value)
    except (ValueError, AttributeError) as exc:
        raise InvalidSnapshot("Snapshot, company and account require UUIDs") from exc
    if parsed.int == 0 or str(parsed) != value:
        raise InvalidSnapshot("UUIDs must be nonzero canonical lowercase strings")


def _date(value):
    try:
        parsed = date.fromisoformat(value)
    except (ValueError, TypeError) as exc:
        raise InvalidSnapshot("Dates require YYYY-MM-DD") from exc
    if parsed.isoformat() != value:
        raise InvalidSnapshot("Dates require YYYY-MM-DD")
    return parsed


def validate_request(request, pb, allow_reconstructed=False):
    """Validate covered history without forecasting an unrequested financial path."""
    if not request.HasField("snapshot") or not request.HasField("model"):
        raise InvalidSnapshot("Snapshot and explicit model context are required")
    context = request.model
    if (context.model_name, context.model_version, context.preprocessing_version) != (
        MODEL_NAME,
        MODEL_VERSION,
        PREPROCESSING_VERSION,
    ):
        raise UnsupportedContext("Requested model/preprocessing version is unavailable")
    s = request.snapshot
    for value in (s.snapshot_id, s.company_id, s.account_id):
        _uuid(value)
    if s.data_version == 0 or s.currency != "RUB":
        raise InvalidSnapshot("A positive data version and RUB currency are required")
    if s.known_payments:
        raise UnsupportedContext("Known payments require an agreed no-double-counting policy")
    if not 1 <= request.horizon_days <= 30 or len(s.history) != 90:
        raise InvalidSnapshot("Require 90 history days and a horizon of 1..30")
    cutoff = _date(s.cutoff_date)
    if not s.HasField("initial_balance_minor"):
        raise InvalidSnapshot("Observed initial EOD balance is required")
    try:
        start = cutoff - timedelta(days=89)
        cutoff + timedelta(days=request.horizon_days)
    except OverflowError as exc:
        raise InvalidSnapshot("Date range is outside the supported calendar") from exc
    history, rounding = [], False
    for index, raw in enumerate(s.history):
        day = _date(raw.date)
        if day != start + timedelta(days=index):
            raise InvalidSnapshot("History must be consecutive and end exactly at cutoff")
        if not all(
            raw.HasField(f) for f in ("inflow_minor", "outflow_minor", "closing_balance_minor")
        ):
            raise InvalidSnapshot("Every history amount must be explicitly present")
        if raw.inflow_minor < 0 or raw.outflow_minor < 0:
            raise InvalidSnapshot("History flows must be nonnegative")
        sources = {pb.DAY_SOURCE_OBSERVED, pb.DAY_SOURCE_VERIFIED_NO_OPERATIONS}
        if allow_reconstructed:
            sources.add(pb.DAY_SOURCE_RECONSTRUCTED_COVERED)
        if raw.source not in sources:
            raise InvalidSnapshot("Unknown history coverage cannot become a zero-flow day")
        observed = raw.source == pb.DAY_SOURCE_OBSERVED
        if raw.source == pb.DAY_SOURCE_VERIFIED_NO_OPERATIONS and (
            raw.inflow_minor or raw.outflow_minor
        ):
            raise InvalidSnapshot("Verified no-operations day must have zero flows")
        delta = 0
        if history:
            delta = raw.closing_balance_minor - (
                history[-1].balance + raw.inflow_minor - raw.outflow_minor
            )
            if abs(delta) > (1 if observed else 0):
                raise InvalidSnapshot("History contains an unexplained balance discontinuity")
        rounding |= delta != 0
        history.append(
            Daily(
                day,
                raw.inflow_minor,
                raw.outflow_minor,
                raw.closing_balance_minor,
                observed,
                False,
                0,
                delta,
            )
        )
    if not history[-1].observed_balance or history[-1].balance != s.initial_balance_minor:
        raise InvalidSnapshot("Cutoff EOD must be observed and equal the initial balance")
    return history, rounding


def forecast(request, pb):
    """No I/O, training, future truth or mutable model selection."""
    history, rounding = validate_request(request, pb)
    s, context = request.snapshot, request.model
    predictions = predict(history, request.horizon_days, MODEL_NAME)
    if any(not MIN_INT64 <= p.balance <= MAX_INT64 for p in predictions):
        raise InvalidSnapshot("Forecast balance exceeds signed int64 capacity")
    result = pb.PredictResponse(
        snapshot_id=s.snapshot_id,
        company_id=s.company_id,
        account_id=s.account_id,
        data_version=s.data_version,
        currency=s.currency,
        cutoff_date=s.cutoff_date,
        start_date=predictions[0].day.isoformat(),
        model=context,
        history_only_includes_recurring_flows=True,
        warnings=["HISTORY_ONLY_NO_CALENDAR", "UNCALIBRATED_NO_PROBABILITY"],
    )
    if rounding:
        result.warnings.append("OBSERVED_RECONCILIATION_WITHIN_ONE_MINOR_UNIT")
    for p in predictions:
        result.days.add(
            date=p.day.isoformat(),
            inflow_minor=p.inflow,
            outflow_minor=p.outflow,
            reference_balance_minor=p.balance,
        )
        if p.balance < 0 and not result.HasField("first_negative_balance_date"):
            result.first_negative_balance_date = p.day.isoformat()
    return result
