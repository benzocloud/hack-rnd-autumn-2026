"""Safety boundaries of the daily-balance experiment and product metrics."""

import csv
import json
import sys
import tempfile
import unittest
from dataclasses import replace
from datetime import date, timedelta
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from cashflow_baseline.balance_diagnostics import (  # noqa: E402
    diagnostic_row,
    human_sample,
    product_metrics,
    write_csv,
)
from cashflow_baseline.balance_experiment import gates, saved_control  # noqa: E402
from cashflow_baseline.balance_model import (  # noqa: E402
    RECEIPT_METHODS,
    cautious_forecast,
    channel_forecast,
    combine,
    expense_calendar,
    feature_row,
    incoming_forecast,
    receipt_channels,
    rounded_median,
)
from cashflow_baseline.data import (  # noqa: E402
    Company,
    DataError,
    Operation,
    daily_as_of,
    history_at,
    money,
    parse_money,
    sha256,
)
from cashflow_baseline.evaluation import ForecastRecord  # noqa: E402
from cashflow_baseline.models import predict  # noqa: E402
from cashflow_baseline.report import write_predictions  # noqa: E402
from research.balance_audit import audit  # noqa: E402


def fixture():
    operations, balance = [], 100000000
    start = date(2025, 1, 1)
    for i in range(250):
        day = start + timedelta(days=i)
        entries = []
        if day.weekday() < 5:
            entries += [(12345 + i % 4, True, "Зачисление средств по операциям эквайринга")]
        if day.weekday() == 2:
            entries += [(23457, True, "Взнос наличной выручки")]
        if i % 28 == 0:
            entries += [(34567, True, "Перевод собственных средств с расчетного счета")]
        if day.day == 10:
            entries += [(100001, False, "Оплата аренды помещения за месяц")]
        entries += [(123, False, "Комиссия банка за перевод")]
        balance += sum(amount if incoming else -amount for amount, incoming, _ in entries)
        for j, (amount, incoming, purpose) in enumerate(entries):
            operations.append(
                Operation(
                    "C001",
                    f"{i}-{j}",
                    day,
                    amount,
                    incoming,
                    "0012345678",
                    "Компания",
                    purpose,
                    balance,
                )
            )
    return Company("C001", "Example", operations)


class BalanceModelTests(unittest.TestCase):
    def setUp(self):
        self.company = fixture()
        self.cutoff = date(2025, 7, 20)
        self.history, reason = history_at(self.company, self.cutoff, 90)
        self.assertFalse(reason)

    def test_all_models_and_calendar_ignore_future_mutation_and_deletion(self):
        past = replace(
            self.company, operations=[op for op in self.company.operations if op.day <= self.cutoff]
        )
        changed = replace(
            self.company,
            operations=[
                replace(
                    op,
                    amount=999999999999,
                    balance=-999999999,
                    purpose="Заем учредителя",
                    inn="9999999",
                )
                if op.day > self.cutoff
                else op
                for op in self.company.operations
            ],
        )
        for method in RECEIPT_METHODS:
            expected = incoming_forecast(self.company, self.history, 30, method)
            self.assertEqual(expected, incoming_forecast(past, self.history, 30, method))
            self.assertEqual(expected, incoming_forecast(changed, self.history, 30, method))
        self.assertEqual(
            expense_calendar(self.company, self.history, 30),
            expense_calendar(changed, self.history, 30),
        )

    def test_channels_preserve_all_money_and_own_transfers(self):
        channels, _ = receipt_channels(self.company, self.history)
        self.assertEqual(
            sum(sum(v.values()) for v in channels.values()), sum(d.inflow for d in self.history)
        )
        self.assertGreater(sum(channels["other"].values()), 0)
        result, evidence = incoming_forecast(self.company, self.history, 30, "channel_events")
        self.assertGreater(evidence["channels"]["other"]["history_total_cents"], 0)
        self.assertTrue(all(isinstance(v, int) and v >= 0 for v in result))

    def test_rare_regular_receipts_have_explicit_dates(self):
        days = [date(2026, 1, 1) + timedelta(days=i) for i in range(90)]
        values = [10001 if i % 14 == 0 else 0 for i in range(90)]
        result, evidence = channel_forecast(values, days, 30, "channel_events")
        self.assertEqual(evidence["mode"], "renewal_events")
        self.assertEqual([i for i, amount in enumerate(result) if amount], [8, 22])
        self.assertEqual(sum(result), 20002)

    def test_irregular_rare_receipts_explicitly_fall_back_without_erasing(self):
        days = [date(2026, 1, 1) + timedelta(days=i) for i in range(90)]
        values = [9000 if i in {3, 8, 21, 50, 88} else 0 for i in range(90)]
        for method in RECEIPT_METHODS:
            result, evidence = channel_forecast(values, days, 30, method)
            self.assertEqual(evidence["mode"], "irregular_mean_fallback")
            self.assertEqual(result, [500] * 30)

    def test_event_slots_preserve_expected_budget_to_minor_unit(self):
        days = [date(2026, 1, 1) + timedelta(days=i) for i in range(90)]
        values = [
            10001 + i * 13 if day.weekday() < 5 and i % 13 else 0 for i, day in enumerate(days)
        ]
        expected, _ = channel_forecast(values, days, 30, "channel_expected")
        events, _ = channel_forecast(values, days, 30, "channel_events")
        self.assertEqual(sum(events), sum(expected))
        self.assertTrue(
            all(
                v == 0
                for i, v in enumerate(events)
                if (days[-1] + timedelta(days=i + 1)).weekday() >= 5
            )
        )

    def test_logistic_features_never_use_current_or_later_label(self):
        values = [0, 1, 0, 1] * 30
        changed = values[:70] + [999999] * 50
        self.assertEqual(
            feature_row(values, 70, date(2026, 1, 1)), feature_row(changed, 70, date(2026, 1, 1))
        )

    def test_bad_required_history_rejected_not_hidden_by_fallback(self):
        history = list(self.history)
        history[-2] = replace(history[-2], gap=True)
        with self.assertRaises(DataError):
            incoming_forecast(self.company, history, 30, "channel_events")

    def test_calendar_does_not_repeat_paid_period_and_keeps_other_expenses(self):
        outgoing, patterns, events = expense_calendar(self.company, self.history, 30)
        self.assertTrue(patterns)
        self.assertTrue(all(event.day.month == 8 for event in events))
        self.assertEqual(sum(outgoing), 123 * 30 + 100001)

    def test_cautious_is_below_expected_cumulatively_and_conserves_balance(self):
        expected = predict(self.history, 30, "mean_90")
        cautious, evidence = cautious_forecast(self.company, self.history, expected)
        self.assertEqual(evidence["kind"], "sensitivity_not_probability")
        self.assertTrue(all(a.balance <= b.balance for a, b in zip(cautious, expected)))
        self.assertEqual(
            cautious,
            combine(self.history, [p.inflow for p in cautious], [p.outflow for p in cautious]),
        )

    def test_exact_large_money_and_negative_or_float_flow_rejection(self):
        self.assertEqual(rounded_median([10**20 + 1, 10**20 + 2]), 10**20 + 2)
        rows = combine(self.history, [10**20 + 1], [10**20])
        self.assertEqual(rows[0].balance, self.history[-1].balance + 1)
        for bad in (-1, 1.2):
            with self.assertRaises(DataError):
                combine(self.history, [bad], [0])

    def record(self):
        full = daily_as_of(self.company, self.company.last_date)
        truth = [full[self.cutoff + timedelta(days=i)] for i in range(1, 31)]
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

    def test_diagnostic_decomposition_and_saved_control_truth_validation(self):
        record = self.record()
        detail = diagnostic_row(record, self.company)
        self.assertIn(
            detail["dominant_component"],
            {"inflow_amount", "inflow_timing", "outflow_amount", "outflow_timing"},
        )
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder)
            write_predictions(output, [record])
            self.assertEqual(
                saved_control(output / "predictions.csv", "mean_90", [record]), [record]
            )
            altered = replace(record, truth=[replace(d, inflow=d.inflow + 1) for d in record.truth])
            with self.assertRaises(DataError):
                saved_control(output / "predictions.csv", "mean_90", [altered])

    def test_product_metrics_null_when_no_gaps_and_gate_rejects_extra_alarm(self):
        metrics = product_metrics([self.record()])
        self.assertIsNone(metrics["new_gap_depth_mae"])
        self.assertIsNone(metrics["mean_warning_days"])
        candidate = {
            **metrics,
            "balance_mae": metrics["balance_mae"] * 0.9,
            "new_gap": {**metrics["new_gap"], "fp": 1},
        }
        self.assertIn(
            "new_gap_fp",
            gates(
                candidate, metrics, {"frequent": candidate}, {"frequent": metrics}, {"improved": 1}
            ),
        )

    def test_human_sample_blinded_and_excludes_prior_review(self):
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder)
            prior = output / "prior.json"
            excluded = self.company.operations[0].operation_id
            prior.write_text(json.dumps([{"operation_id": excluded}]))
            summary = human_sample(output, [self.company], prior)
            text = (output / "human_audit/blind_A.csv").read_text(encoding="utf-8-sig")
            self.assertNotIn("rule_category", text)
            with (output / "human_audit/blind_A.csv").open(encoding="utf-8-sig") as stream:
                rows = list(csv.DictReader(stream, delimiter=";"))
            self.assertNotIn(excluded, {r["operation_id"] for r in rows})
            self.assertTrue(all(not r["human_category"] for r in rows))
            self.assertFalse(summary["human_labels_completed"])

    def test_independent_audit_detects_one_cent_tampering(self):
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder)
            data, reference = output / "data", output / "reference"
            data.mkdir()
            reference.mkdir()
            write_csv(data / "companies.csv", [{"company_id": "C001", "industry": "Example"}])
            write_csv(
                data / "all_operations.csv",
                [
                    {
                        "company_id": op.company,
                        "operation_id": op.operation_id,
                        "operation_date": op.day.isoformat(),
                        "amount": money(op.amount),
                        "direction": "Поступление" if op.incoming else "Списание",
                        "counterparty_inn": op.inn,
                        "counterparty_name": op.name,
                        "payment_purpose": op.purpose,
                        "balance_eod": money(op.balance),
                    }
                    for op in self.company.operations
                ],
            )
            write_predictions(output, [self.record()])
            write_predictions(reference, [self.record()])
            write_csv(
                output / "error_decomposition.csv",
                [
                    {
                        "company_id": "C001",
                        "cutoff": self.cutoff.isoformat(),
                        "receipt_type": "frequent",
                    }
                ],
            )
            (output / "explanations.json").write_text("[]")
            (output / "manifest.json").write_text(
                json.dumps(
                    {
                        "source_sha256": {
                            name: sha256(data / name)
                            for name in ("companies.csv", "all_operations.csv")
                        }
                    }
                )
            )
            self.assertTrue(
                audit(output, reference / "predictions.csv", data)["cash_arithmetic_exact"]
            )
            with (output / "predictions.csv").open(encoding="utf-8-sig") as stream:
                rows = list(csv.DictReader(stream, delimiter=";"))
            rows[-1]["predicted_balance"] = money(parse_money(rows[-1]["predicted_balance"]) + 1)
            write_csv(output / "predictions.csv", rows)
            with self.assertRaises(DataError):
                audit(output, reference / "predictions.csv", data)


if __name__ == "__main__":
    unittest.main()
