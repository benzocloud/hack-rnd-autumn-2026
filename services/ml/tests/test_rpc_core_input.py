"""Core reference normalization and consumer checks, without participant dataset."""

import unittest
from dataclasses import replace
from datetime import date, timedelta

from test_rpc import RPC_READY

if RPC_READY:
    import grpc
    from cashflow_rpc.core_input import Coverage, prepare_financial_request, verify_response
    from cashflow_rpc.financial import financial_forecast
    from cashflow_rpc.inference import InvalidSnapshot
    from cashflow_rpc.server import create_server
    from google.protobuf.json_format import MessageToJson, Parse
    from ml.v1 import financial_pb2 as fpb
    from ml.v1 import financial_pb2_grpc as rpc
    from ml.v1 import forecast_pb2 as pb
    from test_rpc_financial import financial_request


def core_request(with_calendar=False):
    original = financial_request()
    snapshot = pb.ForecastSnapshot()
    snapshot.CopyFrom(original.snapshot)
    snapshot.ClearField("history")
    cutoff = date.fromisoformat(snapshot.cutoff_date)
    coverage = Coverage(
        (cutoff - timedelta(days=89)).isoformat(), cutoff.isoformat(), "test-import-receipt"
    )
    return prepare_financial_request(
        snapshot,
        original.operations,
        coverage,
        pb,
        fpb,
        calendar=original.calendar if with_calendar else None,
        limitations=("historical_availability_unverified",),
    )


@unittest.skipUnless(RPC_READY, "Install RPC requirements and generate both ML protos")
class CoreInputTests(unittest.TestCase):
    def test_backwards_balances_require_coverage_and_keep_only_cutoff_observed(self):
        request = core_request()
        self.assertEqual(len(request.snapshot.history), 90)
        self.assertEqual(request.snapshot.history[0].closing_balance_minor, 9900)
        self.assertEqual(request.snapshot.history[-1].closing_balance_minor, 1000)
        self.assertTrue(
            all(
                d.source == pb.DAY_SOURCE_RECONSTRUCTED_COVERED
                for d in request.snapshot.history[:-1]
            )
        )
        self.assertEqual(request.snapshot.history[-1].source, pb.DAY_SOURCE_OBSERVED)
        self.assertEqual(request.calendar.knowledge_mode, fpb.CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE)
        self.assertEqual(request.calendar.as_of_business_date, "")
        result = financial_forecast(request, pb, fpb)
        self.assertEqual(result.forecast.days[-1].reference_balance_minor, -2000)
        self.assertTrue(result.forecast.history_only_includes_recurring_flows)
        self.assertIn("HISTORY_ONLY_NO_CALENDAR", result.forecast.warnings)
        self.assertNotIn("EXPLICIT_COMPLETE_CALENDAR_REPLACEMENT", result.forecast.warnings)
        self.assertEqual(
            verify_response(request, result), result.forecast.first_negative_balance_date
        )

    def test_missing_coverage_future_facts_and_duplicates_rejected(self):
        source = financial_request()
        source.snapshot.ClearField("history")
        cutoff = date.fromisoformat(source.snapshot.cutoff_date)
        coverage = Coverage(
            (cutoff - timedelta(days=89)).isoformat(), cutoff.isoformat(), "import-v1"
        )
        for invalid in (
            None,
            replace(coverage, reference=""),
            replace(coverage, start=cutoff.isoformat()),
            replace(coverage, end=(cutoff + timedelta(days=1)).isoformat()),
        ):
            with self.subTest(coverage=invalid), self.assertRaises(InvalidSnapshot):
                prepare_financial_request(source.snapshot, source.operations, invalid, pb, fpb)
        facts = list(source.operations)
        with self.assertRaises(InvalidSnapshot):
            prepare_financial_request(source.snapshot, facts + [facts[0]], coverage, pb, fpb)
        facts[0].date = (cutoff + timedelta(days=1)).isoformat()
        with self.assertRaises(InvalidSnapshot):
            prepare_financial_request(source.snapshot, facts, coverage, pb, fpb)

    def test_unavailable_calendar_never_accepts_schedule_or_invented_date(self):
        mutations = [
            lambda r: setattr(r.calendar, "as_of_business_date", r.snapshot.cutoff_date),
            lambda r: r.calendar.payments.add(payment_id="invented"),
            lambda r: r.calendar.groups.add(group_id="invented"),
            lambda r: setattr(r.calendar, "snapshot_id", financial_request().calendar.snapshot_id),
            lambda r: r.ClearField("availability_limitations"),
        ]
        for mutate in mutations:
            request = core_request()
            mutate(request)
            with self.assertRaises(InvalidSnapshot):
                financial_forecast(request, pb, fpb)

    def test_live_calendar_remains_available_and_is_not_overwritten_by_fallback(self):
        request = core_request(with_calendar=True)
        self.assertEqual(request.calendar.knowledge_mode, fpb.CALENDAR_KNOWLEDGE_MODE_CURRENT)
        response = financial_forecast(request, pb, fpb)
        self.assertEqual(sum(d.outflow_minor for d in response.forecast.days), 450)
        self.assertNotIn("historical_calendar_unavailable", response.availability_limitations)
        self.assertIsNone(verify_response(request, response))

    def test_consumer_rejects_wrong_revision_money_dates_and_missing_payments(self):
        mutations = [
            lambda r: setattr(r.forecast, "data_version", 2),
            lambda r: setattr(r.forecast, "start_date", "2026-01-01"),
            lambda r: setattr(r.forecast.days[0], "reference_balance_minor", 0),
            lambda r: setattr(r.uncertainty.bands[0], "date", "2026-01-01"),
            lambda r: r.ClearField("availability_limitations"),
            lambda r: r.ClearField("replaced_historical_operation_ids"),
            lambda r: r.ClearField("components"),
            lambda r: setattr(r.classified_operations[0], "regularity_key", "changed"),
            lambda r: setattr(r, "calendar_data_version", 99),
        ]
        request = core_request(with_calendar=True)
        for mutate in mutations:
            response = financial_forecast(request, pb, fpb)
            mutate(response)
            with self.assertRaises(InvalidSnapshot):
                verify_response(request, response)

    def test_consumer_rejects_unknown_or_duplicated_history_evidence(self):
        request = core_request()
        response = financial_forecast(request, pb, fpb)
        response.history_evidence[0].historical_operation_ids.append("invented")
        with self.assertRaises(InvalidSnapshot):
            verify_response(request, response)

    def test_canonical_json_preserves_money_above_javascript_integer_precision(self):
        request = core_request()
        large = 2**53 + 1
        request.snapshot.initial_balance_minor = large
        encoded = MessageToJson(request, preserving_proto_field_name=True)
        self.assertIn('"initial_balance_minor": "9007199254740993"', encoded)
        self.assertEqual(
            Parse(encoded, fpb.PredictFinancialRequest()).snapshot.initial_balance_minor, large
        )

    def test_prepared_core_request_crosses_actual_rpc(self):
        request = core_request()
        server, port = create_server(port=0, insecure_local=True)
        server.start()
        try:
            with grpc.insecure_channel(f"127.0.0.1:{port}") as channel:
                result = rpc.FinancialForecastServiceStub(channel).PredictFinancial(
                    request, timeout=3
                )
                self.assertEqual(result, financial_forecast(request, pb, fpb))
                verify_response(request, result)
        finally:
            server.stop(0).wait(timeout=3)
