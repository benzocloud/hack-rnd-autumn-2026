"""Exact learned features, artifacts and strict temporal training boundaries."""

import hashlib
import json
import sys
import tempfile
import unittest
from datetime import date, timedelta
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from cashflow_baseline.data import Daily
from cashflow_baseline.evaluation import ForecastRecord
from cashflow_baseline.models import predict
from cashflow_rpc import trained_pool
from cashflow_rpc.inference import UnsupportedContext
from cashflow_rpc.trained_pool import (
    SEED,
    allocate_numerators,
    feature_numerators,
    parse_artifact,
    predict_allocations,
)
from train_model import candidate, fit


class TrainedPoolTests(unittest.TestCase):
    def history(self, amount=100):
        start = date(2025, 1, 1)
        return [(start + timedelta(days=i), {"a": (amount, 0)}) for i in range(90)]

    def test_money_above_float_precision_is_exact(self):
        amount = 2**53 + 7
        result = predict_allocations(
            self.history(amount), {"inflow": [1, 2, 1], "outflow": [4, 0, 0]}
        )
        self.assertEqual([sum(d[0].values()) for d in result], [amount] * 30)
        self.assertEqual([sum(d[1].values()) for d in result], [0] * 30)

    def test_allocation_preserves_rounded_total(self):
        result = allocate_numerators({"b": 720, "a": 720, "c": 720})
        self.assertEqual(result, {"b": 1, "a": 1, "c": 0})
        self.assertEqual(allocate_numerators({}), {})

    def test_weekly_and_recent_features_use_only_their_past_dates(self):
        history = self.history(0)
        history[-1][1]["a"] = (80, 0)
        weekly = predict_allocations(history, {"inflow": [0, 0, 4], "outflow": [4, 0, 0]})
        self.assertEqual(sum(weekly[0][0].values()), 0)
        self.assertEqual(sum(weekly[6][0].values()), 10)
        recent = predict_allocations(history, {"inflow": [0, 4, 0], "outflow": [4, 0, 0]})
        self.assertEqual(sum(recent[0][0].values()), 3)
        with self.assertRaises(ValueError):
            feature_numerators(history[:-1])

    def test_checksum_and_schema_fail_explicitly(self):
        obj = dict(
            schema_version=1,
            algorithm="convex_flow_pool_quarters_v1",
            trained_through="2025-09-30",
            weights=dict(inflow=[4, 0, 0], outflow=[4, 0, 0]),
            source_sha256={"companies.csv": "a" * 64, "all_operations.csv": "b" * 64},
            seed=SEED,
        )
        payload = json.dumps(obj).encode()
        self.assertEqual(parse_artifact(payload, hashlib.sha256(payload).hexdigest()), obj)
        with self.assertRaises(UnsupportedContext):
            parse_artifact(payload, "0" * 64)
        for wrong in ([True, 3, 0], [-1, 5, 0], [0, 0, 0]):
            obj["weights"]["inflow"] = wrong
            payload = json.dumps(obj).encode()
            with self.assertRaises(UnsupportedContext):
                parse_artifact(payload, hashlib.sha256(payload).hexdigest())

    def record(self, company, cutoff=date(2025, 3, 31)):
        history = [
            Daily(cutoff - timedelta(days=89 - i), 100, 0, 1000, True, False, 0) for i in range(90)
        ]
        predictions = predict(history, 30, "mean_90")
        truth = [Daily(p.day, p.inflow, p.outflow, p.balance, True, False, 0) for p in predictions]
        return ForecastRecord(
            company, "unknown", cutoff, "development", "mean_90", 1000, history, truth, predictions
        )

    def test_non_ascii_checksum_file_is_an_explicit_model_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            assets = Path(directory)
            (assets / "linear_pool_v1.sha256").write_bytes(b"\xff")
            with patch.object(trained_pool, "ASSETS", assets):
                trained_pool.load_artifact.cache_clear()
                try:
                    with self.assertRaises(UnsupportedContext):
                        trained_pool.load_artifact()
                finally:
                    trained_pool.load_artifact.cache_clear()

    def test_fit_never_uses_unmatured_targets(self):
        train = [self.record(f"C{i % 20:03}") for i in range(100)]
        weights, _, info = fit(train)
        future = self.record("NEW", date(2026, 1, 1))
        future.truth = [Daily(d.day, 10**12, 0, 10**12, True, False, 0) for d in future.truth]
        second, _, new_info = fit(train + [future])
        self.assertEqual(weights, dict(inflow=[4, 0, 0], outflow=[4, 0, 0]))
        self.assertEqual((weights, info), (second, new_info))

    def test_forecast_does_not_read_future_values_or_dates(self):
        record = self.record("C001")
        weights = dict(inflow=[1, 2, 1], outflow=[4, 0, 0])
        expected = candidate(record, weights).predictions
        record.truth = []
        self.assertEqual(candidate(record, weights).predictions, expected)


if __name__ == "__main__":
    unittest.main()
