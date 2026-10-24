"""Rehearse the deployed demo company over Predict: Core-shaped calls, not Core itself.

The fixture mirrors deploy/stack/seed-demo.py: 90 days with one inflow of
100000 and one outflow of 400000 minor units each, accepted EOD 5000000 at
cutoff, explicit 90-day coverage. Case A is encoded like Core encodeFinancial
(calendar unavailable) and checked with Core validateFinancialResponse rules.
Cases B-C add a confirmed manual payment with a CURRENT calendar, which the
Core adapter in main does not send yet; they verify ML arithmetic only.
"""

import argparse
import json
from datetime import date, timedelta
from pathlib import Path
from uuid import NAMESPACE_URL, uuid5

import grpc
from cashflow_rpc.core_input import Coverage, prepare_financial_request, verify_response
from google.protobuf.json_format import MessageToJson
from ml.v1 import forecast_pb2 as pb
from ml.v1 import forecast_pb2_grpc as rpc

INFLOW, OUTFLOW, EOD = 100_000, 400_000, 5_000_000
PAYMENT, PAYMENT_DAY, PAID = 1_000_000, 5, 300_000
CORE_LIMITATIONS = ("historical_calendar_unavailable", "historical_availability_unverified")
CORE_WARNINGS = (
    "HISTORY_ONLY_NO_CALENDAR",
    "UNCALIBRATED_NO_PROBABILITY",
    "UNCALIBRATED_EMPIRICAL_BANDS",
)


def uid(value):
    return str(uuid5(NAMESPACE_URL, "saldo-demo-company-rehearsal/" + value))


def operation(day, name, inflow=0, outflow=0, kind="manual"):
    return pb.FinancialOperation(
        operation_id=uid(f"{day}/{name}"),
        date=day.isoformat(),
        inflow_minor=inflow,
        outflow_minor=outflow,
        category_code="unknown",
        category_source="unknown",
        counterparty_name="Синтетический контрагент",
        payment_purpose="Демонстрационная операция, не реальные данные",
        version=1,
        factual_source_kind=kind,
    )


def build(cutoff, version, calendar=None, extra=(), eod=EOD):
    operations = []
    for back in range(90):
        day = cutoff - timedelta(days=back)
        operations.append(operation(day, "in", inflow=INFLOW))
        operations.append(operation(day, "out", outflow=OUTFLOW))
    operations.extend(extra)
    snapshot = pb.ForecastSnapshot(
        snapshot_id=uid(f"snapshot/{cutoff}/{version}"),
        company_id=uid("company"),
        account_id=uid("account"),
        data_version=version,
        currency="RUB",
        cutoff_date=cutoff.isoformat(),
        initial_balance_minor=eod,
    )
    coverage = Coverage(
        (cutoff - timedelta(days=89)).isoformat(),
        cutoff.isoformat(),
        f"Complete synthetic demo fixture: {len(operations)} known operations",
    )
    request = prepare_financial_request(
        snapshot,
        operations,
        coverage,
        pb,
        calendar=calendar,
        limitations=() if calendar else CORE_LIMITATIONS,
    )
    request.model.configuration_id = "mean_90_components_v1"  # set by Core encodeFinancial
    return request


def current_calendar(request_version, cutoff, payments, groups):
    return pb.FinancialCalendar(
        snapshot_id=uid(f"calendar/{cutoff}/{request_version}"),
        data_version=request_version,
        as_of_business_date=(cutoff + timedelta(days=1)).isoformat(),
        knowledge_mode=pb.CALENDAR_KNOWLEDGE_MODE_CURRENT,
        no_post_cutoff_movements=True,  # synthetic clock: nothing happened after cutoff
        groups=groups,
        payments=payments,
    )


def manual_payment(cutoff, remaining, version=1):
    due = (cutoff + timedelta(days=PAYMENT_DAY)).isoformat()
    return pb.ScheduledPayment(
        payment_id=uid("manual-payment"),
        version=version,
        effective_date=due,
        original_due_date=due,
        remaining_outflow_minor=remaining,
        category_code="manual_demo_obligation",
        group_id="manual-payment",
        source="manual_synthetic_demo",
    )


def first_negative(cutoff, balances):
    for index, value in enumerate(balances, 1):
        if value < 0:
            return (cutoff + timedelta(days=index)).isoformat()
    return None


def unavailable(probability):
    return probability.WhichOneof("estimate") == "unavailable_reason" and bool(
        probability.unavailable_reason
    )


def check_core_rules(request, response):
    """Port of Core validateFinancialResponse for the calendar-unavailable profile."""
    history = request.snapshot.history
    mean_in = (sum(d.inflow_minor for d in history) + 45) // 90
    mean_out = (sum(d.outflow_minor for d in history) + 45) // 90
    cutoff = date.fromisoformat(request.snapshot.cutoff_date)
    problems = []
    if len(response.days) != 30 or response.replaced_historical_operation_ids:
        problems.append("days_or_replacement")
    if not response.history_only_includes_recurring_flows:
        problems.append("history_only_flag")
    if list(response.availability_limitations) != list(request.availability_limitations):
        problems.append("limitations")
    if list(response.classified_operations) != list(request.operations):
        problems.append("classified_operations")
    if any(w not in response.warnings for w in CORE_WARNINGS):
        problems.append("warnings")
    if response.gap_probability.unavailable_reason != "not_calibrated":
        problems.append("gap_probability")
    balance, first = request.snapshot.initial_balance_minor, None
    for n, day in enumerate(response.days):
        if (
            day.date != (cutoff + timedelta(days=n + 1)).isoformat()
            or day.HasField("balance_lower_minor")
            or day.HasField("balance_upper_minor")
            or day.negative_balance_probability.unavailable_reason != "not_calibrated"
            or (day.inflow_minor, day.outflow_minor) != (mean_in, mean_out)
        ):
            problems.append(f"day_{n + 1}")
        balance += day.inflow_minor - day.outflow_minor
        if balance != day.reference_balance_minor:
            problems.append(f"balance_{n + 1}")
        if first is None and balance < 0:
            first = day.date
    if (response.first_negative_balance_date or None) != first:
        problems.append("first_negative")
    return problems


def run(target, args):
    output = args.output_dir.resolve()
    cutoff = args.cutoff
    paid_fact = operation(cutoff, "partial-paid", outflow=PAID, kind="synthetic_partial_payment")
    added = pb.CalendarGroup(
        group_id="manual-payment", complete_for_horizon=True, additional_obligation_confirmed=True
    )
    reconciled = pb.CalendarGroup(
        group_id="manual-payment",
        historical_operation_ids=[paid_fact.operation_id],
        complete_for_horizon=True,
    )
    net = INFLOW - OUTFLOW
    base = [EOD + net * k for k in range(1, 31)]
    with_payment = [b - (PAYMENT if k >= PAYMENT_DAY else 0) for k, b in enumerate(base, 1)]
    remaining = PAYMENT - PAID
    partial = [b - PAID - (remaining if k >= PAYMENT_DAY else 0) for k, b in enumerate(base, 1)]
    cases = {
        "A_core_shape_no_payment": (build(cutoff, 1), base),
        "B_manual_payment_added": (
            build(
                cutoff, 2, current_calendar(2, cutoff, [manual_payment(cutoff, PAYMENT)], [added])
            ),
            with_payment,
        ),
        "C_partial_paid_reconciled": (
            build(
                cutoff,
                3,
                current_calendar(
                    3, cutoff, [manual_payment(cutoff, remaining, version=2)], [reconciled]
                ),
                [paid_fact],
                EOD - PAID,
            ),
            partial,
        ),
    }
    # Same facts without the replacement link: the paid part leaks into the mean.
    naive = build(
        cutoff,
        4,
        current_calendar(4, cutoff, [manual_payment(cutoff, remaining, version=2)], [added]),
        [paid_fact],
        EOD - PAID,
    )
    output.mkdir(parents=True)
    report = {"cutoff": cutoff.isoformat(), "main": args.main_sha, "steps": {}}
    with channel_for(target, args) as channel:
        grpc.channel_ready_future(channel).result(timeout=5)
        client = rpc.ForecastServiceStub(channel)
        responses = {}
        for name, (request, wanted) in cases.items():
            response = client.Predict(request, timeout=10)
            verify_response(request, response)
            actual = [day.reference_balance_minor for day in response.days]
            payment_parts = [
                c for c in response.components if uid("manual-payment") in c.source_ids
            ]
            step = {
                "days": len(response.days),
                "dates_consecutive": [d.date for d in response.days]
                == [(cutoff + timedelta(days=k)).isoformat() for k in range(1, 31)],
                "exact_balances": actual == wanted,
                "first_negative": response.first_negative_balance_date or None,
                "expected_first_negative": first_negative(cutoff, wanted),
                "closing_minor": actual[-1],
                "horizon_outflow_minor": sum(d.outflow_minor for d in response.days),
                "payment_components": len(payment_parts),
                "payment_component_outflow_minor": sum(c.outflow_minor for c in payment_parts),
                "replaced_ids": list(response.replaced_historical_operation_ids),
                "gap_probability_unavailable_reason": response.gap_probability.unavailable_reason
                or None,
                "uncertainty_calibrated": response.uncertainty.calibrated,
                "uncertainty_reason": response.uncertainty.gap_probability_unavailable_reason,
                "all_days_probability_unavailable": all(
                    unavailable(d.negative_balance_probability)
                    and d.negative_balance_probability.unavailable_reason == "not_calibrated"
                    for d in response.days
                ),
                "repeat_identical": client.Predict(request, timeout=10) == response,
            }
            if name.startswith("A_"):
                step["core_rule_problems"] = check_core_rules(request, response)
            report["steps"][name] = step
            responses[name] = response
            for label, message in (("request", request), ("response", response)):
                (output / f"{name}-{label}.json").write_text(
                    MessageToJson(message, preserving_proto_field_name=True), encoding="utf-8"
                )
        naive_response = client.Predict(naive, timeout=10)
        verify_response(naive, naive_response)
    c_days = responses["C_partial_paid_reconciled"].days
    report["double_count_check"] = {
        "reconciled_closing_minor": c_days[-1].reference_balance_minor,
        "naive_closing_minor": naive_response.days[-1].reference_balance_minor,
        "naive_extra_outflow_per_day": naive_response.days[0].outflow_minor
        - c_days[0].outflow_minor,
    }
    checks = report["steps"]
    b, c = checks["B_manual_payment_added"], checks["C_partial_paid_reconciled"]
    report["passed"] = (
        all(
            s["days"] == 30
            and s["dates_consecutive"]
            and s["exact_balances"]
            and s["first_negative"] == s["expected_first_negative"]
            and s["gap_probability_unavailable_reason"] == "not_calibrated"
            and s["uncertainty_reason"] == "REQUIREMENT_OPEN_NO_CALIBRATED_GAP_PROBABILITY"
            and not s["uncertainty_calibrated"]
            and s["all_days_probability_unavailable"]
            and s["repeat_identical"]
            for s in checks.values()
        )
        and not checks["A_core_shape_no_payment"]["core_rule_problems"]
        and b["payment_components"] == 1
        and b["payment_component_outflow_minor"] == PAYMENT
        and b["horizon_outflow_minor"] == 30 * OUTFLOW + PAYMENT
        and c["replaced_ids"] == [paid_fact.operation_id]
        and c["payment_components"] == 1
        and c["payment_component_outflow_minor"] == remaining
        and c["horizon_outflow_minor"] == 30 * OUTFLOW + remaining
        and c["closing_minor"] == b["closing_minor"]
        and report["double_count_check"]["naive_extra_outflow_per_day"] > 0
    )
    report["scope"] = (
        "Core-shaped client -> ML over the given transport; not real Core/API/frontend. "
        "Core main sends calendar UNAVAILABLE and rejects known payments, so B-C are ML-only."
    )
    (output / "summary.json").write_text(
        json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8"
    )
    print(json.dumps(report, ensure_ascii=False, indent=2))
    if not report["passed"]:
        raise SystemExit("Demo company rehearsal failed")


def channel_for(target, args):
    if args.insecure_local:
        if target.rsplit(":", 1)[0] != "127.0.0.1":
            raise ValueError("Insecure fixture client requires IPv4 loopback")
        return grpc.insecure_channel(target)
    credentials = grpc.ssl_channel_credentials(
        root_certificates=args.ca.read_bytes(),
        private_key=args.private_key.read_bytes(),
        certificate_chain=args.certificate.read_bytes(),
    )
    options = [("grpc.ssl_target_name_override", args.server_name)] if args.server_name else []
    return grpc.secure_channel(target, credentials, options=options)


def main():
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("--target", required=True)
    parser.add_argument("--insecure-local", action="store_true")
    parser.add_argument("--ca", type=Path)
    parser.add_argument("--certificate", type=Path)
    parser.add_argument("--private-key", type=Path)
    parser.add_argument("--server-name", help="TLS name of the ML certificate, e.g. ml")
    parser.add_argument("--cutoff", type=date.fromisoformat, default=date(2026, 9, 30))
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--main-sha", required=True)
    args = parser.parse_args()
    if args.output_dir.exists():
        parser.error("Output must be new")
    tls = (args.ca, args.certificate, args.private_key)
    if args.insecure_local and any(tls):
        parser.error("Local insecure mode and TLS credentials cannot be mixed")
    if not args.insecure_local and not all(tls):
        parser.error("An external endpoint requires mTLS credentials")
    run(args.target, args)


if __name__ == "__main__":
    main()
