"""Check category precedence, as-of evidence and conservation of calendar flows."""

import sys
import unittest
from dataclasses import replace
from datetime import date, timedelta
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from cashflow_baseline.categories import categorize  # noqa: E402
from cashflow_baseline.data import Company, Operation, history_at  # noqa: E402
from cashflow_baseline.recurring import (  # noqa: E402
    fit_monthly,
    future_events,
    hybrid_predict,
    infer_patterns,
    scheduled_date,
)


def operation(day, purpose, amount=100000, incoming=False, identifier=None):
    return Operation(
        "C001",
        identifier or day.isoformat(),
        day,
        amount,
        incoming,
        "0012345678",
        "Банк",
        purpose,
        10000000,
    )


def monthly_company():
    start, end = date(2026, 1, 1), date(2026, 8, 31)
    items, balance, cursor = [], 10000000, start
    while cursor <= end:
        entries = [(100000, True, "", "daily-in"), (10000, False, "", "daily-out")]
        if cursor == scheduled_date(cursor.year, cursor.month, 15, "previous"):
            entries.append((100000, False, "Перечисление заработной платы (аванс)", "advance"))
        if cursor == scheduled_date(cursor.year, cursor.month, 0, "previous"):
            entries.append(
                (150000, False, "Перечисление заработной платы (окончательный расчет)", "salary")
            )
        if cursor == scheduled_date(cursor.year, cursor.month, 1, "next"):
            entries.extend(
                [
                    (50000, False, "Оплата аренды помещения", "rent"),
                    (1000, False, "Комиссия за ведение счета", "fee"),
                ]
            )
        balance += sum(amount if incoming else -amount for amount, incoming, _, _ in entries)
        for amount, incoming, purpose, suffix in entries:
            item = operation(cursor, purpose, amount, incoming, cursor.isoformat() + suffix)
            items.append(replace(item, balance=balance))
        cursor += timedelta(days=1)
    return Company("C001", "Example", items)


class CategoryTests(unittest.TestCase):
    def test_explicit_tax_mentioning_wages_is_not_payroll(self):
        item = operation(date(2026, 1, 1), "Перечисление налога с заработной платы")
        self.assertEqual(categorize(item).label, "tax")

    def test_generic_vat_disclaimer_does_not_mean_tax(self):
        item = operation(date(2026, 1, 1), "Оплата товара. Без налога (НДС)")
        self.assertNotEqual(categorize(item).label, "tax")
        self.assertEqual(categorize(replace(item, purpose="Единый налоговый платёж")).label, "tax")

    def test_bank_counterparty_does_not_merge_payroll_and_fees(self):
        day = date(2026, 1, 1)
        labels = [
            categorize(operation(day, text)).label
            for text in [
                "Перечисление заработной платы (аванс)",
                "Перечисление заработной платы (окончательный расчет)",
                "Комиссия за ведение счета",
            ]
        ]
        self.assertEqual(labels, ["salary_advance", "salary_final", "bank_fee"])

    def test_acquiring_with_commission_and_zero_refund_is_revenue(self):
        item = operation(
            date(2026, 1, 1),
            "Зачисление эквайринга. Мерчант 12, комиссия 200, возврат покупки 0",
            incoming=True,
        )
        self.assertEqual(categorize(item).label, "acquiring_revenue")

    def test_owner_funding_and_borrowing_are_not_customer_revenue(self):
        day = date(2026, 1, 1)
        self.assertEqual(
            categorize(
                operation(day, "Предоставление беспроцентного займа учредителем", incoming=True)
            ).label,
            "owner_financing",
        )
        self.assertEqual(
            categorize(operation(day, "Зачисление кредита", incoming=True)).label, "loan_receipt"
        )
        self.assertEqual(
            categorize(operation(day, "Возврат беспроцентного займа учредителю")).label,
            "owner_repayment",
        )

    def test_unknown_is_preserved(self):
        for purpose in ["", "НДС не облагается", "Непонятная операция"]:
            self.assertEqual(categorize(operation(date(2026, 1, 1), purpose)).label, "unknown")


class RecurringTests(unittest.TestCase):
    def test_month_end_and_weekend_boundary(self):
        self.assertEqual(scheduled_date(2026, 2, 28, "next"), date(2026, 3, 2))
        self.assertEqual(scheduled_date(2026, 5, 0, "previous"), date(2026, 5, 29))
        self.assertEqual(scheduled_date(2024, 2, 31, "none"), date(2024, 2, 29))

    def test_one_off_or_same_month_repetitions_do_not_establish_schedule(self):
        items = [
            operation(date(2026, 1, day), "Оплата аренды", identifier=str(day))
            for day in [1, 5, 10, 15]
        ]
        self.assertIsNone(fit_monthly(items, "rent", date(2026, 1, 31)))
        self.assertIsNone(fit_monthly(items[:1], "rent", date(2026, 1, 31)))

    def test_monthly_bank_flows_are_separate_patterns(self):
        patterns = infer_patterns(monthly_company(), date(2026, 6, 25))
        labels = {pattern.category for pattern in patterns}
        self.assertTrue({"salary_advance", "salary_final", "rent", "bank_fee"} <= labels)

    def test_early_paid_period_is_not_forecast_again(self):
        days = [date(2026, 3, 10), date(2026, 4, 10), date(2026, 5, 10), date(2026, 6, 8)]
        items = [operation(day, "Оплата аренды") for day in days]
        pattern = fit_monthly(items, "rent", date(2026, 6, 8))
        self.assertIsNotNone(pattern)
        self.assertEqual(future_events([pattern], date(2026, 6, 8), 14), [])

    def test_tiny_tax_penalties_stay_outside_main_schedule(self):
        items = []
        for month in range(1, 6):
            items.append(
                operation(
                    scheduled_date(2026, month, 28, "next"),
                    "ЕНП",
                    100000,
                    identifier=f"tax-{month}",
                )
            )
            items.append(
                operation(date(2026, month, 17), "ЕНП", 100, identifier=f"penalty-{month}")
            )
        pattern = fit_monthly(items, "tax", date(2026, 6, 1))
        self.assertIsNotNone(pattern)
        self.assertFalse(
            any(identifier.startswith("penalty") for identifier in pattern.matched_ids)
        )

    def test_hybrid_does_not_count_regular_expenses_twice(self):
        source = monthly_company()
        history, reason = history_at(source, date(2026, 6, 25), 90)
        self.assertFalse(reason)
        predictions, _, events = hybrid_predict(source, history, 30)
        balance = history[-1].balance
        for prediction in predictions:
            expected_out = 10000 + sum(
                e.amount for e in events if e.day == prediction.day and not e.incoming
            )
            self.assertEqual(prediction.inflow, 100000)
            self.assertEqual(prediction.outflow, expected_out)
            balance += prediction.inflow - prediction.outflow
            self.assertEqual(prediction.balance, balance)

    def test_future_mutation_cannot_change_categories_learned_patterns_or_forecast(self):
        source, cutoff = monthly_company(), date(2026, 6, 25)
        history, _ = history_at(source, cutoff, 90)
        changed = Company(
            source.company_id,
            source.industry,
            [
                replace(
                    op, amount=900000000, purpose="Крупное погашение кредита", balance=-999999999
                )
                if op.day > cutoff
                else op
                for op in source.operations
            ],
        )
        self.assertEqual(hybrid_predict(source, history, 30), hybrid_predict(changed, history, 30))


if __name__ == "__main__":
    unittest.main()
