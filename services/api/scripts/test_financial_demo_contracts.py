"""D2.0 REST schema regressions; no runtime readiness is implied."""

import copy
import unittest
from pathlib import Path

import yaml
from jsonschema import Draft202012Validator, FormatChecker, ValidationError

ROOT = Path(__file__).resolve().parents[3]
ID = "019a0c65-9800-7000-8000-000000000001"


class FinancialDemoContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.spec = yaml.safe_load((ROOT / "docs/api/openapi.yaml").read_text())
        cls.schemas = cls.spec["components"]["schemas"]

    def validate(self, name, value):
        schema = self.schemas[name] | {"components": self.spec["components"]}
        Draft202012Validator(schema, format_checker=FormatChecker()).validate(value)

    def reject(self, name, value):
        with self.assertRaises(ValidationError):
            self.validate(name, value)

    def group_request(self):
        return {
            "kind": "additional",
            "cutoff_date": "2026-09-30",
            "horizon_start": "2026-10-01",
            "horizon_end": "2026-10-30",
            "additional": {"confirmed": True},
        }

    def test_group_requires_one_confirmed_typed_branch(self):
        request = self.group_request()
        self.validate("CreateReconciliationGroupRequest", request)
        for change in [
            {"additional": {"confirmed": False}},
            {"additional": None},
            {"kind": "unknown"},
            {"confirmed_by": ID},
            {"replacement": {"historical_operations": [], "complete_for_horizon": True}},
        ]:
            self.reject("CreateReconciliationGroupRequest", request | change)
        replacement = request | {
            "kind": "replacement",
            "replacement": {
                "historical_operations": [{"transaction_id": ID, "version": "2"}],
                "complete_for_horizon": True,
            },
        }
        del replacement["additional"]
        self.validate("CreateReconciliationGroupRequest", replacement)
        for operations in [
            [],
            [{"transaction_id": "opaque", "version": "2"}],
            [{"transaction_id": ID, "version": "0"}],
        ]:
            invalid = copy.deepcopy(replacement)
            invalid["replacement"]["historical_operations"] = operations
            self.reject("CreateReconciliationGroupRequest", invalid)

    def test_payment_group_patch_preserves_omitted_and_distinguishes_clear(self):
        for value in [None, ID]:
            self.validate(
                "UpdatePaymentRequest", {"version": "2", "reconciliation_group_id": value}
            )
        self.validate("UpdatePaymentRequest", {"version": "2", "description": "changed"})
        self.reject("UpdatePaymentRequest", {"version": "2"})
        self.reject("UpdatePaymentRequest", {"version": "2", "reconciliation_group_id": "invalid"})
        self.reject("RevokeReconciliationGroupRequest", {})
        self.reject("RevokeReconciliationGroupRequest", {"version": "0"})
        self.validate("RevokeReconciliationGroupRequest", {"version": "2"})

    def test_legacy_examples_and_optional_nullable_read_fields(self):
        for name in ["Import", "ImportPreview", "ForecastInput", "ForecastSummary", "Payment"]:
            example = self.spec["components"]["examples"].get(name)
            if example is not None:
                self.validate(name, example["value"])
        for name in ["ImportPreview", "Import", "ForecastInput"]:
            coverage = self.schemas[name]["properties"]["history_coverage"]
            self.assertTrue(coverage["readOnly"])
            self.assertNotIn("history_coverage", self.schemas[name]["required"])
            validator = Draft202012Validator(coverage | {"components": self.spec["components"]})
            validator.validate(None)

    def test_fixture_coverage_requires_explicit_limitation(self):
        proof = {
            "kind": "dataset_fixture_assumption",
            "from_date": "2026-07-03",
            "through_date": "2026-09-30",
            "reference": "dataset_fixture_assumption:hash",
            "data_version": "2",
            "limitations": [],
        }
        self.reject("HistoryCoverage", proof)
        self.validate(
            "HistoryCoverage",
            proof | {"limitations": ["dataset_coverage_assumed_not_bank_certified"]},
        )
        self.reject("HistoryCoverage", proof | {"kind": "unknown"})

    def test_synthetic_read_context_and_request_authority(self):
        request = {"cutoff_date": "2026-09-30", "horizon_days": 30}
        self.validate("CreateForecastRequest", request)
        self.validate("CreateForecastRequest", request | {"calendar_mode": "synthetic_scenario"})
        for change in [
            {"calendar_mode": None},
            {"calendar_mode": "current"},
            {"scenario_context": {}},
            {"history_coverage": None},
        ]:
            self.reject("CreateForecastRequest", request | change)
        example = copy.deepcopy(
            self.spec["components"]["examples"]["ForecastList"]["value"]["items"][0]
        )
        self.reject("ForecastSummary", example | {"calendar_mode": "synthetic_scenario"})
        context = {
            "scenario_reference": "demo-1",
            "recorded_at": "2026-10-24T20:00:00Z",
            "scenario_business_date": "2026-09-30",
        }
        self.validate(
            "ForecastSummary",
            example | {"calendar_mode": "synthetic_scenario", "scenario_context": context},
        )
        self.reject(
            "ForecastSummary", example | {"calendar_mode": "available", "scenario_context": context}
        )
        self.reject("ForecastSummary", example | {"scenario_context": context})

    def test_group_mutations_require_csrf_and_idempotency_headers(self):
        paths = self.spec["paths"]
        root = "/companies/{company_id}/reconciliation-groups"
        for path in [root, root + "/{group_id}/revoke"]:
            refs = {item["$ref"].rsplit("/", 1)[-1] for item in paths[path]["post"]["parameters"]}
            self.assertEqual(refs, {"Csrf", "IdempotencyKey"})
            self.assertIn("409", paths[path]["post"]["responses"])

    def test_analysis_has_typed_anchor_and_no_probability_or_raw_party(self):
        hypothesis = {
            "pattern_id": "pattern-1",
            "rule_id": "monthly",
            "direction": "outflow",
            "historical_operations": [{"transaction_id": ID, "version": "2"}],
            "day_of_month": 15,
            "weekend_shift": "none",
            "expected_amount_minor": "100",
            "confirmations": 3,
            "expected_periods": 3,
            "first_evidence_date": "2026-07-15",
            "last_evidence_date": "2026-09-15",
            "category_code": "salary",
            "rule_version": "1",
            "status": "hypothesis",
        }
        self.validate("RecurringHypothesis", hypothesis)
        for change in [
            {"month_end": True},
            {"day_of_month": 0},
            {"gap_probability": 0.9},
            {"counterparty_inn": "123"},
        ]:
            self.reject("RecurringHypothesis", hypothesis | change)
        analysis = {
            "analysis_version": "text_categories_monthly_v1",
            "window_from": "2026-07-03",
            "window_through": "2026-09-30",
            "classified_operations": [],
            "recurring_hypotheses": [hypothesis],
        }
        self.validate("ForecastAnalysis", analysis)
        self.reject("ForecastAnalysis", analysis | {"analysis_version": "unknown"})


if __name__ == "__main__":
    unittest.main()
