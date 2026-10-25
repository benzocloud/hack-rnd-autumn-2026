"""Pure reference mapping from covered Core facts and accepted EOD; no CSV or DB I/O."""

from collections import defaultdict
from dataclasses import dataclass
from datetime import timedelta

from .financial import FINANCIAL_PREPROCESSING, FINANCIAL_VERSION, checked
from .inference import MODEL_NAME, InvalidSnapshot, _date, _uuid


@dataclass(frozen=True)
class Coverage:
    """Caller attestation, backed by an import receipt/snapshot in Core storage.

    Neither dates nor absence of rows prove coverage. This helper checks shape;
    Core must establish the truth and retain the referenced evidence.
    """

    start: str
    end: str
    reference: str
    kind: str


def prepare_financial_request(snapshot, operations, coverage, pb, *, calendar=None, limitations=()):
    """Build canonical protobuf; consumers port this mapping beside their adapter.

    snapshot contains identity/version/RUB/cutoff/accepted initial EOD only.
    Operations are immutable facts filtered to the last 90 days by Core. Only
    cutoff is marked observed; earlier balances are reconstructed backwards.
    An unavailable calendar is explicit, never an invented certified empty one.
    """
    for identifier in (snapshot.snapshot_id, snapshot.company_id, snapshot.account_id):
        _uuid(identifier)
    if (
        snapshot.currency != "RUB"
        or snapshot.data_version < 1
        or not snapshot.HasField("initial_balance_minor")
        or snapshot.history
        or snapshot.known_payments
    ):
        raise InvalidSnapshot(
            "Core input requires metadata and accepted EOD, without prebuilt history/calendar"
        )
    cutoff = _date(snapshot.cutoff_date)
    try:
        start = cutoff - timedelta(days=89)
        cutoff + timedelta(days=30)
    except OverflowError as exc:
        raise InvalidSnapshot("Core date range exceeds supported calendar") from exc
    if (
        not isinstance(coverage, Coverage)
        or not coverage.reference.strip()
        or coverage.kind not in ("certified", "dataset_fixture_assumption")
        or any(item.startswith("history_coverage_kind_") for item in limitations)
        or _date(coverage.start) > start
        or _date(coverage.end) != cutoff
    ):
        raise InvalidSnapshot("Core must attest coverage of all 90 days through cutoff")
    operations = list(operations)
    if len(operations) > 20000:
        raise InvalidSnapshot("Core input exceeds bounded operation count")
    totals = {}
    seen = set()
    for operation in operations:
        _uuid(operation.operation_id)
        day = _date(operation.date)
        incoming, outgoing = operation.inflow_minor, operation.outflow_minor
        if (
            not start <= day <= cutoff
            or operation.operation_id in seen
            or operation.version < 1
            or not ((incoming > 0 and outgoing == 0) or (outgoing > 0 and incoming == 0))
        ):
            raise InvalidSnapshot(
                "Core facts require unique IDs, covered dates, versions and one positive direction"
            )
        seen.add(operation.operation_id)
        values = totals.setdefault(day, [0, 0])
        values[0] = checked(values[0] + incoming)
        values[1] = checked(values[1] + outgoing)
    request = pb.PredictRequest(
        snapshot=snapshot,
        horizon_days=30,
        model=pb.ModelContext(
            model_name=MODEL_NAME,
            model_version=FINANCIAL_VERSION,
            preprocessing_version=FINANCIAL_PREPROCESSING,
        ),
        availability_limitations=list(limitations),
    )
    fixture_limitation = "dataset_coverage_assumed_not_bank_certified"
    if (
        coverage.kind == "dataset_fixture_assumption"
        and fixture_limitation not in request.availability_limitations
    ):
        request.availability_limitations.append(fixture_limitation)
    if coverage.kind == "certified" and fixture_limitation in request.availability_limitations:
        raise InvalidSnapshot("Certified coverage cannot carry the dataset fixture limitation")
    request.availability_limitations.append("history_coverage_kind_" + coverage.kind)
    request.operations.extend(sorted(operations, key=lambda op: (op.date, op.operation_id)))
    balance = snapshot.initial_balance_minor
    history = []
    for index in range(89, -1, -1):
        day = start + timedelta(days=index)
        incoming, outgoing = totals.get(day, (0, 0))
        history.append(
            pb.HistoricalDay(
                date=day.isoformat(),
                inflow_minor=incoming,
                outflow_minor=outgoing,
                closing_balance_minor=balance,
                source=pb.DAY_SOURCE_OBSERVED
                if day == cutoff
                else pb.DAY_SOURCE_RECONSTRUCTED_COVERED,
            )
        )
        if index:
            balance = checked(balance - incoming + outgoing)
    request.snapshot.history.extend(reversed(history))
    if calendar is not None:
        request.calendar.CopyFrom(calendar)
    else:
        request.calendar.CopyFrom(
            pb.FinancialCalendar(
                snapshot_id=snapshot.snapshot_id,
                data_version=snapshot.data_version,
                knowledge_mode=pb.CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE,
            )
        )
        if "historical_calendar_unavailable" not in request.availability_limitations:
            request.availability_limitations.append("historical_calendar_unavailable")
    return request


def verify_response(request, response):
    """Reference consumer checks before Core persists READY; independent of CSV/DB."""
    s, forecast = request.snapshot, response
    date_from_cutoff = _date(s.cutoff_date)
    start = date_from_cutoff + timedelta(days=1)
    if (
        forecast.snapshot_id,
        forecast.company_id,
        forecast.account_id,
        forecast.data_version,
        forecast.cutoff_date,
        forecast.currency,
        forecast.model,
        forecast.start_date,
    ) != (
        s.snapshot_id,
        s.company_id,
        s.account_id,
        s.data_version,
        s.cutoff_date,
        s.currency,
        request.model,
        start.isoformat(),
    ):
        raise InvalidSnapshot("Response identity/version/model mismatch")
    if len(forecast.days) != 30 or len(response.uncertainty.bands) != 30:
        raise InvalidSnapshot("Response requires 30 days and bands")
    if (response.calendar_snapshot_id, response.calendar_data_version) != (
        request.calendar.snapshot_id,
        request.calendar.data_version,
    ):
        raise InvalidSnapshot("Response calendar identity mismatch")
    if not set(request.availability_limitations) <= set(response.availability_limitations):
        raise InvalidSnapshot("Response discarded input availability limitations")
    if response.classified_operations != request.operations:
        raise InvalidSnapshot("Response changed caller-owned classified facts")
    expected_covered = {
        identifier
        for group in request.calendar.groups
        for identifier in group.historical_operation_ids
    }
    if (
        len(response.replaced_historical_operation_ids) != len(expected_covered)
        or set(response.replaced_historical_operation_ids) != expected_covered
    ):
        raise InvalidSnapshot("Response historical replacement mismatch")
    parts = defaultdict(lambda: [0, 0])
    evidence = {item.evidence_id for item in response.history_evidence}
    uncovered = {operation.operation_id for operation in request.operations} - expected_covered
    evidence_operations = []
    for item in response.history_evidence:
        if not item.evidence_id or not item.historical_operation_ids:
            raise InvalidSnapshot("Response has empty history evidence")
        evidence_operations.extend(item.historical_operation_ids)
    if (
        len(evidence) != len(response.history_evidence)
        or len(evidence_operations) != len(uncovered)
        or set(evidence_operations) != uncovered
    ):
        raise InvalidSnapshot("Response history evidence changed or duplicated uncovered facts")
    payments = {item.payment_id: item for item in request.calendar.payments}
    seen_components, seen_payments = set(), set()
    dates = {(date_from_cutoff + timedelta(days=step)).isoformat() for step in range(1, 31)}
    for part in response.components:
        if (
            not part.component_id
            or part.component_id in seen_components
            or part.date not in dates
            or min(part.inflow_minor, part.outflow_minor) < 0
        ):
            raise InvalidSnapshot("Response component identity/date/amount mismatch")
        seen_components.add(part.component_id)
        if part.kind == "known_remaining_payment":
            identifier = part.source_ids[0] if len(part.source_ids) == 1 else None
            if identifier not in payments or identifier in seen_payments:
                raise InvalidSnapshot("Response duplicated or invented known payment")
            payment = payments[identifier]
            if (
                part.date,
                part.inflow_minor,
                part.outflow_minor,
                part.category_code,
                part.regularity_key,
                part.category_id,
                part.category_company_id,
            ) != (
                payment.effective_date,
                payment.remaining_inflow_minor,
                payment.remaining_outflow_minor,
                payment.category_code,
                payment.regularity_key,
                payment.category_id,
                payment.category_company_id,
            ):
                raise InvalidSnapshot("Response known payment changed amount/date/category")
            seen_payments.add(identifier)
        elif (
            part.kind != "mean_90_uncovered"
            or len(part.source_ids) != 1
            or part.source_ids[0] not in evidence
        ):
            raise InvalidSnapshot("Response residual component lacks history evidence")
        parts[part.date][0] += part.inflow_minor
        parts[part.date][1] += part.outflow_minor
    if seen_payments != set(payments):
        raise InvalidSnapshot("Response omitted a known payment")
    balance, first_negative = s.initial_balance_minor, None
    for step, (day, band) in enumerate(zip(forecast.days, response.uncertainty.bands), 1):
        expected_date = (date_from_cutoff + timedelta(days=step)).isoformat()
        balance = checked(balance + day.inflow_minor - day.outflow_minor)
        if (
            day.date != expected_date
            or band.date != day.date
            or band.lower_minor > band.upper_minor
            or min(day.inflow_minor, day.outflow_minor) < 0
            or balance != day.reference_balance_minor
            or parts[day.date] != [day.inflow_minor, day.outflow_minor]
        ):
            raise InvalidSnapshot("Response date, money, component or band mismatch")
        if balance < 0 and first_negative is None:
            first_negative = day.date
    if first_negative != (
        forecast.first_negative_balance_date
        if forecast.HasField("first_negative_balance_date")
        else None
    ):
        raise InvalidSnapshot("Response first-negative date mismatch")
    return first_negative
