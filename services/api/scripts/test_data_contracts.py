import copy
import json
import unittest
from pathlib import Path

import yaml
from jsonschema import Draft202012Validator, FormatChecker, ValidationError

ROOT = Path(__file__).resolve().parents[3]
CONTRACTS = ROOT / "docs/data-contracts"


class CalculationContractTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.schema = json.loads((CONTRACTS / "forecast-calculation-input.schema.json").read_text())
        cls.example = json.loads((CONTRACTS / "examples/calculation.sample.json").read_text())
        cls.validator = Draft202012Validator(cls.schema, format_checker=FormatChecker())

    def test_copied_rest_definitions_match_current_openapi(self):
        schemas = yaml.safe_load((ROOT / "docs/api/openapi.yaml").read_text())["components"][
            "schemas"
        ]
        for name, definition in self.schema["$defs"].items():
            if name in schemas:
                with self.subTest(name=name):
                    expected = json.loads(
                        json.dumps(schemas[name]).replace("#/components/schemas/", "#/$defs/")
                    )
                    self.assertEqual(definition, expected)

    def test_calculation_example_is_valid_with_uuid_formats(self):
        self.validator.validate(self.example)

    def test_historical_backtest_rejects_synthetic_calendar(self):
        example = copy.deepcopy(self.example)
        example["mode"] = "history_backtest"
        example["known_payments"] = []
        example["input"]["calendar_mode"] = "synthetic_scenario"
        example["input"]["scenario_context"] = {
            "scenario_reference": "demo-1",
            "recorded_at": "2026-10-24T20:00:00Z",
            "scenario_business_date": example["input"]["cutoff_date"],
        }
        with self.assertRaises(ValidationError):
            self.validator.validate(example)

    def test_internal_ids_reject_opaque_or_nil_values(self):
        for path in [("company_id",), ("input", "snapshot_id"), ("input", "account_id")]:
            for invalid in ["snap_123", "00000000-0000-0000-0000-000000000000"]:
                with self.subTest(path=path, invalid=invalid):
                    example = copy.deepcopy(self.example)
                    target = example
                    for key in path[:-1]:
                        target = target[key]
                    target[path[-1]] = invalid
                    with self.assertRaises(ValidationError):
                        self.validator.validate(example)
