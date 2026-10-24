"""Check receipt semantics, past-only model choice and single-account conservation."""

import sys
import unittest
from dataclasses import replace
from datetime import date, timedelta
from fractions import Fraction
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from cashflow_baseline.categories import categorize  # noqa: E402
from cashflow_baseline.data import Company, Daily, DataError, Operation, history_at  # noqa: E402
from cashflow_baseline.inflow import (  # noqa: E402
    adaptive_predict,
    inflow_predict,
    round_fraction,
    seasonal_predict,
)
from cashflow_baseline.models import predict  # noqa: E402


def op(purpose, incoming=True, name="Контрагент"):
    return Operation(
        "C001", "example", date(2026, 1, 1), 100000, incoming, "0012345678", name, purpose, 1000000
    )


class AuditedCategoryTests(unittest.TestCase):
    def test_explicit_receipt_sources_are_not_sales_or_own_transfers(self):
        cases = [
            ("Взнос в уставный капитал ООО", "Учредитель", "owner_capital"),
            (
                "Предоставление беспроцентного займа по договору № 12",
                "Учредитель (физическое лицо)",
                "owner_financing",
            ),
            (
                "Выплата страхового возмещения по полису № 1",
                "Страховая компания",
                "insurance_receipt",
            ),
            ("Субсидия на возмещение части затрат", "Министерство", "grant_receipt"),
            ("Грант на развитие субъекта МСП", "Фонд", "grant_receipt"),
            ("Пополнение счета собственными средствами", "ИП", "self_funding_unknown"),
            ("Возврат гарантийного удержания по договору", "Заказчик", "retention_receipt"),
        ]
        for purpose, name, expected in cases:
            with self.subTest(purpose=purpose):
                self.assertEqual(categorize(op(purpose, name=name)).label, expected)

    def test_generic_settlement_needs_marketplace_context(self):
        purpose = "Перечисление денежных средств по договору оферты за период"
        self.assertEqual(
            categorize(op(purpose, name="ООО Маркетплейс-568")).label, "marketplace_revenue"
        )
        self.assertEqual(categorize(op(purpose, name="ООО Другой контрагент")).label, "unknown")
        self.assertEqual(categorize(op("", name="ООО Маркетплейс-568")).label, "unknown")

    def test_personal_and_dividend_flows_are_owner_withdrawals(self):
        for purpose in ["Перевод на личный счет предпринимателя", "Выплата дивидендов участнику"]:
            self.assertEqual(categorize(op(purpose, incoming=False)).label, "owner_withdrawal")

    def test_booking_and_capital_or_debt_contexts_are_separate(self):
        self.assertEqual(
            categorize(op("Перечисление по агентскому договору за проживание гостей")).label,
            "agency_revenue",
        )
        self.assertEqual(
            categorize(op("Возврат платежа: неверно указаны реквизиты")).label, "refund_in"
        )
        self.assertNotEqual(
            categorize(op("Пополнение рекламного кабинета", incoming=False)).label,
            "self_funding_unknown",
        )

    def test_closure_is_candidate_not_verified_pair(self):
        self.assertEqual(
            categorize(
                op(
                    "Перечисление остатка денежных средств в связи с закрытием счета на счет в банке",
                    incoming=False,
                )
            ).label,
            "own_transfer_candidate",
        )


class InflowTests(unittest.TestCase):
    def history(self, value=840000):
        return [
            Daily(date(2026, 1, 1) + timedelta(days=i), value, 10000, 10000000, True, False, 0)
            for i in range(90)
        ]

    def test_exact_half_up_rounding(self):
        self.assertEqual(round_fraction(Fraction(5, 2)), 3)
        self.assertEqual(round_fraction(Fraction(4, 3)), 1)

    def test_stable_receipts_and_baseline_outflow_conserve_balance(self):
        history = self.history()
        predictions = inflow_predict(history, 30, "robust_12w_50")
        balance = history[-1].balance
        for actual, baseline in zip(predictions, predict(history, 30, "mean_90")):
            self.assertEqual(actual.inflow, 840000)
            self.assertEqual(actual.outflow, baseline.outflow)
            balance += actual.inflow - actual.outflow
            self.assertEqual(actual.balance, balance)

    def test_rare_receipt_keeps_positive_mean_component(self):
        history = self.history(0)
        history[-10] = replace(history[-10], inflow=840000)
        result = inflow_predict(history, 30, "robust_12w_50")
        self.assertTrue(all(p.inflow == 5000 for p in result))

    def test_zero_flow_is_zero_and_quality_gaps_are_rejected(self):
        history = self.history(0)
        self.assertTrue(all(p.inflow == 0 for p in inflow_predict(history, 30, "robust_12w_50")))
        history[-2] = replace(history[-2], gap=True)
        with self.assertRaises(DataError):
            inflow_predict(history, 30, "robust_12w_50")

    def company(self):
        operations, balance = [], 1000000000
        start = date(2025, 1, 1)
        for i in range(240):
            amount = 100000 if i < 212 else 50000
            balance += amount
            operations.append(
                Operation(
                    "C001",
                    str(i),
                    start + timedelta(days=i),
                    amount,
                    True,
                    "0012345678",
                    "Покупатель",
                    "Оплата",
                    balance,
                )
            )
        return Company("C001", "Example", operations)

    def test_adaptive_uses_only_past_and_current_fold_outcomes(self):
        company = self.company()
        cutoff = company.operations[210].day
        history, reason = history_at(company, cutoff, 90)
        self.assertFalse(reason)
        changed = replace(
            company,
            operations=[
                replace(o, amount=999999999, purpose="Заем", balance=-999999999)
                if o.day > cutoff
                else o
                for o in company.operations
            ],
        )
        forecast, evidence = adaptive_predict(company, history, 30)
        self.assertEqual((forecast, evidence), adaptive_predict(changed, history, 30))
        self.assertEqual(evidence["selected"], "mean_90")
        self.assertTrue(
            all(
                date.fromisoformat(day) + timedelta(days=28) <= cutoff
                for day in evidence["fold_cutoffs"]
            )
        )

    def test_insufficient_inner_history_falls_back_without_dropping_window(self):
        company = self.company()
        cutoff = company.operations[90].day
        history, _ = history_at(company, cutoff, 90)
        forecast, evidence = adaptive_predict(company, history, 30)
        self.assertEqual(evidence["selected"], "mean_90")
        self.assertEqual(forecast, predict(history, 30, "mean_90"))

    def test_seasonal_fallback_is_identical_without_annual_evidence(self):
        company = self.company()
        cutoff = company.operations[210].day
        history, _ = history_at(company, cutoff, 90)
        forecast, evidence = seasonal_predict(company, history, 30)
        self.assertEqual(forecast, predict(history, 30, "mean_90"))
        self.assertEqual(evidence["annual_evidence_days"], 0)

    def test_seasonal_keeps_own_receipts_and_cannot_see_future(self):
        start, balance, operations = date(2024, 1, 1), 1000000000, []
        for i in range(530):
            day = start + timedelta(days=i)
            balance += 190000
            for suffix, amount, purpose in [
                ("sale", 100000, "Оплата покупателя"),
                ("own", 90000, "Перевод собственных средств с расчетного счета в банке"),
            ]:
                operations.append(
                    Operation(
                        "C001",
                        str(i) + suffix,
                        day,
                        amount,
                        True,
                        "0012345678",
                        "Компания",
                        purpose,
                        balance,
                    )
                )
        company = Company("C001", "Example", operations)
        cutoff = start + timedelta(days=499)
        history, _ = history_at(company, cutoff, 90)
        forecast, evidence = seasonal_predict(company, history, 30)
        self.assertEqual(evidence["annual_evidence_days"], 30)
        self.assertEqual(evidence["other_receipts_90d_cents"], 90 * 90000)
        self.assertTrue(all(p.inflow == 190000 for p in forecast))
        changed = replace(
            company,
            operations=[
                replace(o, amount=999999999, balance=-999999999, purpose="Заем")
                if o.day > cutoff
                else o
                for o in operations
            ],
        )
        self.assertEqual((forecast, evidence), seasonal_predict(changed, history, 30))
        balance = history[-1].balance
        for p in forecast:
            balance += p.inflow - p.outflow
            self.assertEqual(p.balance, balance)


if __name__ == "__main__":
    unittest.main()
