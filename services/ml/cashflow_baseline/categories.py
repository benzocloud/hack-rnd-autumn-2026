"""Transparent text rules; confidence scores are heuristics, not measured accuracy."""

import re
import unicodedata
from dataclasses import dataclass
from functools import lru_cache

from .data import Operation


@dataclass(frozen=True)
class Category:
    label: str
    confidence: float
    rule: str


def normalize(text: str) -> str:
    return " ".join(unicodedata.normalize("NFKC", text).lower().replace("ё", "е").split())


@lru_cache(maxsize=32768)
def categorize(operation: Operation) -> Category:
    text = normalize(operation.purpose)
    if not text:
        return Category("unknown", 0.0, "empty_purpose")
    if "пополнение счета собственными средствами" in text:
        return Category("self_funding_unknown", 0.5, "self_funding_source_unspecified")
    if operation.incoming and "взнос в уставный капитал" in text:
        return Category("owner_capital", 0.95, "explicit_capital_contribution")
    if operation.incoming and "учредитель" in normalize(operation.name) and "овердрафт" in text:
        return Category("owner_financing", 0.8, "owner_overdraft_support")
    if re.search(r"предоставление.*займа учредителем", text):
        return Category(
            "owner_financing" if operation.incoming else "owner_repayment", 0.95, "owner_funding"
        )
    if re.search(r"возврат.*займа учредител", text):
        return Category(
            "owner_repayment" if not operation.incoming else "loan_receipt",
            0.95,
            "owner_loan_return",
        )
    if "беспроцентного займа" in text and "учредител" in normalize(operation.name):
        return Category(
            "owner_financing" if operation.incoming else "owner_repayment",
            0.9,
            "owner_named_counterparty_loan",
        )
    if not operation.incoming and re.search(
        r"личн.{0,20}(?:счет|карт)|счет физическ|выплата дивиденд", text
    ):
        return Category("owner_withdrawal", 0.95, "personal_transfer_or_dividend")
    if "перечисление остатка" in text and "закрытием счета" in text:
        return Category("own_transfer_candidate", 0.6, "closure_transfer_unverified_account")
    if re.search(r"перевод собственных средств|между.*счет", text):
        label = (
            "owner_withdrawal"
            if not operation.incoming and re.search(r"личн|физическ", text)
            else "own_transfer"
        )
        return Category(label, 0.95, "own_funds_transfer")
    if not operation.incoming and re.search(r"процент.*овердрафт", text):
        return Category("overdraft_interest", 0.95, "overdraft_interest")
    if re.search(r"кредит|лизинг", text):
        return Category(
            "loan_receipt" if operation.incoming else "loan_payment", 0.9, "credit_or_lease"
        )
    if operation.incoming and "возврат гарантийного удержания" in text:
        return Category("retention_receipt", 0.9, "contract_retention_release")
    if text.startswith("возврат"):
        return Category("refund_in" if operation.incoming else "refund_out", 0.85, "payment_return")
    if operation.incoming and re.search(r"страхов.*возмещени", text):
        return Category("insurance_receipt", 0.95, "explicit_insurance_compensation")
    if operation.incoming and re.search(r"субсидия|грант на развитие", text):
        return Category("grant_receipt", 0.95, "explicit_grant_or_subsidy")
    if operation.incoming and "агентскому договору" in text and "проживание гостей" in text:
        return Category("agency_revenue", 0.9, "booking_agency_settlement")
    if not operation.incoming and "исполнительному листу" in text:
        return Category("enforcement_payment", 0.9, "explicit_enforcement_order")
    # An explicit tax remittance can mention wages without being a payroll payment.
    if not operation.incoming and re.search(
        r"единый налог|\bенп\b|страхов.*взнос|уплата.*налог|перечисление.*налог", text
    ):
        return Category("tax", 0.95, "explicit_tax_text")
    if not operation.incoming and re.search(r"заработн|зарплат", text):
        label = (
            "salary_advance"
            if "аванс" in text
            else "salary_final"
            if "окончательн" in text
            else "salary"
        )
        return Category(label, 0.98, "payroll_text")
    if not operation.incoming and "аренд" in text:
        label = (
            "equipment_rent" if re.search(r"pos|терминал|оборудован|автомобил", text) else "rent"
        )
        return Category(label, 0.9, "rental_text")
    if not operation.incoming and re.search(r"комисси|ведение счета|обслуживание счета", text):
        return Category("bank_fee", 0.9, "bank_fee_text")
    if not operation.incoming and re.search(
        r"электроэнерг|водоснаб|коммуналь|отоплен|вывоз.*отход", text
    ):
        return Category("utilities", 0.9, "utility_text")
    if not operation.incoming and re.search(
        r"подписк|услуг.*связи|доступ.*интернет|абонентск|использования по", text
    ):
        return Category("subscription", 0.8, "subscription_text")
    if operation.incoming and re.search(r"эквайринг|мерчант|сбп|qr-код", text):
        return Category("acquiring_revenue", 0.95, "acquiring_text")
    if operation.incoming and re.search(r"торговая выручка|инкассированная выручка", text):
        return Category("cash_revenue", 0.9, "cash_revenue_text")
    if operation.incoming and (
        re.search(r"маркетплейс|маркет.плейс|реестр продаж|отчет.*агента", text)
        or (
            "маркетплейс" in normalize(operation.name)
            and "перечисление денежных средств по договору оферты" in text
        )
    ):
        return Category("marketplace_revenue", 0.8, "marketplace_text")
    if operation.incoming and re.search(r"оплат|аванс|окончательный расчет|поступлен", text):
        return Category("customer_payment", 0.65, "generic_customer_payment")
    if not operation.incoming and re.search(
        r"оплат|закуп|пополнение|аванс по договору подряда|окончательный расчет по договору", text
    ):
        return Category("operating_expense", 0.55, "generic_operating_expense")
    return Category("unknown", 0.0, "no_matching_rule")
