import unittest
from datetime import date, timedelta

from cashflow_rpc.csv_calendar import (
    CSV_CALENDAR_LIMITATIONS,
    CSV_CALENDAR_PREPROCESSING,
    CSV_CALENDAR_VERSION,
    csv_calendar_forecast,
)
from cashflow_rpc.csv_forecast import csv_forecast
from cashflow_rpc.inference import InvalidSnapshot, UnsupportedContext
from test_rpc import RPC_READY
from test_rpc_csv import csv_request

if RPC_READY:
    from ml.v1 import forecast_pb2 as pb


def csv_calendar_request():
    request = csv_request()
    request.model.model_version = CSV_CALENDAR_VERSION
    request.model.preprocessing_version = CSV_CALENDAR_PREPROCESSING
    request.model.configuration_id = CSV_CALENDAR_VERSION
    del request.availability_limitations[:]
    request.availability_limitations.extend(CSV_CALENDAR_LIMITATIONS)
    request.calendar.knowledge_mode = pb.CALENDAR_KNOWLEDGE_MODE_CURRENT
    request.calendar.as_of_business_date = request.snapshot.cutoff_date
    return request


def add_payment(request, amount=900):
    cutoff = date.fromisoformat(request.snapshot.cutoff_date)
    request.calendar.groups.add(
        group_id="019a0c65-9800-7000-8000-000000000050",
        complete_for_horizon=True,
        additional_obligation_confirmed=True,
    )
    return request.calendar.payments.add(
        payment_id="019a0c65-9800-7000-8000-000000000051",
        version=1,
        effective_date=(cutoff + timedelta(days=3)).isoformat(),
        original_due_date=(cutoff + timedelta(days=3)).isoformat(),
        remaining_outflow_minor=amount,
        category_code="unknown",
        group_id=request.calendar.groups[0].group_id,
        source="manual",
    )


@unittest.skipUnless(RPC_READY, "Install RPC requirements and generate the current ML proto")
class CsvCalendarForecastTests(unittest.TestCase):
    def test_sparse_current_lifecycle_preserves_unknown_history_and_exact_remaining(self):
        request = csv_calendar_request()
        payment = add_payment(request)
        original = request.SerializeToString()
        result = csv_calendar_forecast(request, pb)
        self.assertEqual(request.SerializeToString(), original)
        self.assertEqual(len(result.days), 30)
        self.assertEqual(result.days[-1].reference_balance_minor, 96100)
        self.assertFalse(request.snapshot.history[1].HasField("inflow_minor"))
        self.assertEqual(request.snapshot.history[1].source, pb.DAY_SOURCE_UNAVAILABLE)
        self.assertEqual(result.csv_history, request.csv_history)
        self.assertEqual(result.classified_operations, request.operations)
        self.assertEqual(list(result.availability_limitations), list(CSV_CALENDAR_LIMITATIONS))
        self.assertFalse(result.history_only_includes_recurring_flows)
        self.assertFalse(result.uncertainty.bands)
        self.assertEqual(result.uncertainty.scenarios, 0)
        self.assertEqual(result.gap_probability.unavailable_reason, "not_calibrated")
        self.assertEqual(result, csv_calendar_forecast(request, pb))
        for amount, version in ((1500, 2), (1300, 3), (1500, 4)):
            payment.remaining_outflow_minor, payment.version = amount, version
            result = csv_calendar_forecast(request, pb)
            self.assertEqual(result.days[-1].reference_balance_minor, 97000 - amount)
            known = [c for c in result.components if c.kind == "known_remaining_payment"]
            self.assertEqual(len(known), 1)
            self.assertEqual(list(known[0].source_ids), [payment.payment_id])
            self.assertEqual(known[0].outflow_minor, amount)
        payment.effective_date = payment.original_due_date = result.days[4].date
        shifted = csv_calendar_forecast(request, pb)
        self.assertEqual(shifted.days[3].reference_balance_minor, 95700)
        self.assertEqual(shifted.days[4].reference_balance_minor, 94250)
        del request.calendar.payments[:]
        cancelled = csv_calendar_forecast(request, pb)
        self.assertEqual(cancelled.days[-1].reference_balance_minor, 97000)

    def test_complete_replacement_survives_zero_occurrences(self):
        request = csv_calendar_request()
        add_payment(request)
        group = request.calendar.groups[0]
        group.additional_obligation_confirmed = False
        group.historical_operation_ids.append(request.operations[1].operation_id)
        result = csv_calendar_forecast(request, pb)
        self.assertEqual(result.days[-1].reference_balance_minor, 97600)
        self.assertEqual(
            list(result.replaced_historical_operation_ids), [request.operations[1].operation_id]
        )
        del request.calendar.payments[:]
        empty = csv_calendar_forecast(request, pb)
        self.assertEqual(empty.days[-1].reference_balance_minor, 98500)
        self.assertEqual(len(empty.history_evidence), 1)
        self.assertEqual(
            list(empty.history_evidence[0].historical_operation_ids),
            [request.operations[0].operation_id],
        )
        # Input facts and EOD remain original; only inference residual changes.
        self.assertEqual(empty.classified_operations, request.operations)
        del request.calendar.groups[:]
        revoked = csv_calendar_forecast(request, pb)
        self.assertEqual(revoked.days[-1].reference_balance_minor, 97000)

    def test_unknown_modes_provenance_overlap_and_forged_history_are_rejected(self):
        def duplicate_group(request):
            group = request.calendar.groups[0]
            group.additional_obligation_confirmed = False
            group.historical_operation_ids.append(request.operations[0].operation_id)
            duplicate = request.calendar.groups.add()
            duplicate.CopyFrom(group)
            duplicate.group_id += "other"

        changes = (
            lambda r: setattr(r.calendar, "knowledge_mode", pb.CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE),
            lambda r: setattr(
                r.calendar, "knowledge_mode", pb.CALENDAR_KNOWLEDGE_MODE_HISTORICAL_AS_OF_CERTIFIED
            ),
            lambda r: setattr(r.calendar, "knowledge_mode", 99),
            lambda r: setattr(r.calendar, "data_version", 1),
            lambda r: setattr(r.calendar, "as_of_business_date", "2026-10-02"),
            lambda r: setattr(r.calendar, "as_of_business_date", "2026-09-30"),
            lambda r: setattr(r.calendar.scenario_context, "scenario_reference", "forged-history"),
            lambda r: setattr(r.calendar.groups[0], "additional_obligation_confirmed", False),
            lambda r: setattr(r.calendar.groups[0], "complete_for_horizon", False),
            lambda r: setattr(r.calendar.payments[0], "group_id", "unknown"),
            lambda r: setattr(r.calendar.payments[0], "effective_date", "2026-10-04"),
            lambda r: setattr(r.calendar.payments[0], "original_due_date", "2026-09-29"),
            lambda r: setattr(r.calendar.payments[0], "source", "inferred_from_category"),
            lambda r: setattr(r.calendar.payments[0], "remaining_outflow_minor", 0),
            lambda r: setattr(r.calendar.payments[0], "remaining_inflow_minor", 1),
            lambda r: setattr(r.snapshot.history[1], "source", pb.DAY_SOURCE_RECONSTRUCTED_COVERED),
            lambda r: setattr(r.snapshot.history[-1], "closing_balance_minor", 95501),
            lambda r: r.availability_limitations.append("history_coverage_kind_certified"),
            duplicate_group,
        )
        for change in changes:
            with self.subTest(change=change):
                request = csv_calendar_request()
                add_payment(request)
                change(request)
                with self.assertRaises(InvalidSnapshot):
                    csv_calendar_forecast(request, pb)

    def test_context_trajectories_and_overflow_never_fallback(self):
        request = csv_calendar_request()
        add_payment(request)
        with self.assertRaises(UnsupportedContext):
            csv_forecast(request, pb)
        request.model.model_version = "unknown"
        with self.assertRaises(UnsupportedContext):
            csv_calendar_forecast(request, pb)
        request = csv_calendar_request()
        request.model.random_seed = "1"
        with self.assertRaises(UnsupportedContext):
            csv_calendar_forecast(request, pb)
        request = csv_calendar_request()
        add_payment(request, amount=2**63 - 1)
        payment = request.calendar.payments.add()
        payment.CopyFrom(request.calendar.payments[0])
        payment.payment_id = "019a0c65-9800-7000-8000-000000000052"
        with self.assertRaises(InvalidSnapshot):
            csv_calendar_forecast(request, pb)
