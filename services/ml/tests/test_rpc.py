"""Snapshot boundary and real transport tests; optional RPC dependencies."""

import importlib.util
import sys
import unittest
from datetime import date, timedelta
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

try:
    RPC_READY = all(importlib.util.find_spec(p) is not None for p in ("grpc", "ml.v1.forecast_pb2"))
except ModuleNotFoundError:
    RPC_READY = False

if RPC_READY:
    import grpc
    from cashflow_rpc.inference import (
        MODEL_NAME,
        MODEL_VERSION,
        PREPROCESSING_VERSION,
        InvalidSnapshot,
        UnsupportedContext,
        forecast,
    )
    from cashflow_rpc.server import create_server
    from ml.v1 import forecast_pb2 as pb
    from ml.v1 import forecast_pb2_grpc as rpc


def make_request(incoming=100, outgoing=100, initial=0, horizon=30):
    cutoff = date(2026, 10, 23)
    request = pb.PredictRequest(horizon_days=horizon)
    request.model.model_name = MODEL_NAME
    request.model.model_version = MODEL_VERSION
    request.model.preprocessing_version = PREPROCESSING_VERSION
    s = request.snapshot
    s.snapshot_id = "019a1234-0000-7000-8000-000000000001"
    s.company_id = "019a1234-0000-7000-8000-000000000002"
    s.account_id = "019a1234-0000-7000-8000-000000000003"
    s.data_version = 1
    s.currency = "RUB"
    s.cutoff_date = cutoff.isoformat()
    s.initial_balance_minor = initial
    balance = initial - 90 * (incoming - outgoing)
    for step in range(90):
        balance += incoming - outgoing
        s.history.add(
            date=(cutoff - timedelta(days=89 - step)).isoformat(),
            inflow_minor=incoming,
            outflow_minor=outgoing,
            closing_balance_minor=balance,
            source=pb.DAY_SOURCE_OBSERVED,
        )
    return request


@unittest.skipUnless(RPC_READY, "Install RPC requirements and run generate_rpc.py")
class SnapshotTests(unittest.TestCase):
    def test_zero_balance_presence_and_exact_deterministic_forecast(self):
        request = make_request(incoming=3, outgoing=5)
        result = forecast(request, pb)
        self.assertTrue(request.snapshot.HasField("initial_balance_minor"))
        self.assertEqual(
            result.SerializeToString(deterministic=True),
            forecast(request, pb).SerializeToString(deterministic=True),
        )
        self.assertEqual(len(result.days), 30)
        self.assertEqual(result.first_negative_balance_date, "2026-10-24")
        self.assertEqual(result.days[-1].reference_balance_minor, -60)
        self.assertTrue(all(d.inflow_minor == 3 and d.outflow_minor == 5 for d in result.days))
        self.assertTrue(result.history_only_includes_recurring_flows)

    def test_no_gap_is_absent_not_empty_probability(self):
        result = forecast(make_request(), pb)
        self.assertFalse(result.HasField("first_negative_balance_date"))
        self.assertEqual(result.gap_probability.WhichOneof("estimate"), "unavailable_reason")
        self.assertEqual(result.gap_probability.unavailable_reason, "not_estimated")
        self.assertTrue(
            all(
                day.negative_balance_probability.WhichOneof("estimate") == "unavailable_reason"
                and day.negative_balance_probability.unavailable_reason == "not_estimated"
                for day in result.days
            )
        )

    def test_invalid_coverage_dates_and_amounts(self):
        mutations = [
            lambda r: r.snapshot.ClearField("initial_balance_minor"),
            lambda r: r.snapshot.history[0].ClearField("inflow_minor"),
            lambda r: setattr(r.snapshot.history[0], "inflow_minor", -1),
            lambda r: setattr(r.snapshot.history[0], "source", 0),
            lambda r: setattr(
                r.snapshot.history[0], "source", pb.DAY_SOURCE_VERIFIED_NO_OPERATIONS
            ),
            lambda r: setattr(r.snapshot.history[0], "date", "2026-10-24"),
            lambda r: setattr(r.snapshot, "cutoff_date", "20261023"),
            lambda r: setattr(r.snapshot, "company_id", "C001"),
            lambda r: setattr(r.snapshot, "company_id", "00000000-0000-0000-0000-000000000000"),
            lambda r: setattr(r.snapshot, "data_version", 0),
            lambda r: setattr(r.snapshot, "currency", "USD"),
            lambda r: setattr(r.snapshot.history[-1], "closing_balance_minor", 100),
            lambda r: r.snapshot.history.pop(),
            lambda r: setattr(r, "horizon_days", 31),
            lambda r: setattr(r, "horizon_days", 0),
            lambda r: r.ClearField("snapshot"),
            lambda r: r.ClearField("model"),
        ]
        for index, mutate in enumerate(mutations):
            with self.subTest(case=index):
                request = make_request()
                mutate(request)
                with self.assertRaises(InvalidSnapshot):
                    forecast(request, pb)

    def test_calendar_and_unknown_model_rejected(self):
        request = make_request()
        request.snapshot.known_payments.add(payment_id="future")
        with self.assertRaises(UnsupportedContext):
            forecast(request, pb)
        for field in ("model_name", "model_version", "preprocessing_version"):
            request = make_request()
            setattr(request.model, field, "unavailable")
            with self.assertRaises(UnsupportedContext):
                forecast(request, pb)

    def test_reconciliation_warning_and_no_operations(self):
        request = make_request()
        request.snapshot.history[20].closing_balance_minor += 1
        self.assertIn(
            "OBSERVED_RECONCILIATION_WITHIN_ONE_MINOR_UNIT", forecast(request, pb).warnings
        )
        request.snapshot.history[20].closing_balance_minor += 1
        with self.assertRaises(InvalidSnapshot):
            forecast(request, pb)
        request = make_request(incoming=0, outgoing=0)
        for raw in request.snapshot.history[:-1]:
            raw.source = pb.DAY_SOURCE_VERIFIED_NO_OPERATIONS
        self.assertEqual(forecast(request, pb).days[-1].reference_balance_minor, 0)
        request.snapshot.history[-1].source = pb.DAY_SOURCE_VERIFIED_NO_OPERATIONS
        with self.assertRaises(InvalidSnapshot):
            forecast(request, pb)

    def test_int64_overflow_rejected_before_serialization(self):
        request = make_request(incoming=1, outgoing=0, initial=2**63 - 1)
        with self.assertRaises(InvalidSnapshot):
            forecast(request, pb)

    def test_rounded_mean_matches_existing_baseline(self):
        from cashflow_baseline.data import Daily
        from cashflow_baseline.models import predict

        request = make_request()
        # Different inflows and balance paths with a non-integral daily mean.
        balance = -1000
        for index, raw in enumerate(request.snapshot.history):
            raw.inflow_minor = index % 7
            raw.outflow_minor = index % 5
            balance += raw.inflow_minor - raw.outflow_minor
            raw.closing_balance_minor = balance
        request.snapshot.initial_balance_minor = balance
        history = [
            Daily(
                date.fromisoformat(r.date),
                r.inflow_minor,
                r.outflow_minor,
                r.closing_balance_minor,
                True,
                False,
                0,
            )
            for r in request.snapshot.history
        ]
        for horizon in (1, 7, 30):
            request.horizon_days = horizon
            actual = forecast(request, pb)
            expected = predict(history, horizon, "mean_90")
            self.assertEqual(
                [(d.inflow_minor, d.outflow_minor, d.reference_balance_minor) for d in actual.days],
                [(d.inflow, d.outflow, d.balance) for d in expected],
            )


@unittest.skipUnless(RPC_READY, "Install RPC requirements and run generate_rpc.py")
class TransportTests(unittest.TestCase):
    def setUp(self):
        self.server, port = create_server(port=0, insecure_local=True)
        self.server.start()
        self.channel = grpc.insecure_channel(f"127.0.0.1:{port}")
        grpc.channel_ready_future(self.channel).result(timeout=3)
        self.stub = rpc.ForecastServiceStub(self.channel)

    def tearDown(self):
        self.channel.close()
        self.server.stop(grace=0).wait(timeout=3)

    def test_real_rpc_and_wire_identity(self):
        request = make_request(incoming=3, outgoing=5)
        result = self.stub.Predict(request, timeout=3)
        self.assertEqual(result, forecast(request, pb))

    def test_statuses_deadline_and_limits(self):
        invalid = make_request()
        invalid.snapshot.ClearField("initial_balance_minor")
        unsupported = make_request()
        unsupported.model.model_version = "missing"
        huge = make_request()
        huge.model.model_version = "x" * (3 * 1024 * 1024)
        for request, timeout, code in [
            (invalid, 3, grpc.StatusCode.INVALID_ARGUMENT),
            (unsupported, 3, grpc.StatusCode.FAILED_PRECONDITION),
            (make_request(), None, grpc.StatusCode.INVALID_ARGUMENT),
            (make_request(), 60, grpc.StatusCode.INVALID_ARGUMENT),
            (make_request(), 30.05, grpc.StatusCode.INVALID_ARGUMENT),
            (huge, 3, grpc.StatusCode.RESOURCE_EXHAUSTED),
        ]:
            with (
                self.subTest(code=code, timeout=timeout),
                self.assertRaises(grpc.RpcError) as error,
            ):
                self.stub.Predict(request, timeout=timeout)
            self.assertEqual(error.exception.code(), code)

    def test_financial_context_never_falls_back_to_baseline(self):
        variants = [
            lambda r: setattr(r.model, "configuration_id", "financial_v1"),
            lambda r: setattr(r.model, "random_seed", "0"),
            lambda r: setattr(r.model, "trajectory_count", 100),
            lambda r: r.operations.add(operation_id="fact"),
            lambda r: r.calendar.groups.add(group_id="salary"),
            lambda r: r.availability_limitations.append("unavailable"),
            lambda r: r.snapshot.history[0].quality_codes.append("INCOMPLETE_COVERAGE"),
        ]
        for change in variants:
            with self.subTest(change=change):
                request = make_request()
                change(request)
                with self.assertRaises(grpc.RpcError) as error:
                    self.stub.Predict(request, timeout=3)
                self.assertEqual(error.exception.code(), grpc.StatusCode.FAILED_PRECONDITION)

    def test_insecure_mode_cannot_bind_external_address(self):
        with self.assertRaises(ValueError):
            create_server(host="0.0.0.0", insecure_local=True)
        with self.assertRaises(ValueError):
            create_server()


if __name__ == "__main__":
    unittest.main()
