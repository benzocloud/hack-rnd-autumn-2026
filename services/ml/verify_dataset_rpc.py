"""Dataset client for a real RPC; replay fixtures are not historical calendar facts."""

import argparse
import csv
import hashlib
import json
import subprocess
import sys
from collections import defaultdict
from contextlib import contextmanager
from datetime import date, timedelta
from pathlib import Path
from uuid import NAMESPACE_URL, uuid5

import grpc
from cashflow_baseline.categories import categorize
from cashflow_baseline.data import history_at, load_companies, parse_money, sha256
from cashflow_baseline.models import predict, rounded_mean
from cashflow_baseline.recurring import infer_patterns
from cashflow_rpc.core_input import Coverage, prepare_financial_request, verify_response
from google.protobuf.json_format import MessageToJson
from ml.v1 import forecast_pb2 as pb
from ml.v1 import forecast_pb2_grpc as rpc

SERVICE = Path(__file__).resolve().parent
ROOT = SERVICE.parents[1]
SCENARIO_LIMITATION = "synthetic_calendar_scenario_not_historical_fact"


def fixture_id(value):
    """Deterministic UUIDv5 only for local fixtures; production IDs belong to Core."""
    return str(uuid5(NAMESPACE_URL, "saldo-dataset-rpc-fixture/" + value))


def dataset_request(data_dir, company_id, cutoff):
    company = next((c for c in load_companies(data_dir) if c.company_id == company_id), None)
    if company is None:
        raise ValueError("Company is absent from dataset")
    history, reason = history_at(company, cutoff, 90)
    if reason:
        raise ValueError("Dataset window is not eligible: " + reason)
    patterns = infer_patterns(company, cutoff)
    keys = {}
    for pattern in patterns:
        key = fixture_id(repr((company_id, pattern.category, pattern.counterparty, pattern.anchor)))
        for identifier in pattern.matched_ids:
            keys[identifier] = key
    operations = []
    for operation in company.operations:
        if not cutoff - timedelta(days=89) <= operation.day <= cutoff:
            continue
        category = categorize(operation)
        operations.append(
            pb.FinancialOperation(
                operation_id=fixture_id(operation.operation_id),
                date=operation.day.isoformat(),
                inflow_minor=operation.amount if operation.incoming else 0,
                outflow_minor=0 if operation.incoming else operation.amount,
                category_code=category.label,
                category_source="rule:" + category.rule,
                regularity_key=keys.get(operation.operation_id, ""),
                version=1,
                category_id=fixture_id(company_id + "/category/" + category.label),
                category_company_id=fixture_id(company_id),
                counterparty_inn=operation.inn,
                counterparty_name=operation.name,
                payment_purpose=operation.purpose,
                factual_source_kind="dataset_fixture_import",
                source_namespace="participant_dataset",
                external_operation_id=operation.operation_id,
            )
        )
    snapshot = pb.ForecastSnapshot(
        snapshot_id=fixture_id(company_id + "/" + cutoff.isoformat()),
        company_id=fixture_id(company_id),
        account_id=fixture_id(company_id + "/account"),
        data_version=1,
        currency="RUB",
        cutoff_date=cutoff.isoformat(),
        initial_balance_minor=history[-1].balance,
    )
    coverage = Coverage(
        start=(cutoff - timedelta(days=89)).isoformat(),
        end=cutoff.isoformat(),
        reference="dataset_fixture_assumption:" + sha256(data_dir / "all_operations.csv"),
        kind="dataset_fixture_assumption",
    )
    request = prepare_financial_request(
        snapshot,
        operations,
        coverage,
        pb,
        limitations=(
            "historical_availability_unverified",
            "dataset_coverage_assumed_not_bank_certified",
        ),
    )
    return request, history, len(patterns), coverage


def calendar_scenario(base):
    """Controlled replay: manual future salary is invented, explicitly not backtest truth."""
    request = pb.PredictRequest()
    request.CopyFrom(base)
    grouped = defaultdict(list)
    for operation in request.operations:
        if operation.category_code == "salary_final" and operation.outflow_minor:
            grouped[operation.counterparty_inn].append(operation)
    if not grouped:
        raise ValueError("Fixture company requires a historical salary_final group")
    group = max(grouped.values(), key=lambda values: (len(values), values[0].counterparty_inn))
    request.snapshot.snapshot_id = fixture_id(base.snapshot.snapshot_id + "/scenario-facts")
    request.snapshot.data_version = base.snapshot.data_version + 1
    request.calendar.snapshot_id = fixture_id(base.snapshot.snapshot_id + "/scenario-calendar")
    request.calendar.data_version = request.snapshot.data_version
    request.calendar.knowledge_mode = pb.CALENDAR_KNOWLEDGE_MODE_CURRENT
    # The replay clock is cutoff. This is not today's calendar backdated as fact.
    request.calendar.as_of_business_date = base.snapshot.cutoff_date
    request.availability_limitations.append(SCENARIO_LIMITATION)
    request.calendar.groups.add(
        group_id="scenario-salary",
        historical_operation_ids=[operation.operation_id for operation in group],
        complete_for_horizon=True,
    )
    due = (date.fromisoformat(base.snapshot.cutoff_date) + timedelta(days=5)).isoformat()
    request.calendar.payments.add(
        payment_id=fixture_id(base.snapshot.snapshot_id + "/manual-scenario-salary"),
        version=1,
        effective_date=due,
        original_due_date=due,
        remaining_outflow_minor=4_500_000,
        category_code=group[0].category_code,
        group_id="scenario-salary",
        source="manual_synthetic_scenario",
        regularity_key=group[0].regularity_key,
        category_id=group[0].category_id,
        category_company_id=group[0].category_company_id,
    )
    return request


def frozen_days(path, company, cutoff):
    result = []
    with path.open(encoding="utf-8-sig", newline="") as source:
        for row in csv.DictReader(source, delimiter=";"):
            if (row["company_id"], row["cutoff_date"], row["method"]) == (
                company,
                cutoff,
                "mean_90",
            ):
                result.append(
                    (
                        row["date"],
                        parse_money(row["predicted_inflow"]),
                        parse_money(row["predicted_outflow"]),
                        parse_money(row["predicted_balance"]),
                    )
                )
    return sorted(result)


def day_values(response):
    return [
        (d.date, d.inflow_minor, d.outflow_minor, d.reference_balance_minor) for d in response.days
    ]


@contextmanager
def local_server():
    """Own a separate server process; cleanup touches only this child."""
    process = subprocess.Popen(
        [sys.executable, "-B", str(SERVICE / "run_server.py"), "--insecure-local", "--port", "0"],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
    )
    try:
        line = process.stdout.readline().strip()
        prefix = "ML gRPC listening on "
        if not line.startswith(prefix):
            raise RuntimeError("Local server did not start: " + line)
        yield line[len(prefix) :]
    finally:
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)
        process.stdout.close()


def channel_for(target, args):
    if args.start_local_server or args.insecure_local:
        if target.rsplit(":", 1)[0] != "127.0.0.1":
            raise ValueError("Insecure fixture client requires IPv4 loopback")
        return grpc.insecure_channel(target)
    credentials = grpc.ssl_channel_credentials(
        root_certificates=args.ca.read_bytes(),
        private_key=args.private_key.read_bytes(),
        certificate_chain=args.certificate.read_bytes(),
    )
    return grpc.secure_channel(target, credentials)


def run(target, args):
    request, history, patterns, coverage = dataset_request(args.data_dir, args.company, args.cutoff)
    scenario = calendar_scenario(request)
    updated = pb.PredictRequest()
    updated.CopyFrom(scenario)
    updated.snapshot.snapshot_id = fixture_id(scenario.snapshot.snapshot_id + "/remaining-update")
    updated.snapshot.data_version += 1
    updated.calendar.snapshot_id = fixture_id(scenario.calendar.snapshot_id + "/remaining-update")
    updated.calendar.data_version = updated.snapshot.data_version
    updated.calendar.payments[0].version = 2
    updated.calendar.payments[0].remaining_outflow_minor -= 500_000
    requests = {
        "history-only": request,
        "calendar-scenario": scenario,
        "remaining-updated": updated,
    }
    with channel_for(target, args) as channel:
        grpc.channel_ready_future(channel).result(timeout=5)
        stub = rpc.ForecastServiceStub(channel)
        responses = {name: stub.Predict(value, timeout=3) for name, value in requests.items()}
        repeated = stub.Predict(request, timeout=3)
        if repeated != responses["history-only"]:
            raise ValueError("Repeated request changed result")
        duplicate = pb.PredictRequest()
        duplicate.CopyFrom(scenario)
        duplicate.calendar.payments.add().CopyFrom(duplicate.calendar.payments[0])
        try:
            stub.Predict(duplicate, timeout=3)
        except grpc.RpcError as exc:
            if exc.code() != grpc.StatusCode.INVALID_ARGUMENT:
                raise
        else:
            raise ValueError("Duplicate payment was accepted")
    for name, response in responses.items():
        verify_response(requests[name], response)
    expected = [
        (p.day.isoformat(), p.inflow, p.outflow, p.balance) for p in predict(history, 30, "mean_90")
    ]
    frozen = frozen_days(args.frozen, args.company, args.cutoff.isoformat())
    if len(frozen) != 30 or day_values(responses["history-only"]) != frozen or expected != frozen:
        raise ValueError("Financial RPC differs from baseline/frozen 30-day prediction")
    covered = set(scenario.calendar.groups[0].historical_operation_ids)
    residual_mean = rounded_mean(
        [
            sum(
                op.outflow_minor
                for op in request.operations
                if op.date == day.date and op.operation_id not in covered
            )
            for day in request.snapshot.history
        ]
    )
    incoming = expected[0][1]
    balance = request.snapshot.initial_balance_minor
    due = scenario.calendar.payments[0].effective_date
    for point in responses["calendar-scenario"].days:
        outgoing = residual_mean + (4_500_000 if point.date == due else 0)
        balance += incoming - outgoing
        if (point.inflow_minor, point.outflow_minor, point.reference_balance_minor) != (
            incoming,
            outgoing,
            balance,
        ):
            raise ValueError("Known payment was not counted exactly once after replacement")
    for old, new in zip(responses["calendar-scenario"].days, responses["remaining-updated"].days):
        if new.reference_balance_minor - old.reference_balance_minor != (
            500_000 if new.date >= due else 0
        ):
            raise ValueError("Remaining-payment edit applied more/less than once")
    args.output.mkdir(parents=True, exist_ok=False)
    for name in requests:
        for kind, message in (("request", requests[name]), ("response", responses[name])):
            (args.output / f"{name}-{kind}.json").write_text(
                MessageToJson(message, preserving_proto_field_name=True, ensure_ascii=False),
                encoding="utf-8",
            )
    summary = {
        "company": args.company,
        "cutoff": args.cutoff.isoformat(),
        "points": 30,
        "operations": len(request.operations),
        "past_patterns": patterns,
        "baseline_and_frozen_exact_match": True,
        "initial_balance_minor": request.snapshot.initial_balance_minor,
        "baseline_closing_minor": responses["history-only"].days[-1].reference_balance_minor,
        "baseline_first_negative": verify_response(request, responses["history-only"]),
        "replaced_historical_operations": len(covered),
        "scenario_remaining_minor": 4_500_000,
        "scenario_closing_minor": responses["calendar-scenario"].days[-1].reference_balance_minor,
        "updated_closing_minor": responses["remaining-updated"].days[-1].reference_balance_minor,
        "duplicate_payment_status": "INVALID_ARGUMENT",
        "repeated_response_identical": True,
        "server_process": "separate_local_child" if args.start_local_server else "external",
        "scenario_data_version": scenario.snapshot.data_version,
        "updated_data_version": updated.snapshot.data_version,
        "source_sha256": {
            name: sha256(args.data_dir / name) for name in ("all_operations.csv", "companies.csv")
        },
        "frozen_sha256": sha256(args.frozen),
        "coverage": vars(coverage),
        "limitations": list(request.availability_limitations),
        "scenario_is_historical_calendar_fact": False,
        "scenario_business_date": args.cutoff.isoformat(),
        "currency": "RUB_adapter_convention_not_dataset_metadata",
        "uuid_policy": "fixture_UUIDv5_and_version_1_not_production_Core_IDs",
        "gap_probability": None,
        "gap_probability_requirement_open": True,
        "quality_improvement_claimed": False,
    }
    summary["fixture_sha256"] = {
        p.name: hashlib.sha256(p.read_bytes()).hexdigest()
        for p in sorted(args.output.glob("*.json"))
    }
    (args.output / "summary.json").write_text(
        json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8"
    )
    print(json.dumps(summary, ensure_ascii=True))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    target = parser.add_mutually_exclusive_group(required=True)
    target.add_argument("--start-local-server", action="store_true")
    target.add_argument("--target")
    parser.add_argument("--insecure-local", action="store_true")
    parser.add_argument("--ca", type=Path)
    parser.add_argument("--certificate", type=Path)
    parser.add_argument("--private-key", type=Path)
    parser.add_argument("--data-dir", type=Path, default=ROOT / "data/cashflow/raw")
    parser.add_argument("--company", default="C089")
    parser.add_argument("--cutoff", type=date.fromisoformat, default=date(2026, 5, 26))
    parser.add_argument(
        "--frozen", type=Path, default=SERVICE / "artifacts/baseline_43d7a28/predictions.csv"
    )
    parser.add_argument("--output", type=Path, default=SERVICE / "artifacts/core_rpc_C089_20260526")
    args = parser.parse_args()
    tls = (args.ca, args.certificate, args.private_key)
    if (args.start_local_server or args.insecure_local) and any(tls):
        parser.error("Choose local insecure fixture mode or mTLS credentials")
    if not (args.start_local_server or args.insecure_local) and not all(tls):
        parser.error("External endpoint requires CA and Core client certificate/private key")
    if args.output.exists():
        parser.error(
            "Output already exists; choose a new directory to preserve the previous capture"
        )
    if args.start_local_server:
        with local_server() as address:
            run(address, args)
    else:
        run(args.target, args)


if __name__ == "__main__":
    main()
