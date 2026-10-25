"""Explicit calendar replacement around mean_90; exact money and empirical bands."""

import hashlib
import random
from collections import defaultdict
from datetime import timedelta

from .inference import (
    MAX_INT64,
    MIN_INT64,
    MODEL_NAME,
    MODEL_VERSION,
    PREPROCESSING_VERSION,
    InvalidSnapshot,
    UnsupportedContext,
    _date,
    _uuid,
    validate_request,
)

FINANCIAL_VERSION = "mean_90_components_v1"
FINANCIAL_PREPROCESSING = "calendar_replacement_v1"
SCENARIOS = 512
BLOCK_DAYS = 7
SEED = 20261024


def checked(value):
    if not MIN_INT64 <= value <= MAX_INT64:
        raise InvalidSnapshot("Financial forecast exceeds signed int64 capacity")
    return value


def allocate(totals):
    """Largest remainders preserve the rounded total mean exactly, even by category."""
    result = {key: amount // 90 for key, amount in totals.items()}
    left = (sum(totals.values()) + 45) // 90 - sum(result.values())
    ranked = sorted(totals, key=lambda key: (-(totals[key] % 90), key))
    for key in ranked[:left]:
        result[key] += 1
    return result


def validate_category(record, company_id):
    if record.HasField("category_id"):
        _uuid(record.category_id)
        if record.category_company_id != company_id:
            raise InvalidSnapshot("Category must belong to the snapshot company")
    elif record.HasField("category_company_id"):
        raise InvalidSnapshot("Category company requires category ID")


def empirical_bands(history, known, initial, horizon):
    """Joint moving blocks preserve within-block inflow/outflow dependence."""
    rng = random.Random(SEED)
    values = [[] for _ in range(horizon)]
    for _ in range(SCENARIOS):
        balance = initial
        sample = []
        while len(sample) < horizon:
            start = rng.randrange(len(history) - BLOCK_DAYS + 1)
            sample.extend(history[start : start + BLOCK_DAYS])
        for step, (incoming, outgoing) in enumerate(sample[:horizon]):
            balance = checked(balance + incoming - outgoing + known[step][0] - known[step][1])
            values[step].append(balance)
    # Nearest-rank quantiles stay integer, without floating-point money conversion.
    return [(sorted(v)[51], sorted(v)[460]) for v in values]


def financial_forecast(request, pb):
    if request.HasField("csv_history"):
        raise InvalidSnapshot("CSV quality context requires the explicit CSV profile")
    if not request.HasField("model") or (
        request.model.model_name,
        request.model.model_version,
        request.model.preprocessing_version,
    ) != (MODEL_NAME, FINANCIAL_VERSION, FINANCIAL_PREPROCESSING):
        raise UnsupportedContext("Requested financial model context is unavailable")
    model = request.model
    if (
        model.configuration_id not in ("", "mean_90_components_v1")
        or (model.HasField("random_seed") and model.random_seed != str(SEED))
        or (model.HasField("trajectory_count") and model.trajectory_count != SCENARIOS)
    ):
        raise UnsupportedContext("Requested financial configuration is unavailable")
    if request.horizon_days != 30:
        raise InvalidSnapshot("Financial demo requires exactly 30 daily points")
    if not request.HasField("snapshot") or not request.HasField("calendar"):
        raise InvalidSnapshot("Financial snapshot and explicit calendar context are required")
    coverage_kinds = [
        item
        for item in request.availability_limitations
        if item
        in (
            "history_coverage_kind_certified",
            "history_coverage_kind_dataset_fixture_assumption",
        )
    ]
    fixture_limitation = "dataset_coverage_assumed_not_bank_certified"
    if (
        len(coverage_kinds) != 1
        or (
            coverage_kinds[0].endswith("dataset_fixture_assumption")
            and fixture_limitation not in request.availability_limitations
        )
        or (
            coverage_kinds[0].endswith("certified")
            and fixture_limitation in request.availability_limitations
        )
    ):
        raise InvalidSnapshot(
            "Financial input requires one consistent server-verified history coverage kind"
        )
    if request.calendar.HasField("scenario_context") or any(
        operation.category_rule_id for operation in request.operations
    ):
        raise InvalidSnapshot("Requested v2 financial fields are unavailable in this model version")
    # Reuse all coverage/EOD/money checks without changing the legacy RPC/version.
    base_request = pb.PredictRequest(
        snapshot=request.snapshot,
        horizon_days=30,
        model=pb.ModelContext(
            model_name=MODEL_NAME,
            model_version=MODEL_VERSION,
            preprocessing_version=PREPROCESSING_VERSION,
        ),
    )
    _, rounding = validate_request(base_request, pb, allow_reconstructed=True)
    s, calendar = request.snapshot, request.calendar
    cutoff = _date(s.cutoff_date)
    _uuid(calendar.snapshot_id)
    if calendar.data_version != s.data_version:
        raise InvalidSnapshot("Calendar and facts require one consistent snapshot data version")
    unavailable = calendar.knowledge_mode == pb.CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE
    if unavailable:
        if (
            calendar.as_of_business_date
            or calendar.groups
            or calendar.payments
            or calendar.snapshot_id != s.snapshot_id
            or "historical_calendar_unavailable" not in request.availability_limitations
        ):
            raise InvalidSnapshot(
                "Unavailable calendar requires facts identity, limitation and no schedule/date"
            )
    elif calendar.knowledge_mode == pb.CALENDAR_KNOWLEDGE_MODE_CURRENT:
        available_on = _date(calendar.as_of_business_date)
        # Live knowledge may be newer than yesterday's EOD. Never move that
        # calendar into an old historical backtest or bridge missing EOD days.
        if not cutoff <= available_on <= cutoff + timedelta(days=1):
            raise InvalidSnapshot("Current calendar requires current or preceding-day EOD")
        if available_on > cutoff and not calendar.no_post_cutoff_movements:
            raise InvalidSnapshot(
                "Next-day calendar requires confirmed absence of post-cutoff account movements"
            )
    elif calendar.knowledge_mode == pb.CALENDAR_KNOWLEDGE_MODE_HISTORICAL_AS_OF_CERTIFIED:
        available_on = _date(calendar.as_of_business_date)
        if (
            available_on != cutoff
            or "historical_availability_unverified" in request.availability_limitations
        ):
            raise InvalidSnapshot("Historical calendar requires certified as-of availability")
    else:
        raise InvalidSnapshot("Calendar knowledge mode must be explicit")
    if (
        len(request.operations) > 20000
        or len(calendar.payments) > 1000
        or len(calendar.groups) > 1000
    ):
        raise InvalidSnapshot("Financial snapshot exceeds bounded operation/calendar count")
    by_day = {raw.date: [0, 0] for raw in s.history}
    operations = {}
    for op in request.operations:
        _uuid(op.operation_id)
        validate_category(op, s.company_id)
        if op.operation_id in operations:
            raise InvalidSnapshot("Duplicate historical operation ID")
        if (
            op.date not in by_day
            or not op.category_code
            or not op.category_source
            or op.version < 1
        ):
            raise InvalidSnapshot(
                "Operation requires covered date, category provenance and version"
            )
        if not (
            (op.inflow_minor > 0 and op.outflow_minor == 0)
            or (op.outflow_minor > 0 and op.inflow_minor == 0)
        ):
            raise InvalidSnapshot("Operation requires exactly one positive direction")
        operations[op.operation_id] = op
        by_day[op.date][0] += op.inflow_minor
        by_day[op.date][1] += op.outflow_minor
    for raw in s.history:
        if by_day[raw.date] != [raw.inflow_minor, raw.outflow_minor]:
            raise InvalidSnapshot(
                "Historical operations must exactly reconstruct covered daily flows"
            )
    groups, covered = {}, set()
    for group in calendar.groups:
        if not group.group_id or group.group_id in groups or not group.complete_for_horizon:
            raise InvalidSnapshot("Calendar group must be unique and complete for the horizon")
        ids = list(group.historical_operation_ids)
        if len(set(ids)) != len(ids) or any(i not in operations or i in covered for i in ids):
            raise InvalidSnapshot(
                "Calendar replacement has duplicate, overlapping or unknown facts"
            )
        if (not ids and not group.additional_obligation_confirmed) or (
            ids and group.additional_obligation_confirmed
        ):
            raise InvalidSnapshot(
                "Additional obligation and historical replacement are explicit alternatives"
            )
        groups[group.group_id] = group
        covered.update(ids)
    known, known_components, seen = [[0, 0] for _ in range(30)], [], set()
    for payment in calendar.payments:
        _uuid(payment.payment_id)
        validate_category(payment, s.company_id)
        effective = _date(payment.effective_date)
        _date(payment.original_due_date)
        incoming, outgoing = payment.remaining_inflow_minor, payment.remaining_outflow_minor
        if payment.payment_id in seen or payment.version < 1 or payment.group_id not in groups:
            raise InvalidSnapshot("Payment requires unique ID, version and reconciliation group")
        if not (cutoff < effective <= cutoff + timedelta(days=30)):
            raise InvalidSnapshot(
                "Payment requires original due date and explicit effective horizon date"
            )
        if (
            not payment.category_code
            or not payment.source
            or not ((incoming > 0 and outgoing == 0) or (outgoing > 0 and incoming == 0))
        ):
            raise InvalidSnapshot("Known payment requires positive remaining amount and provenance")
        seen.add(payment.payment_id)
        step = (effective - cutoff).days - 1
        known[step][0] = checked(known[step][0] + incoming)
        known[step][1] = checked(known[step][1] + outgoing)
        known_components.append(
            pb.ForecastComponent(
                component_id="known:" + payment.payment_id,
                date=payment.effective_date,
                category_code=payment.category_code,
                inflow_minor=incoming,
                outflow_minor=outgoing,
                kind="known_remaining_payment",
                source_ids=[payment.payment_id],
                regularity_key=payment.regularity_key,
            )
        )
        if payment.HasField("category_id"):
            known_components[-1].category_id = payment.category_id
            known_components[-1].category_company_id = payment.category_company_id
    # Non-covered history is the only source of baseline and bootstrap residuals.
    residual = {raw.date: [0, 0] for raw in s.history}
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
        residual[op.date][0] += op.inflow_minor
        residual[op.date][1] += op.outflow_minor
        totals[0][key] += op.inflow_minor
        totals[1][key] += op.outflow_minor
        source_groups[key].append(identifier)
    allocated = [allocate(t) for t in totals]
    result = pb.PredictResponse(
        calendar_snapshot_id=calendar.snapshot_id,
        calendar_data_version=calendar.data_version,
        replaced_historical_operation_ids=sorted(covered),
        classified_operations=request.operations,
        availability_limitations=request.availability_limitations,
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
    result.MergeFrom(
        pb.PredictResponse(
            snapshot_id=s.snapshot_id,
            company_id=s.company_id,
            account_id=s.account_id,
            data_version=s.data_version,
            model=request.model,
            currency=s.currency,
            cutoff_date=s.cutoff_date,
            start_date=(cutoff + timedelta(days=1)).isoformat(),
            history_only_includes_recurring_flows=unavailable,
            warnings=["UNCALIBRATED_NO_PROBABILITY"],
        )
    )
    if rounding:
        result.warnings.append("OBSERVED_RECONCILIATION_WITHIN_ONE_MINOR_UNIT")
    result.warnings.extend(
        [
            "HISTORY_ONLY_NO_CALENDAR" if unavailable else "EXPLICIT_COMPLETE_CALENDAR_REPLACEMENT",
            "UNCALIBRATED_EMPIRICAL_BANDS",
        ]
    )
    balance = s.initial_balance_minor
    for step in range(30):
        day = (cutoff + timedelta(days=step + 1)).isoformat()
        incoming = checked(sum(allocated[0].values()) + known[step][0])
        outgoing = checked(sum(allocated[1].values()) + known[step][1])
        balance = checked(balance + incoming - outgoing)
        result.days.add(
            date=day, inflow_minor=incoming, outflow_minor=outgoing, reference_balance_minor=balance
        )
        if balance < 0 and not result.HasField("first_negative_balance_date"):
            result.first_negative_balance_date = day
        for key in sorted(set(allocated[0]) | set(allocated[1])):
            inc, out = allocated[0].get(key, 0), allocated[1].get(key, 0)
            if inc or out:
                result.components.add(
                    component_id=f"residual:{step}:{evidence_ids[key]}",
                    date=day,
                    category_code=key[0],
                    inflow_minor=inc,
                    outflow_minor=out,
                    kind="mean_90_uncovered",
                    source_ids=[evidence_ids[key]],
                    regularity_key=key[2],
                    category_id=key[3] if key[3] else None,
                    category_company_id=key[4] if key[4] else None,
                )
    result.components.extend(known_components)
    result.uncertainty.method = "paired_moving_block_bootstrap_uncalibrated"
    result.uncertainty.scenarios, result.uncertainty.block_days, result.uncertainty.seed = (
        SCENARIOS,
        BLOCK_DAYS,
        SEED,
    )
    result.uncertainty.lower_quantile, result.uncertainty.upper_quantile = 0.1, 0.9
    result.uncertainty.calibrated = False
    result.uncertainty.gap_probability_unavailable_reason = (
        "REQUIREMENT_OPEN_NO_CALIBRATED_GAP_PROBABILITY"
    )
    result.uncertainty.assumptions.extend(
        [
            "Past uncovered 7-day blocks represent future residual flows; calendar amounts/dates are fixed.",
            "Only 90 days; changing business conditions and calendar-payment uncertainty are not modeled.",
            "Nominal empirical quantiles are not validated coverage or whole-horizon gap probability.",
        ]
    )
    bands = empirical_bands(
        [residual[r.date] for r in s.history], known, s.initial_balance_minor, 30
    )
    for point, (low, high) in zip(result.days, bands):
        result.uncertainty.bands.add(date=point.date, lower_minor=low, upper_minor=high)
    result.gap_probability.unavailable_reason = "not_calibrated"
    for point in result.days:
        point.negative_balance_probability.unavailable_reason = "not_calibrated"
    return result
