import unittest
from datetime import date, timedelta

from cashflow_rpc.csv_forecast import (
    CSV_PREPROCESSING,
    CSV_VERSION,
    LIMITATIONS,
    QUALITY_VERSION,
    csv_forecast,
)
from cashflow_rpc.financial import financial_forecast
from cashflow_rpc.inference import InvalidSnapshot, UnsupportedContext
from test_rpc import RPC_READY

if RPC_READY:
    from ml.v1 import forecast_pb2 as pb


def csv_request():
    start = date(2026, 7, 2)
    cutoff = start + timedelta(days=89)
    identity = "019a0c65-9800-7000-8000-000000000001"
    r = pb.PredictRequest(
        snapshot=pb.ForecastSnapshot(
            snapshot_id=identity,
            company_id=identity,
            account_id=identity,
            data_version=2,
            currency="RUB",
            cutoff_date=cutoff.isoformat(),
            initial_balance_minor=95500,
        ),
        horizon_days=30,
        model=pb.ModelContext(
            model_name="mean_90",
            model_version=CSV_VERSION,
            preprocessing_version=CSV_PREPROCESSING,
            configuration_id=CSV_VERSION,
        ),
        csv_history=pb.CsvHistoryContext(
            rule_version=QUALITY_VERSION,
            from_date=start.isoformat(),
            through_date=cutoff.isoformat(),
            data_version=2,
            first_operation_date=start.isoformat(),
        ),
        calendar=pb.FinancialCalendar(
            snapshot_id=identity,
            data_version=2,
            knowledge_mode=pb.CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE,
        ),
        availability_limitations=LIMITATIONS,
    )
    for n in range(90):
        r.snapshot.history.add(
            date=(start + timedelta(days=n)).isoformat(), source=pb.DAY_SOURCE_UNAVAILABLE
        )
    for n, incoming, outgoing, balance in ((0, 9000, 0, 100000), (89, 0, 4500, 95500)):
        day = r.snapshot.history[n]
        day.source = pb.DAY_SOURCE_PARTIAL
        day.inflow_minor, day.outflow_minor, day.closing_balance_minor = incoming, outgoing, balance
        r.operations.add(
            operation_id=f"019a0c65-9800-7000-8000-{n + 2:012d}",
            date=day.date,
            inflow_minor=incoming,
            outflow_minor=outgoing,
            version=1,
            category_code="unknown",
            category_source="unknown",
            factual_source_kind="imported",
            source_namespace="dataset_v1",
            external_operation_id=f"C001-{n}",
        )
    return r


@unittest.skipUnless(RPC_READY, "Install RPC requirements and generate the current ML proto")
class CsvForecastTests(unittest.TestCase):
    def test_manual_supplement_retains_manual_provenance(self):
        request = csv_request()
        operation = request.operations[-1]
        operation.factual_source_kind = "manual"
        operation.source_namespace = ""
        operation.external_operation_id = ""
        result = csv_forecast(request, pb)
        self.assertEqual(result.days[-1].reference_balance_minor, 97000)
        self.assertEqual(result.classified_operations[-1].factual_source_kind, "manual")
        operation.source_namespace = "dataset_v1"
        with self.assertRaises(InvalidSnapshot):
            csv_forecast(request, pb)

    def test_sparse_history_projects_without_claiming_coverage(self):
        r = csv_request()
        before = r.SerializeToString()
        result = csv_forecast(r, pb)
        self.assertEqual(r.SerializeToString(), before)
        self.assertFalse(r.snapshot.history[1].HasField("inflow_minor"))
        self.assertFalse(r.snapshot.history[1].HasField("closing_balance_minor"))
        self.assertEqual(len(result.days), 30)
        self.assertEqual(result.days[-1].reference_balance_minor, 97000)
        self.assertEqual(result.csv_history, r.csv_history)
        self.assertEqual(list(result.availability_limitations), list(LIMITATIONS))
        self.assertFalse(result.uncertainty.calibrated)
        self.assertFalse(result.uncertainty.bands)
        self.assertEqual(result.gap_probability.unavailable_reason, "not_calibrated")
        self.assertEqual(result, csv_forecast(r, pb))

    def test_invalid_or_forged_quality_is_rejected(self):
        changes = (
            lambda r: setattr(r.csv_history, "data_version", 1),
            lambda r: setattr(r.snapshot.history[-1], "closing_balance_minor", 95501),
            lambda r: setattr(r.snapshot.history[1], "inflow_minor", 0),
            lambda r: setattr(r.snapshot.history[1], "source", pb.DAY_SOURCE_OBSERVED),
            lambda r: r.availability_limitations.append("history_coverage_kind_certified"),
            lambda r: setattr(r.csv_history, "first_operation_date", "2026-07-03"),
            lambda r: setattr(r.operations[0], "version", 0),
        )
        for change in changes:
            with self.subTest(change=change):
                r = csv_request()
                change(r)
                with self.assertRaises(InvalidSnapshot):
                    csv_forecast(r, pb)

    def test_legacy_profile_does_not_accept_csv_context(self):
        with self.assertRaises(InvalidSnapshot):
            financial_forecast(csv_request(), pb)
        r = csv_request()
        r.model.model_version = "mean_90_components_v1"
        with self.assertRaises(UnsupportedContext):
            csv_forecast(r, pb)
