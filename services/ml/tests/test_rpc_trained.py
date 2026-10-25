"""New pinned model through the existing financial RPC, including exact lifecycle."""

import hashlib
import unittest
from datetime import date, timedelta
from unittest.mock import patch
from uuid import UUID

from test_rpc import RPC_READY
from test_rpc_financial import financial_request

if RPC_READY:
    import grpc
    from cashflow_rpc import trained_pool
    from cashflow_rpc.financial import financial_forecast
    from cashflow_rpc.inference import InvalidSnapshot, UnsupportedContext
    from cashflow_rpc.server import create_server
    from ml.v1 import forecast_pb2 as pb
    from ml.v1 import forecast_pb2_grpc as rpc


def trained_request():
    request = financial_request()
    _, checksum = trained_pool.load_artifact()
    request.model.model_name = trained_pool.MODEL_NAME
    request.model.model_version = trained_pool.MODEL_VERSION
    request.model.preprocessing_version = trained_pool.PREPROCESSING_VERSION
    request.model.configuration_id = trained_pool.configuration_id(checksum)
    request.model.random_seed = str(trained_pool.SEED)
    request.model.trajectory_count = 512
    return request


def unavailable(request):
    request.calendar.Clear()
    request.calendar.snapshot_id = request.snapshot.snapshot_id
    request.calendar.data_version = request.snapshot.data_version
    request.calendar.knowledge_mode = pb.CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE
    request.availability_limitations.append("historical_calendar_unavailable")


@unittest.skipUnless(RPC_READY, "Generate bindings and install RPC requirements")
class TrainedFinancialTests(unittest.TestCase):
    def test_pinned_context_exact_components_and_unavailable_probability(self):
        request = trained_request()
        with patch.object(
            trained_pool,
            "predict_allocations",
            side_effect=AssertionError("No redundant feature maps at the fitted corner"),
        ):
            result = financial_forecast(request, pb)
        self.assertEqual(result.model, request.model)
        self.assertEqual(result.snapshot_id, request.snapshot.snapshot_id)
        self.assertEqual(result.calendar_snapshot_id, request.calendar.snapshot_id)
        self.assertEqual(result.classified_operations, request.operations)
        self.assertEqual(len(result.days), 30)
        self.assertEqual(sum(d.outflow_minor for d in result.days), 450)
        self.assertEqual(result.days[-1].reference_balance_minor, 550)
        self.assertFalse(result.uncertainty.calibrated)
        self.assertEqual(result.gap_probability.unavailable_reason, "not_calibrated")
        for point in result.days:
            self.assertEqual(
                point.negative_balance_probability.unavailable_reason, "not_calibrated"
            )
            parts = [c for c in result.components if c.date == point.date]
            self.assertEqual(sum(c.inflow_minor for c in parts), point.inflow_minor)
            self.assertEqual(sum(c.outflow_minor for c in parts), point.outflow_minor)

    def test_actual_partial_settlement_conserves_total_day_by_day(self):
        before = trained_request()
        expected = financial_forecast(before, pb)
        paid = trained_request()
        paid_id = str(UUID(int=4000))
        paid.operations.add(
            operation_id=paid_id,
            date=paid.snapshot.cutoff_date,
            outflow_minor=250,
            category_code="salary",
            category_source="manual",
            version=1,
        )
        paid.snapshot.history[-1].outflow_minor += 250
        paid.snapshot.history[-1].closing_balance_minor -= 250
        paid.snapshot.initial_balance_minor -= 250
        paid.calendar.groups[0].historical_operation_ids.append(paid_id)
        paid.calendar.payments[0].remaining_outflow_minor = 200
        actual = financial_forecast(paid, pb)
        for i, (a, b) in enumerate(zip(actual.days, expected.days, strict=True)):
            self.assertEqual(
                a.reference_balance_minor, b.reference_balance_minor - (250 if i < 4 else 0)
            )
        self.assertEqual(actual.days[-1].reference_balance_minor, 550)
        self.assertIn(paid_id, actual.replaced_historical_operation_ids)
        paid.calendar.ClearField("payments")
        cancelled = financial_forecast(paid, pb)
        self.assertTrue(all(d.reference_balance_minor == 750 for d in cancelled.days))
        self.assertIn(paid_id, {op.operation_id for op in cancelled.classified_operations})
        self.assertIn(paid_id, cancelled.replaced_historical_operation_ids)

    def test_transfer_cancellation_and_additional_payment(self):
        request = trained_request()
        request.calendar.payments[0].remaining_outflow_minor = 5000
        early = financial_forecast(request, pb)
        moved = (date.fromisoformat(request.snapshot.cutoff_date) + timedelta(days=10)).isoformat()
        request.calendar.payments[0].effective_date = moved
        late = financial_forecast(request, pb)
        self.assertEqual(late.first_negative_balance_date, moved)
        self.assertNotEqual(early.first_negative_balance_date, moved)
        self.assertEqual(
            early.days[-1].reference_balance_minor, late.days[-1].reference_balance_minor
        )
        request.calendar.ClearField("payments")
        self.assertTrue(
            all(d.reference_balance_minor == 1000 for d in financial_forecast(request, pb).days)
        )
        additional = trained_request()
        additional.calendar.groups[0].ClearField("historical_operation_ids")
        additional.calendar.groups[0].additional_obligation_confirmed = True
        result = financial_forecast(additional, pb)
        self.assertEqual(result.days[-1].reference_balance_minor, -2450)
        evidence = {v.evidence_id for v in result.history_evidence}
        residual = [c for c in result.components if c.kind == "linear_pool_uncovered"]
        self.assertEqual(len(residual), 30)
        self.assertTrue(all(set(c.source_ids) <= evidence for c in residual))

    def test_unavailable_calendar_is_preserved_as_unavailable(self):
        request = trained_request()
        unavailable(request)
        result = financial_forecast(request, pb)
        self.assertTrue(result.history_only_includes_recurring_flows)
        self.assertEqual(result.days[-1].reference_balance_minor, -2000)
        self.assertEqual(result.availability_limitations, request.availability_limitations)
        self.assertEqual(result.replaced_historical_operation_ids, [])

    def test_wrong_context_artifact_and_historical_unavailability_fail(self):
        for field, value in (
            ("configuration_id", ""),
            ("random_seed", "0"),
            ("model_name", "mean_90"),
        ):
            request = trained_request()
            setattr(request.model, field, value)
            with self.assertRaises(UnsupportedContext):
                financial_forecast(request, pb)
        request = trained_request()
        request.snapshot.cutoff_date = "2025-01-01"
        with self.assertRaises(UnsupportedContext):
            financial_forecast(request, pb)
        request = trained_request()
        with patch.object(
            trained_pool, "load_artifact", side_effect=UnsupportedContext("missing artifact")
        ):
            with self.assertRaises(UnsupportedContext):
                financial_forecast(request, pb)
            self.assertEqual(
                financial_forecast(financial_request(), pb).days[-1].reference_balance_minor, 550
            )

    def test_duplicate_replacement_and_int64_overflow_remain_rejected(self):
        request = trained_request()
        request.calendar.groups[0].historical_operation_ids.append(
            request.operations[0].operation_id
        )
        with self.assertRaises(InvalidSnapshot):
            financial_forecast(request, pb)
        request = trained_request()
        request.calendar.payments[0].remaining_outflow_minor = 0
        request.calendar.payments[0].remaining_inflow_minor = 2**63 - 1
        with self.assertRaises(InvalidSnapshot):
            financial_forecast(request, pb)

    def test_old_financial_profile_response_bytes_are_unchanged(self):
        expected = {
            "full": "af3f55049bd3756e9f265646dd6d57de63e0061a8f2c14ed27e6b62e383e190d",
            "partial": "1bf826b4219b23bc55f53168d4d86911e85803c3e6a61ce15d40eb503117d6d9",
            "cancelled": "030dde8dd366915123fd655191a89eea0542ea2b9fc0ea53b780ab19c8c43790",
            "additional": "417fc3c8b2b1f6de001414268ed7bb186ec9c6128d391d4a0c101fb4d3d46685",
            "unavailable": "1017fbbab3a43736e6269a95ed68ea3bf5928f23c62d674e4fbf2aa3f106ebfc",
        }
        for label, checksum in expected.items():
            request = financial_request()
            if label == "partial":
                request.calendar.payments[0].remaining_outflow_minor = 200
            elif label == "cancelled":
                request.calendar.ClearField("payments")
            elif label == "additional":
                request.calendar.groups[0].ClearField("historical_operation_ids")
                request.calendar.groups[0].additional_obligation_confirmed = True
            elif label == "unavailable":
                unavailable(request)
            response = financial_forecast(request, pb).SerializeToString(deterministic=True)
            self.assertEqual(hashlib.sha256(response).hexdigest(), checksum, label)


@unittest.skipUnless(RPC_READY, "Generate bindings and install RPC requirements")
class TrainedTransportTests(unittest.TestCase):
    def test_new_profile_over_mutual_tls_and_existing_error_mapping(self):
        from test_rpc_tls import certificate, pem

        ca = certificate("trained-test-root", ca=True)
        root, _ = pem(ca)
        server_cert, server_key = pem(certificate("localhost", ca, server=True))
        server, port = create_server(
            port=0, certificate=server_cert, private_key=server_key, client_ca=root
        )
        server.start()
        try:
            for name in ("saldo-core", "saldo-api"):
                cert, key = pem(certificate(name, ca))
                credentials = grpc.ssl_channel_credentials(
                    root, private_key=key, certificate_chain=cert
                )
                with grpc.secure_channel(f"127.0.0.1:{port}", credentials) as channel:
                    call = rpc.ForecastServiceStub(channel).Predict
                    request = trained_request()
                    if name != "saldo-core":
                        with self.assertRaises(grpc.RpcError) as error:
                            call(request, timeout=3)
                        self.assertEqual(error.exception.code(), grpc.StatusCode.PERMISSION_DENIED)
                        continue
                    self.assertEqual(call(request, timeout=3), financial_forecast(request, pb))
                    request.model.configuration_id = "wrong"
                    with self.assertRaises(grpc.RpcError) as error:
                        call(request, timeout=3)
                    self.assertEqual(error.exception.code(), grpc.StatusCode.FAILED_PRECONDITION)
                    request = trained_request()
                    request.operations.pop()
                    with self.assertRaises(grpc.RpcError) as error:
                        call(request, timeout=3)
                    self.assertEqual(error.exception.code(), grpc.StatusCode.INVALID_ARGUMENT)
        finally:
            server.stop(grace=0).wait(timeout=3)


if __name__ == "__main__":
    unittest.main()
