"""Synthetic payment lifecycle on the canonical single Predict RPC, not a backtest."""

import argparse
import json
from datetime import date, timedelta
from pathlib import Path
from uuid import NAMESPACE_URL, uuid5

import grpc
from cashflow_baseline.categories import categorize
from cashflow_baseline.data import history_at, load_companies, sha256
from cashflow_baseline.models import predict
from cashflow_rpc.core_input import Coverage, prepare_financial_request, verify_response
from google.protobuf.json_format import MessageToJson
from ml.v1 import forecast_pb2 as pb
from ml.v1 import forecast_pb2_grpc as rpc
from verify_dataset_rpc import channel_for, local_server

SERVICE = Path(__file__).resolve().parent
DEMO_DATE = date(2026, 10, 1)
CUTOFF = DEMO_DATE - timedelta(days=1)


def uid(value):
    return str(uuid5(NAMESPACE_URL, "saldo-manual-demo-v2/" + value))


def build(company, history, version, payments=(), groups=(), extra=(), paid=0):
    operations = []
    for operation in company.operations:
        if not CUTOFF - timedelta(days=89) <= operation.day <= CUTOFF:
            continue
        category = categorize(operation)
        operations.append(
            pb.FinancialOperation(
                operation_id=uid(company.company_id + "/" + operation.operation_id),
                date=operation.day.isoformat(),
                inflow_minor=operation.amount if operation.incoming else 0,
                outflow_minor=0 if operation.incoming else operation.amount,
                category_code=category.label,
                category_source="rule:" + category.rule,
                version=1,
                factual_source_kind="dataset_fixture_import",
                source_namespace="participant_dataset",
                external_operation_id=operation.operation_id,
            )
        )
    operations.extend(extra)
    snapshot = pb.ForecastSnapshot(
        snapshot_id=uid("snapshot/" + str(version)),
        company_id=uid("company/" + company.company_id),
        account_id=uid("account/" + company.company_id),
        data_version=version,
        currency="RUB",
        cutoff_date=CUTOFF.isoformat(),
        initial_balance_minor=history[-1].balance - paid,
    )
    calendar = pb.FinancialCalendar(
        snapshot_id=uid("calendar/" + str(version)),
        data_version=version,
        as_of_business_date=DEMO_DATE.isoformat(),
        knowledge_mode=pb.CALENDAR_KNOWLEDGE_MODE_CURRENT,
        # Explicit synthetic clock: every added fact is on cutoff, none after it.
        no_post_cutoff_movements=True,
        groups=groups,
        payments=payments,
    )
    return prepare_financial_request(
        snapshot,
        operations,
        Coverage(
            (CUTOFF - timedelta(days=89)).isoformat(),
            CUTOFF.isoformat(),
            "dataset_coverage_assumed_not_bank_certified",
            "dataset_fixture_assumption",
        ),
        pb,
        calendar=calendar,
        limitations=(
            "synthetic_manual_payment_demo_not_historical_calendar",
            "dataset_coverage_assumed_not_bank_certified",
        ),
    )


def payment(amount, version, day):
    due = (CUTOFF + timedelta(days=day)).isoformat()
    return pb.ScheduledPayment(
        payment_id=uid("manual-payment"),
        version=version,
        effective_date=due,
        original_due_date=due,
        remaining_outflow_minor=amount,
        category_code="manual_demo_obligation",
        group_id="manual-payment",
        source="manual_synthetic_demo",
    )


def run(target, args):
    output = args.output_dir.resolve()
    companies = load_companies(args.data_dir)
    company, history = None, None
    for candidate in companies:
        values, reason = history_at(candidate, CUTOFF, 90)
        if not reason and candidate.last_date == CUTOFF:
            company, history = candidate, values
            break
    if company is None:
        raise ValueError("No company has a clean observed cutoff for the demo clock")
    baseline = [p.balance for p in predict(history, 30, "mean_90")]
    if min(baseline) < 0:
        raise ValueError("This fixture requires a nonnegative baseline before adding a payment")
    # Divisibility by 270 makes the partial-payment residual test exact in kopecks.
    amount = ((baseline[9] + 10_000 + 269) // 270) * 270
    edited, paid = amount + 27_000, (amount + 27_000) // 3
    additional = pb.CalendarGroup(
        group_id="manual-payment", complete_for_horizon=True, additional_obligation_confirmed=True
    )
    fact = pb.FinancialOperation(
        operation_id=uid("paid-fact"),
        date=CUTOFF.isoformat(),
        outflow_minor=paid,
        category_code="manual_demo_obligation",
        category_source="manual_match",
        version=1,
        factual_source_kind="synthetic_partial_payment",
        source_namespace="synthetic_demo",
        external_operation_id="partial-1",
    )
    replacement = pb.CalendarGroup(
        group_id="manual-payment",
        historical_operation_ids=[fact.operation_id],
        complete_for_horizon=True,
    )

    def expected(value, day, initial_paid=0):
        return [
            b - initial_paid - (value if index >= day else 0) for index, b in enumerate(baseline, 1)
        ]

    cases = [
        ("0_baseline", build(company, history, 1), baseline),
        (
            "1_add_day10",
            build(company, history, 2, [payment(amount, 1, 10)], [additional]),
            expected(amount, 10),
        ),
        (
            "2_edit_amount",
            build(company, history, 3, [payment(edited, 2, 10)], [additional]),
            expected(edited, 10),
        ),
        (
            "3_move_day20",
            build(company, history, 4, [payment(edited, 3, 20)], [additional]),
            expected(edited, 20),
        ),
        (
            "4_partial_reconciled",
            build(
                company, history, 5, [payment(edited - paid, 4, 20)], [replacement], [fact], paid
            ),
            expected(edited - paid, 20, paid),
        ),
        (
            "5_cancel_remaining",
            build(company, history, 6, [], [replacement], [fact], paid),
            expected(0, 31, paid),
        ),
    ]
    output.mkdir(parents=True)
    steps = []
    with channel_for(target, args) as channel:
        grpc.channel_ready_future(channel).result(timeout=5)
        client = rpc.ForecastServiceStub(channel)
        for name, request, wanted in cases:
            response = client.Predict(request, timeout=10)
            verify_response(request, response)
            actual = [day.reference_balance_minor for day in response.days]
            if actual != wanted or client.Predict(request, timeout=10) != response:
                raise ValueError("Exact arithmetic or repeat check failed: " + name)
            if response.gap_probability.WhichOneof("estimate") != "unavailable_reason":
                raise ValueError("Uncalibrated probability must remain unavailable")
            for label, message in (("request", request), ("response", response)):
                (output / f"{name}-{label}.json").write_text(
                    MessageToJson(message, preserving_proto_field_name=True), encoding="utf-8"
                )
            steps.append(
                {
                    "step": name,
                    "exact_daily_balances": True,
                    "repeat_exact": True,
                    "first_negative": response.first_negative_balance_date or None,
                    "closing_minor": actual[-1],
                }
            )
        if steps[1]["first_negative"] != (CUTOFF + timedelta(days=10)).isoformat():
            raise ValueError("Adding the synthetic payment must demonstrate a new negative balance")
        duplicate = pb.PredictRequest()
        duplicate.CopyFrom(cases[-1][1])
        duplicate.operations.add().CopyFrom(fact)
        stale_bridge = pb.PredictRequest()
        stale_bridge.CopyFrom(cases[0][1])
        stale_bridge.calendar.no_post_cutoff_movements = False
        for name, request in (
            ("duplicate_internal_id", duplicate),
            ("next_day_without_attestation", stale_bridge),
        ):
            try:
                client.Predict(request, timeout=10)
            except grpc.RpcError as exc:
                if exc.code() != grpc.StatusCode.INVALID_ARGUMENT:
                    raise
                steps.append({"step": name, "status": "INVALID_ARGUMENT"})
            else:
                raise ValueError("Invalid request was accepted: " + name)
        naive = build(
            company, history, 7, [payment(edited - paid, 4, 20)], [additional], [fact], paid
        )
        naive_result = client.Predict(naive, timeout=10)
        verify_response(naive, naive_result)
        extra_outflow = cases[4][2][-1] - naive_result.days[-1].reference_balance_minor
        if extra_outflow != paid // 3:
            raise ValueError("Partial payment trap did not match the independent arithmetic")
        clone = pb.FinancialOperation()
        clone.CopyFrom(fact)
        clone.operation_id = uid("reimport-new-internal-id")
        reimport = build(company, history, 8, [], [replacement], [fact, clone], 2 * paid)
        reimport_result = client.Predict(reimport, timeout=10)
        verify_response(reimport, reimport_result)
    result = {
        "synthetic": True,
        "company": company.company_id,
        "cutoff": CUTOFF.isoformat(),
        "demo_date": DEMO_DATE.isoformat(),
        "main": args.main_sha,
        "rpc": "ml.v1.ForecastService/Predict",
        "financial_model": "mean_90/mean_90_components_v1/calendar_replacement_v1",
        "source_sha256": {
            name: sha256(args.data_dir / name) for name in ("all_operations.csv", "companies.csv")
        },
        "payment_minor": amount,
        "edited_minor": edited,
        "paid_minor": paid,
        "steps": steps,
        "naive_partial_extra_outflow_minor": extra_outflow,
        "new_uuid_same_external_key_accepted_by_ml": True,
        "core_import_idempotence_verified": False,
        "core_worker_storage_api_e2e_verified": False,
        "gap_probability": None,
        "transport": "separate_local_process_insecure_loopback"
        if args.start_local_server
        else "external_insecure_loopback"
        if args.insecure_local
        else "external_mtls",
        "limitations": [
            "synthetic future payments and partial fact, not historical knowledge",
            "fixture UUIDv5/version counters, not production IDs",
            "coverage is an explicit dataset assumption",
            "Core must deduplicate full scoped external keys and provide true reconciliation links",
        ],
    }
    (output / "summary.json").write_text(
        json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8"
    )
    print(json.dumps(result, ensure_ascii=False))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--start-local-server", action="store_true")
    mode.add_argument("--target")
    parser.add_argument("--insecure-local", action="store_true")
    parser.add_argument("--ca", type=Path)
    parser.add_argument("--certificate", type=Path)
    parser.add_argument("--private-key", type=Path)
    parser.add_argument("--data-dir", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--main-sha", required=True)
    args = parser.parse_args()
    if args.output_dir.exists():
        parser.error("Output must be new")
    source, output = args.data_dir.resolve(), args.output_dir.resolve()
    if source == output or source in output.parents or output in source.parents:
        parser.error("Input data and output must not overlap")
    tls = (args.ca, args.certificate, args.private_key)
    if (args.start_local_server or args.insecure_local) and any(tls):
        parser.error("Local insecure mode and TLS credentials cannot be mixed")
    if not (args.start_local_server or args.insecure_local) and not all(tls):
        parser.error("An external endpoint requires mTLS credentials")
    if args.start_local_server:
        with local_server() as target:
            run(target, args)
    else:
        run(args.target, args)


if __name__ == "__main__":
    main()
