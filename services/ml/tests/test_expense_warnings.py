"""Past-fold safety, warning decomposition, and unchanged expense controls."""

import sys
import tempfile
import unittest
from dataclasses import replace
from datetime import date, timedelta
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from cashflow_baseline.balance_model import combine, expense_calendar  # noqa: E402
from cashflow_baseline.data import Company, DataError, Operation, daily_as_of, history_at  # noqa: E402
from cashflow_baseline.evaluation import ForecastRecord  # noqa: E402
from cashflow_baseline.expense_gate import gated_expenses, new_gap_errors  # noqa: E402
from cashflow_baseline.expense_warning_experiment import verify_export  # noqa: E402
from cashflow_baseline.expense_warnings import classify_warnings, event_diagnostics  # noqa: E402
from cashflow_baseline.models import predict  # noqa: E402
from cashflow_baseline.report import write_predictions  # noqa: E402


def regular_company():
    operations, balance = [], 100000000
    start = date(2025, 1, 1)
    for i in range(330):
        day = start + timedelta(days=i)
        out = 1000000 if day.day == 10 else 100
        balance += 100000 - out
        operations += [
            Operation(
                "C001",
                f"{i}-in",
                day,
                100000,
                True,
                "0001",
                "Покупатель",
                "Оплата покупателя",
                balance,
            ),
            Operation(
                "C001",
                f"{i}-out",
                day,
                out,
                False,
                "0002",
                "Арендодатель" if day.day == 10 else "Банк",
                "Оплата аренды помещения" if day.day == 10 else "Комиссия за обслуживание счета",
                balance,
            ),
        ]
    return Company("C001", "Example", operations)


class ExpenseWarningTests(unittest.TestCase):
    def setUp(self):
        self.company = regular_company()
        self.cutoff = date(2025, 10, 1)
        self.history, reason = history_at(self.company, self.cutoff, 90)
        self.assertFalse(reason)

    def record(self):
        known = daily_as_of(self.company, self.company.last_date)
        truth = [known[self.cutoff + timedelta(days=i)] for i in range(1, 31)]
        return ForecastRecord(
            "C001",
            "Example",
            self.cutoff,
            "validation",
            "mean_90",
            self.history[-1].balance,
            self.history,
            truth,
            predict(self.history, 30, "mean_90"),
        )

    def test_gate_accepts_calendar_that_previously_explained_regular_expenses(self):
        predicted, evidence = gated_expenses(self.company, self.history)
        self.assertTrue(evidence["used_calendar"])
        self.assertEqual(len(evidence["folds"]), 2)
        ends = [date.fromisoformat(f["target_end"]) for f in evidence["folds"]]
        self.assertEqual(ends, [self.cutoff - timedelta(days=30), self.cutoff])
        self.assertLess(
            evidence["absolute_balance_errors_cents"]["calendar"],
            evidence["absolute_balance_errors_cents"]["baseline"],
        )
        self.assertEqual(
            predicted,
            combine(
                self.history,
                [p.inflow for p in predict(self.history, 30, "mean_90")],
                expense_calendar(self.company, self.history, 30)[0],
            ),
        )

    def test_gate_ignores_mutated_and_removed_outer_future(self):
        expected = gated_expenses(self.company, self.history)
        for operations in [
            [op for op in self.company.operations if op.day <= self.cutoff],
            [
                replace(op, amount=10**20, balance=-(10**20), purpose="Произвольное будущее")
                if op.day > self.cutoff
                else op
                for op in self.company.operations
            ],
        ]:
            self.assertEqual(
                expected, gated_expenses(replace(self.company, operations=operations), self.history)
            )

    def test_short_additional_history_falls_back_without_dropping_window(self):
        cutoff = date(2025, 4, 15)
        history, reason = history_at(self.company, cutoff, 90)
        self.assertFalse(reason)
        base = predict(history, 30, "mean_90")
        result, evidence = gated_expenses(
            self.company, history, outgoing=[p.outflow + 1 for p in base]
        )
        self.assertEqual(result, base)
        self.assertIn("short_history", evidence["reason"])

    def test_missing_quality_in_inner_target_falls_back(self):
        broken = replace(
            self.company,
            operations=[
                replace(op, balance=op.balance + 1000)
                if op.day == self.cutoff - timedelta(days=40)
                else op
                for op in self.company.operations
            ],
        )
        # Mandatory outer history is provided unchanged; the additional fold's
        # independently reconstructed target must still be rejected.
        base = predict(self.history, 30, "mean_90")
        result, evidence = gated_expenses(
            broken, self.history, outgoing=[p.outflow + 1 for p in base]
        )
        self.assertEqual(result, base)
        self.assertIn("quality", evidence["reason"])

    def test_expense_gate_does_not_change_income_and_is_exact(self):
        predicted, _ = gated_expenses(self.company, self.history)
        self.assertEqual(
            [p.inflow for p in predicted], [p.inflow for p in predict(self.history, 30, "mean_90")]
        )
        balance = self.history[-1].balance
        for p in predicted:
            balance += p.inflow - p.outflow
            self.assertEqual(p.balance, balance)
        with self.assertRaises(DataError):
            gated_expenses(self.company, self.history, horizon=15)

    def test_gap_counts_distinguish_missing_warning_and_false_warning(self):
        record = self.record()
        positive = [replace(p, balance=100) for p in record.predictions]
        negative = [replace(p, balance=-100) for p in record.predictions]
        self.assertEqual(new_gap_errors(100, positive, negative), {"fp": 1, "fn": 0})
        self.assertEqual(new_gap_errors(100, negative, positive), {"fp": 0, "fn": 1})
        self.assertEqual(new_gap_errors(-100, negative, positive), {"fp": 0, "fn": 0})

    def test_new_false_warning_reconciles_and_is_classified(self):
        record = self.record()
        bad = replace(
            record,
            method="expense_only",
            predictions=combine(
                self.history, [p.inflow for p in record.predictions], [100000000] * 30
            ),
        )
        rows, summary = classify_warnings(
            [record], [bad], {"C001": self.company}, {("C001", self.cutoff): []}
        )
        self.assertEqual(summary["validation"]["added"], 1)
        self.assertEqual(rows[0]["dominant_component"], "outflow_amount")
        self.assertEqual(rows[0]["historical_double_counted_operations"], 0)
        self.assertGreater(rows[0]["outflow_overestimate_cents"], 0)

    def test_event_matching_is_explicitly_retrospective_and_censored(self):
        record = self.record()
        _, patterns, _ = expense_calendar(self.company, self.history, 30)
        evidence = event_diagnostics(self.company, record, patterns)
        self.assertTrue(evidence)
        self.assertTrue(all(e["matching_is_diagnostic_not_ground_truth"] for e in evidence))

    def test_final_export_audit_keeps_reference_rows(self):
        record = self.record()
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder)
            ref = output / "reference"
            ref.mkdir()
            write_predictions(ref, [record])
            write_predictions(output, [record, replace(record, method="expense_only")])
            self.assertEqual(verify_export(output, ref / "predictions.csv")["daily_rows"], 60)


if __name__ == "__main__":
    unittest.main()
