import unittest
from datetime import date, timedelta

from cashflow_rpc.analysis import (
    ANALYSIS_VERSION,
    CSV_ANALYSIS_VERSION,
    CSV_CALENDAR_ANALYSIS_VERSION,
    PROFILES,
    analysis_forecast,
)
from cashflow_rpc.csv_calendar import csv_calendar_forecast
from cashflow_rpc.csv_forecast import csv_forecast
from cashflow_rpc.inference import InvalidSnapshot, UnsupportedContext
from test_rpc import RPC_READY
from test_rpc_csv import csv_request
from test_rpc_csv_calendar import add_payment, csv_calendar_request

if RPC_READY:
    from ml.v1 import forecast_pb2 as pb


def analyzed_request(calendar=False):
    r = csv_calendar_request() if calendar else csv_request()
    version = CSV_CALENDAR_ANALYSIS_VERSION if calendar else CSV_ANALYSIS_VERSION
    r.model.model_version = r.model.configuration_id = version
    r.model.preprocessing_version = PROFILES[version][0]
    return r


def monthly_request():
    r = analyzed_request()
    start = date.fromisoformat(r.csv_history.from_date)
    del r.operations[:]
    balance = 100000
    for index, day in enumerate(r.snapshot.history):
        current = start + timedelta(days=index)
        day.ClearField("inflow_minor")
        day.ClearField("outflow_minor")
        day.ClearField("closing_balance_minor")
        day.source = pb.DAY_SOURCE_UNAVAILABLE
        if current.day == 5 or index == 0:
            day.inflow_minor, day.outflow_minor = 0, 900
            balance -= 900
            day.closing_balance_minor, day.source = balance, pb.DAY_SOURCE_PARTIAL
            r.operations.add(
                operation_id=f"019a0c65-9800-7000-8000-{index + 2:012d}",
                date=day.date,
                outflow_minor=900,
                version=1,
                category_code="unknown",
                category_source="unknown",
                counterparty_inn="0012345678",
                counterparty_name="Арендодатель",
                payment_purpose="Аренда помещения",
                factual_source_kind="imported",
                source_namespace="dataset_v1",
                external_operation_id=f"rent-{index}",
            )
    r.snapshot.history[-1].closing_balance_minor = balance
    r.snapshot.history[-1].source = pb.DAY_SOURCE_PARTIAL
    r.snapshot.initial_balance_minor = balance
    return r


@unittest.skipUnless(RPC_READY, "Install RPC requirements and generate the current ML proto")
class RuntimeAnalysisTests(unittest.TestCase):
    def test_money_unchanged_current_and_unavailable_deterministic(self):
        for calendar in (False, True):
            r = analyzed_request(calendar)
            if calendar:
                add_payment(r)
            r.operations[-1].payment_purpose = "Уплата налогов ЕНП"
            original = r.SerializeToString()
            result = analysis_forecast(r, pb)
            self.assertEqual(result, analysis_forecast(r, pb))
            self.assertEqual(r.SerializeToString(), original)
            old = pb.PredictRequest()
            old.CopyFrom(r)
            _, version, preprocessing = PROFILES[r.model.model_version]
            old.model.model_version = old.model.configuration_id = version
            old.model.preprocessing_version = preprocessing
            baseline = (csv_calendar_forecast if calendar else csv_forecast)(old, pb)
            self.assertEqual(result.days, baseline.days)
            self.assertEqual(result.components, baseline.components)
            self.assertEqual(result.history_evidence, baseline.history_evidence)
            self.assertEqual(result.classified_operations[-1].category_code, "tax")
            self.assertEqual(result.classified_operations[-1].category_rule_id, "explicit_tax_text")
            self.assertEqual(result.classified_operations[-1].category_source, "rules")
            self.assertEqual(result.analysis_context.analysis_version, ANALYSIS_VERSION)
            self.assertEqual(result.analysis_context.window_from, r.csv_history.from_date)
            self.assertFalse(result.recurring_hypotheses)
            self.assertFalse(baseline.HasField("analysis_context"))

    def test_monthly_hypothesis_exact_evidence_and_manual_precedence_clear(self):
        r = monthly_request()
        result = analysis_forecast(r, pb)
        self.assertEqual(len(result.recurring_hypotheses), 1)
        h = result.recurring_hypotheses[0]
        self.assertEqual(h.confirmations, 3)
        self.assertEqual(h.rule_id, "monthly_fit")
        self.assertEqual(h.rule_version, ANALYSIS_VERSION)
        self.assertEqual(h.status, pb.RECURRING_HYPOTHESIS_STATUS_HYPOTHESIS)
        self.assertEqual(h.expected_amount_minor, 900)
        facts = {op.operation_id: op for op in r.operations}
        self.assertTrue(
            all(ref.version == facts[ref.operation_id].version for ref in h.historical_operations)
        )
        self.assertEqual(result, analysis_forecast(r, pb))
        for op in r.operations:
            op.category_source = "user"
            op.category_id = "019a0c65-9800-7000-8000-000000000101"
            op.category_company_id = r.snapshot.company_id
        manual = analysis_forecast(r, pb)
        self.assertEqual(manual.classified_operations, r.operations)
        self.assertFalse(manual.recurring_hypotheses)
        for op in r.operations:
            op.category_source = "unknown"
            op.ClearField("category_id")
            op.ClearField("category_company_id")
        self.assertEqual(analysis_forecast(r, pb), result)

    def test_invalid_inputs_never_relax_csv_guards_or_context(self):
        mutations = [
            lambda r: setattr(r.model, "preprocessing_version", "unknown"),
            lambda r: setattr(r.model, "configuration_id", "unknown"),
            lambda r: setattr(r.model, "model_version", "unknown"),
            lambda r: setattr(r.model, "random_seed", "1"),
            lambda r: setattr(r.operations[-1], "date", "2030-01-01"),
            lambda r: setattr(r.operations[-1], "operation_id", r.operations[0].operation_id),
            lambda r: setattr(r.operations[-1], "category_rule_id", "forged"),
            lambda r: r.snapshot.history.add(date="2030-01-01"),
            lambda r: r.snapshot.history.pop(),
            lambda r: r.snapshot.history[-1].ClearField("closing_balance_minor"),
        ]
        for mutate in mutations:
            with self.subTest(mutate=mutate):
                r = analyzed_request()
                mutate(r)
                with self.assertRaises((InvalidSnapshot, UnsupportedContext)):
                    analysis_forecast(r, pb)
