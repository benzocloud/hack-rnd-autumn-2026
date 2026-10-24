"""Behavioral checks for date boundaries, accounting, quality and evaluation."""

import csv
import sys
import tempfile
import unittest
from dataclasses import replace
from datetime import date, timedelta
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from cashflow_baseline.data import (  # noqa: E402
    Company,
    Daily,
    DataError,
    Operation,
    daily_as_of,
    history_at,
    load_companies,
    parse_money,
)
from cashflow_baseline.evaluation import (  # noqa: E402
    Config,
    ForecastRecord,
    aggregate,
    evaluate,
    split_for,
    truth_window,
)
from cashflow_baseline.models import Prediction, predict  # noqa: E402

START = date(2025, 1, 1)


def company(days=180):
    operations, balance = [], 1000000
    for index in range(days):
        day = START + timedelta(days=index)
        incoming = (day.weekday() + 1) * 1000
        balance += incoming - 2000
        operations.extend(
            [
                Operation(
                    "C001",
                    f"{index}-in",
                    day,
                    incoming,
                    True,
                    "0012345678",
                    "Client",
                    "sale",
                    balance,
                ),
                Operation(
                    "C001",
                    f"{index}-out",
                    day,
                    2000,
                    False,
                    "0012345679",
                    "Supplier",
                    "cost",
                    balance,
                ),
            ]
        )
    return Company("C001", "Example", operations)


def config():
    return Config(
        start=START + timedelta(days=90),
        end=START + timedelta(days=120),
        validation_start=date(2026, 1, 1),
        evaluation_start=date(2026, 4, 1),
    )


class DataTests(unittest.TestCase):
    def test_money_is_exact_and_rejects_nonfinite_and_subcent(self):
        self.assertEqual(parse_money("123456789.01"), 12345678901)
        for text in ["NaN", "Infinity", "0.001", "bad"]:
            with self.subTest(text=text), self.assertRaises(DataError):
                parse_money(text)

    def test_daily_balance_is_not_summed_across_operations(self):
        source = company()
        daily = daily_as_of(source, START)
        self.assertEqual(daily[START].balance, source.operations[0].balance)
        self.assertEqual(daily[START].outflow, 2000)

    def test_conflicting_same_day_balances_are_rejected(self):
        source = company()
        source.operations[1] = replace(source.operations[1], balance=999)
        with self.assertRaises(DataError):
            daily_as_of(source, START)

    def test_future_evidence_does_not_retroactively_change_snapshot(self):
        first = Operation("C001", "1", START, 100, True, "", "", "", 1000)
        later = replace(first, operation_id="2", day=START + timedelta(days=10), balance=5000)
        source = Company("C001", "Example", [first, later])
        past = daily_as_of(source, START + timedelta(days=5))
        retrospective = daily_as_of(source, later.day)
        self.assertFalse(any(day.gap for day in past.values()))
        self.assertTrue(retrospective[START + timedelta(days=3)].gap)
        self.assertIsNone(retrospective[START + timedelta(days=3)].balance)
        self.assertTrue(retrospective[later.day].observed_balance)
        self.assertEqual(retrospective[later.day].balance, 5000)

    def test_duplicate_candidates_preserved_but_history_excluded(self):
        source = company()
        op = source.operations[170]
        source.operations.append(replace(op, operation_id="different-id"))
        snapshot = daily_as_of(source, START + timedelta(days=100))
        self.assertEqual(snapshot[op.day].inflow, op.amount * 2)
        self.assertEqual(snapshot[op.day].duplicates, 1)
        _, reason = history_at(source, START + timedelta(days=100), 90)
        self.assertIn(reason, ["history_reconciliation_gap", "history_duplicate_candidate"])

    def test_one_cent_reconciliation_is_retained_as_warning(self):
        source = company()
        source.operations = [
            replace(op, balance=op.balance + int(op.day >= START + timedelta(days=40)))
            for op in source.operations
        ]
        snapshot = daily_as_of(source, START + timedelta(days=100))
        self.assertEqual(snapshot[START + timedelta(days=40)].reconciliation, 1)
        history, reason = history_at(source, START + timedelta(days=100), 90)
        self.assertFalse(reason)
        self.assertEqual(len(history), 90)

    def test_cutoff_requires_observed_balance_and_sufficient_history(self):
        source = company()
        self.assertEqual(history_at(source, START + timedelta(days=20), 90)[1], "short_history")
        cutoff = START + timedelta(days=100)
        source.operations = [op for op in source.operations if op.day != cutoff]
        self.assertEqual(history_at(source, cutoff, 90)[1], "cutoff_balance_not_observed")

    def test_loader_preserves_leading_zero_inn_and_rejects_id_reuse(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "companies.csv").write_text(
                "company_id;industry\nC001;Example\n", encoding="utf-8-sig"
            )
            fields = [
                "company_id",
                "operation_id",
                "operation_date",
                "amount",
                "direction",
                "counterparty_inn",
                "counterparty_name",
                "payment_purpose",
                "balance_eod",
            ]
            row = [
                "C001",
                "id",
                "2025-01-01",
                "1.00",
                "Поступление",
                "0012345678",
                "Client",
                "",
                "2.00",
            ]
            path = root / "all_operations.csv"
            with path.open("w", encoding="utf-8-sig", newline="") as stream:
                writer = csv.writer(stream, delimiter=";")
                writer.writerows([fields, row])
            loaded = load_companies(root)
            self.assertEqual(loaded[0].operations[0].inn, "0012345678")
            with path.open("a", encoding="utf-8", newline="") as stream:
                csv.writer(stream, delimiter=";").writerow(row)
            with self.assertRaises(DataError):
                load_companies(root)


class ForecastTests(unittest.TestCase):
    def test_weekday_forecast_uses_historical_weekday_for_entire_horizon(self):
        history, reason = history_at(company(), START + timedelta(days=100), 90)
        self.assertFalse(reason)
        predictions = predict(history, 30, "weekday_8w")
        balance = history[-1].balance
        for prediction in predictions:
            self.assertEqual(prediction.inflow, (prediction.day.weekday() + 1) * 1000)
            self.assertEqual(prediction.outflow, 2000)
            balance += prediction.inflow - prediction.outflow
            self.assertEqual(prediction.balance, balance)
        self.assertEqual(predictions[0].day, history[-1].day + timedelta(days=1))

    def test_mean_forecast_is_constant_and_balance_rolls_forward(self):
        history, _ = history_at(company(), START + timedelta(days=100), 90)
        predictions = predict(history, 30, "mean_90")
        self.assertEqual(len({day.inflow for day in predictions}), 1)
        expected = history[-1].balance + sum(day.inflow - day.outflow for day in predictions)
        self.assertEqual(predictions[-1].balance, expected)

    def test_mutating_future_cannot_change_either_forecast(self):
        source = company()
        cutoff = START + timedelta(days=100)
        changed = Company(
            "C001",
            "Example",
            [
                replace(op, amount=op.amount * 50, balance=-100000) if op.day > cutoff else op
                for op in source.operations
            ],
        )
        first, _ = history_at(source, cutoff, 90)
        second, _ = history_at(changed, cutoff, 90)
        self.assertEqual(first, second)
        for method in ["mean_90", "weekday_8w"]:
            self.assertEqual(predict(first, 30, method), predict(second, 30, method))

    def test_invalid_or_incomplete_history_is_rejected(self):
        history, _ = history_at(company(), START + timedelta(days=100), 90)
        with self.assertRaises(DataError):
            predict(history[:10], 30, "mean_90")
        with self.assertRaises(DataError):
            predict(history[:10] + history[11:], 30, "weekday_8w")
        with self.assertRaises(DataError):
            predict(history, 0, "mean_90")


class EvaluationTests(unittest.TestCase):
    def test_daily_and_horizon_errors_are_distinct(self):
        truth = [
            Daily(START + timedelta(days=1), 100, 0, 100, True, False, 0),
            Daily(START + timedelta(days=2), 0, 0, 100, True, False, 0),
        ]
        predictions = [
            Prediction(truth[0].day, 0, 0, 0),
            Prediction(truth[1].day, 100, 0, 100),
        ]
        record = ForecastRecord(
            "C001", "Example", START, "evaluation", "mean_90", 0, [], truth, predictions
        )
        metrics = aggregate([record])
        self.assertEqual(metrics["inflow_wape"], 2)
        self.assertEqual(metrics["horizon_inflow_wape"], 0)
        self.assertIsNone(metrics["horizon_outflow_wape"])

    def test_temporal_boundary_windows_are_excluded(self):
        settings = config()
        self.assertEqual(split_for(date(2025, 11, 30), settings), "development")
        self.assertIsNone(split_for(date(2025, 12, 15), settings))
        self.assertEqual(split_for(date(2026, 1, 1), settings), "validation")
        self.assertIsNone(split_for(date(2026, 3, 15), settings))
        self.assertEqual(split_for(date(2026, 4, 1), settings), "evaluation")

    def test_overlapping_future_windows_are_not_allowed(self):
        with self.assertRaises(ValueError):
            replace(config(), stride=7).validate()

    def test_unavailable_future_is_not_filled_with_zero(self):
        source = company(120)
        full = daily_as_of(source, source.last_date)
        truth, reason = truth_window(source, full, source.last_date - timedelta(days=10), 30)
        self.assertEqual(truth, [])
        self.assertEqual(reason, "future_not_fully_observed")

    def test_methods_share_identical_windows_and_no_overlapping_targets(self):
        records, skipped, _ = evaluate([company()], config())
        self.assertFalse(skipped)
        keys = {
            method: [r.cutoff for r in records if r.method == method]
            for method in ["mean_90", "weekday_8w"]
        }
        self.assertEqual(keys["mean_90"], keys["weekday_8w"])
        self.assertTrue(
            all((b - a).days >= 30 for a, b in zip(keys["mean_90"], keys["mean_90"][1:]))
        )

    def test_new_and_continuing_negative_balances_have_separate_metrics(self):
        truth = [Daily(START + timedelta(days=1), 0, 100, -100, True, False, 0)]
        prediction = [Prediction(truth[0].day, 0, 100, 100)]
        new = ForecastRecord(
            "C001", "Example", START, "evaluation", "mean_90", 100, [], truth, prediction
        )
        old = replace(new, company="C002", initial_balance=-100)
        result = aggregate([new, old])
        self.assertEqual(result["new_gap"]["fn"], 1)
        self.assertEqual(result["continuing_gap"]["fn"], 1)
        self.assertIsNone(result["new_gap"]["precision"])
        self.assertEqual(result["new_gap"]["recall"], 0)
        self.assertIsNone(result["inflow_wape"])
        self.assertEqual(result["balance_mae"], 2)

    def test_actual_gap_does_not_equal_predicted_warning(self):
        truth = [Daily(START + timedelta(days=1), 0, 0, 100, True, False, 0)]
        record = ForecastRecord(
            "C001",
            "Example",
            START,
            "evaluation",
            "mean_90",
            100,
            [],
            truth,
            [Prediction(truth[0].day, 0, 0, -100)],
        )
        metrics = aggregate([record])
        self.assertEqual(metrics["new_gap"]["fp"], 1)
        self.assertIsNone(metrics["new_gap"]["recall"])


if __name__ == "__main__":
    unittest.main()
