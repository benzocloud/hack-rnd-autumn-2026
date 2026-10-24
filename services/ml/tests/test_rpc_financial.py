"""Known-calendar replacement, exact components and real financial RPC."""

import unittest
from collections import defaultdict
from datetime import date, timedelta
from uuid import UUID

from test_rpc import RPC_READY, make_request

if RPC_READY:
    import grpc
    from cashflow_rpc.financial import (
        FINANCIAL_PREPROCESSING,
        FINANCIAL_VERSION,
        financial_forecast,
    )
    from cashflow_rpc.inference import InvalidSnapshot
    from cashflow_rpc.server import create_server
    from ml.v1 import forecast_pb2 as pb
    from ml.v1 import forecast_pb2_grpc as frpc


def financial_request():
    base = make_request(incoming=0, outgoing=100, initial=1000)
    request = pb.PredictRequest(snapshot=base.snapshot, horizon_days=30, model=base.model)
    request.model.model_version = FINANCIAL_VERSION
    request.model.preprocessing_version = FINANCIAL_PREPROCESSING
    ids = []
    for i, day in enumerate(base.snapshot.history):
        identifier = str(UUID(int=i + 1))
        ids.append(identifier)
        request.operations.add(
            operation_id=identifier,
            date=day.date,
            outflow_minor=100,
            category_code="salary",
            category_source="manual",
            version=1,
            regularity_key="payroll-series",
        )
    request.calendar.snapshot_id = str(UUID(int=1000))
    request.calendar.data_version = 1
    request.calendar.as_of_business_date = base.snapshot.cutoff_date
    request.calendar.knowledge_mode = pb.CALENDAR_KNOWLEDGE_MODE_CURRENT
    request.calendar.groups.add(
        group_id="payroll", historical_operation_ids=ids, complete_for_horizon=True
    )
    due = (date.fromisoformat(base.snapshot.cutoff_date) + timedelta(days=5)).isoformat()
    request.calendar.payments.add(
        payment_id=str(UUID(int=2000)),
        version=1,
        effective_date=due,
        original_due_date=due,
        remaining_outflow_minor=450,
        category_code="salary",
        group_id="payroll",
        source="manual",
        regularity_key="payroll-series",
    )
    return request


@unittest.skipUnless(RPC_READY, "Install RPC requirements and generate both ML protos")
class FinancialTests(unittest.TestCase):
    def test_calendar_replaces_history_instead_of_double_counting(self):
        request = financial_request()
        result = financial_forecast(request, pb)
        self.assertEqual(len(result.days), 30)
        self.assertEqual(sum(p.outflow_minor for p in result.days), 450)
        self.assertEqual(result.days[-1].reference_balance_minor, 550)
        self.assertEqual(len(result.replaced_historical_operation_ids), 90)
        self.assertEqual(result.classified_operations, request.operations)
        self.assertEqual(result.calendar_data_version, 1)
        self.assertFalse(result.uncertainty.calibrated)
        self.assertEqual(len(result.uncertainty.bands), 30)
        self.assertIn("REQUIREMENT_OPEN", result.uncertainty.gap_probability_unavailable_reason)
        for point, band in zip(result.days, result.uncertainty.bands):
            self.assertEqual(point.reference_balance_minor, band.lower_minor)
            self.assertEqual(band.lower_minor, band.upper_minor)

    def test_partial_remaining_and_cancelled_group(self):
        request = financial_request()
        request.calendar.payments[0].remaining_outflow_minor = 200
        self.assertEqual(financial_forecast(request, pb).days[-1].reference_balance_minor, 800)
        request.calendar.ClearField("payments")
        self.assertEqual(financial_forecast(request, pb).days[-1].reference_balance_minor, 1000)

    def test_additional_obligation_leaves_baseline_history_intact(self):
        request = financial_request()
        group = request.calendar.groups[0]
        group.ClearField("historical_operation_ids")
        group.additional_obligation_confirmed = True
        result = financial_forecast(request, pb)
        self.assertEqual(result.days[-1].reference_balance_minor, -2450)
        self.assertEqual(len(result.replaced_historical_operation_ids), 0)
        components = defaultdict(lambda: [0, 0])
        for component in result.components:
            components[component.date][0] += component.inflow_minor
            components[component.date][1] += component.outflow_minor
        for point in result.days:
            self.assertEqual(components[point.date], [point.inflow_minor, point.outflow_minor])

    def test_duplicate_calendar_and_ambiguous_replacements_rejected(self):
        def duplicate_payment(r):
            r.calendar.payments.add().CopyFrom(r.calendar.payments[0])

        def overlapping_group(r):
            copy = r.calendar.groups.add()
            copy.CopyFrom(r.calendar.groups[0])
            copy.group_id = "overlap"

        mutations = [
            duplicate_payment,
            overlapping_group,
            lambda r: setattr(r.calendar.groups[0], "complete_for_horizon", False),
            lambda r: r.calendar.groups[0].historical_operation_ids.append(str(UUID(int=9999))),
            lambda r: r.calendar.groups[0].ClearField("historical_operation_ids"),
            lambda r: setattr(r.calendar.groups[0], "additional_obligation_confirmed", True),
            lambda r: setattr(r.calendar.payments[0], "version", 0),
            lambda r: setattr(r.calendar.payments[0], "group_id", "missing"),
        ]
        for i, mutate in enumerate(mutations):
            with self.subTest(case=i):
                request = financial_request()
                mutate(request)
                with self.assertRaises(InvalidSnapshot):
                    financial_forecast(request, pb)

    def test_future_knowledge_and_missing_coverage_rejected(self):
        mutations = [
            lambda r: setattr(r.calendar, "as_of_business_date", "2026-10-25"),
            lambda r: setattr(r.calendar, "data_version", 2),
            lambda r: setattr(r.operations[0], "date", "2026-10-24"),
            lambda r: r.operations.pop(),
            lambda r: setattr(r.operations[1], "operation_id", r.operations[0].operation_id),
            lambda r: setattr(r.operations[0], "category_source", ""),
            lambda r: setattr(r.calendar.payments[0], "effective_date", "2026-12-01"),
            lambda r: setattr(r.calendar.payments[0], "remaining_outflow_minor", 0),
            lambda r: setattr(r, "horizon_days", 1),
            lambda r: r.ClearField("calendar"),
        ]
        for i, mutate in enumerate(mutations):
            with self.subTest(case=i):
                request = financial_request()
                mutate(request)
                with self.assertRaises(InvalidSnapshot):
                    financial_forecast(request, pb)

    def test_no_calendar_matches_legacy_baseline_and_deterministic_bands(self):
        from cashflow_rpc.inference import MODEL_VERSION, PREPROCESSING_VERSION, forecast

        request = financial_request()
        request.calendar.ClearField("groups")
        request.calendar.ClearField("payments")
        legacy = pb.PredictRequest(snapshot=request.snapshot, horizon_days=30, model=request.model)
        legacy.model.model_version = MODEL_VERSION
        legacy.model.preprocessing_version = PREPROCESSING_VERSION
        result = financial_forecast(request, pb)
        self.assertEqual(
            [
                (d.date, d.inflow_minor, d.outflow_minor, d.reference_balance_minor)
                for d in result.days
            ],
            [
                (d.date, d.inflow_minor, d.outflow_minor, d.reference_balance_minor)
                for d in forecast(legacy, pb).days
            ],
        )
        self.assertEqual(result, financial_forecast(request, pb))
        request.operations[0].category_code = "another_category"
        split = financial_forecast(request, pb)
        self.assertEqual(split.days, result.days)
        self.assertTrue(all(len(component.component_id) <= 128 for component in split.components))
        evidence = {item.evidence_id for item in split.history_evidence}
        self.assertTrue(
            all(set(component.source_ids) <= evidence for component in split.components)
        )

    def test_one_rpc_uses_explicit_version_not_calendar_presence(self):
        server, port = create_server(port=0, insecure_local=True)
        server.start()
        try:
            with grpc.insecure_channel(f"127.0.0.1:{port}") as channel:
                stub = frpc.ForecastServiceStub(channel)
                request = financial_request()
                request.calendar.ClearField("groups")
                request.calendar.ClearField("payments")
                result = stub.Predict(request, timeout=3)
                self.assertEqual(result.model.model_version, FINANCIAL_VERSION)
                self.assertFalse(result.history_only_includes_recurring_flows)
                request.ClearField("calendar")
                with self.assertRaises(grpc.RpcError) as error:
                    stub.Predict(request, timeout=3)
                self.assertEqual(error.exception.code(), grpc.StatusCode.INVALID_ARGUMENT)
                request.model.model_version = "unknown"
                with self.assertRaises(grpc.RpcError) as error:
                    stub.Predict(request, timeout=3)
                self.assertEqual(error.exception.code(), grpc.StatusCode.FAILED_PRECONDITION)
        finally:
            server.stop(grace=0).wait(timeout=3)

    def test_real_financial_rpc(self):
        server, port = create_server(port=0, insecure_local=True)
        server.start()
        try:
            with grpc.insecure_channel(f"127.0.0.1:{port}") as channel:
                response = frpc.ForecastServiceStub(channel).Predict(financial_request(), timeout=3)
                self.assertEqual(response.days[-1].reference_balance_minor, 550)
                self.assertEqual(response, financial_forecast(financial_request(), pb))
        finally:
            server.stop(grace=0).wait(timeout=3)

    def test_category_uuid_scope_and_metadata_are_preserved(self):
        request = financial_request()
        request.calendar.ClearField("groups")
        request.calendar.ClearField("payments")
        for op in request.operations:
            op.category_id = str(UUID(int=9999))
            op.category_company_id = request.snapshot.company_id
            op.counterparty_inn = "0012345678"
            op.payment_purpose = "Исходное назначение"
        result = financial_forecast(request, pb)
        self.assertEqual(result.classified_operations, request.operations)
        self.assertTrue(all(c.category_id == str(UUID(int=9999)) for c in result.components))
        request.operations[0].category_company_id = str(UUID(int=7777))
        with self.assertRaises(InvalidSnapshot):
            financial_forecast(request, pb)

    def test_live_calendar_can_follow_yesterday_eod_but_not_historical_validation(self):
        request = financial_request()
        request.calendar.as_of_business_date = "2026-10-24"
        request.calendar.no_post_cutoff_movements = True
        self.assertEqual(len(financial_forecast(request, pb).days), 30)
        request.calendar.knowledge_mode = pb.CALENDAR_KNOWLEDGE_MODE_HISTORICAL_AS_OF_CERTIFIED
        with self.assertRaises(InvalidSnapshot):
            financial_forecast(request, pb)
        request.calendar.as_of_business_date = request.snapshot.cutoff_date
        request.availability_limitations.append("historical_availability_unverified")
        with self.assertRaises(InvalidSnapshot):
            financial_forecast(request, pb)

    def test_next_day_remaining_does_not_silently_omit_post_cutoff_settlements(self):
        request = financial_request()
        request.calendar.as_of_business_date = "2026-10-24"
        # An empty group could mean salary was paid today, while initial EOD
        # still precedes that debit. Do not treat it as zero future cash flow.
        request.calendar.ClearField("payments")
        with self.assertRaises(InvalidSnapshot):
            financial_forecast(request, pb)
        request.calendar.no_post_cutoff_movements = True
        self.assertEqual(len(financial_forecast(request, pb).days), 30)

    def test_reconstructed_covered_days_are_not_observed_eod(self):
        request = financial_request()
        for day in request.snapshot.history[:-1]:
            day.source = pb.DAY_SOURCE_RECONSTRUCTED_COVERED
        self.assertEqual(len(financial_forecast(request, pb).days), 30)
        request.snapshot.history[-1].source = pb.DAY_SOURCE_RECONSTRUCTED_COVERED
        with self.assertRaises(InvalidSnapshot):
            financial_forecast(request, pb)

    def test_joint_bootstrap_keeps_flow_dependence_and_fixed_calendar(self):
        from cashflow_rpc.financial import empirical_bands

        # Perfectly paired inflow/outflow must never create a fictitious net risk.
        history = [(i * 100, i * 100) for i in range(90)]
        known = [[0, 0] for _ in range(30)]
        self.assertEqual(empirical_bands(history, known, 123, 30), [(123, 123)] * 30)
        history = [(i % 7 * 100, (i % 5) * 20) for i in range(90)]
        original = empirical_bands(history, known, 123, 30)
        self.assertEqual(original, empirical_bands(history, known, 123, 30))
        known[4][1] = 450
        shifted = empirical_bands(history, known, 123, 30)
        self.assertEqual(shifted[:4], original[:4])
        self.assertEqual(shifted[4:], [(low - 450, high - 450) for low, high in original[4:]])


if __name__ == "__main__":
    unittest.main()
