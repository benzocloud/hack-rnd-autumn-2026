"""Deterministic synthetic XML, using only the researched document profiles."""

import datetime as dt
import uuid
import xml.etree.ElementTree as ET
from pathlib import Path

from .common import ED_NS, ImportFailure, day, digest, major, money, write_json

INN = "0123456789"
KPP = "012301001"
PROFILES = {
    "ed_order_1_25_subset": "Этапы оплаты заказа, подмножество EnterpriseData 1.25",
    "ed_expense_1_25_subset": "Заявка на расход, подмножество EnterpriseData 1.25",
    "fns_ens_5_03_subset": "Предстоящие начисления ЕНС 1160082; описание, без оригинальной XSD",
    "fns_notification_5_03": "Уведомление 1110355: налоговое свидетельство, без срока оплаты",
}
ET.register_namespace("ed", ED_NS)


def element(parent, name, value=None, **attributes):
    child = ET.SubElement(parent, name, attributes)
    if value is not None:
        child.text = str(value)
    return child


def ed(parent, name, value=None):
    return element(parent, f"{{{ED_NS}}}{name}", value)


def ed_keys(root, document_id, as_of):
    keys = ed(root, "КлючевыеСвойства")
    ed(keys, "Ссылка", document_id)
    ed(keys, "Дата", f"{as_of}T09:00:00")
    ed(keys, "Номер", "ДЕМО-" + document_id[:8])
    organization = ed(keys, "Организация")
    ed(organization, "ИНН", INN)


def ed_currency(parent):
    currency = ed(parent, "Валюта")
    ed(currency, "Код", "643")


def order(document_id, row_id, as_of, due):
    root = ET.Element(f"{{{ED_NS}}}Документ.ЗаказКлиента")
    ed_keys(root, document_id, as_of)
    ed_currency(root)
    ed(root, "Сумма", "150000.00")
    counterparty = ed(root, "Контрагент")
    ed(counterparty, "Наименование", "Учебный покупатель")
    rows = ed(root, "ЭтапыОплаты")
    row = ed(rows, "Строка")
    ed(row, "НомерСтрокиДокумента", "1")
    ed(row, "ВариантОплаты", "Кредит")
    ed(row, "ФормаОплаты", "Безналичная")
    ed(row, "ДатаПлатежа", due)
    ed(row, "СуммаПлатежа", "150000.00")
    ed(row, "ИдентификаторСтроки", row_id)
    return root


def expense(document_id, as_of, due, amount, description):
    root = ET.Element(f"{{{ED_NS}}}Документ.ЗаявкаНаРасходДСРасчетыСКонтрагентами")
    ed_keys(root, document_id, as_of)
    common = ed(ed(root, "ОбщиеДанные"), "ОбщиеДанные")
    ed(common, "Статус", "КОплате")
    ed(common, "Сумма", major(amount))
    ed_currency(common)
    ed(common, "НазначениеПлатежа", description)
    ed(common, "КрайнийСрокПлатежа", due)
    ed(common, "УтвержденнаяДатаПлатежа", due)
    counterparty = ed(root, "Контрагент")
    ed(counterparty, "Наименование", "Учебный контрагент")
    return root


def taxpayer(doc, with_name=False):
    attributes = {"ИННЮЛ": INN, "КПП": KPP}
    if with_name:
        attributes["НаимОрг"] = "ООО Демо денежных потоков"
    element(element(doc, "СвНП"), "НПЮЛ", **attributes)


def ens(document_id, as_of, due, gross, earmarked):
    """A source-shaped subset, deliberately not advertised as a complete FNS file."""
    root = ET.Element("Файл", {"ИдФайл": document_id, "ВерсФорм": "5.03"})
    doc = element(
        root,
        "Документ",
        КНД="1160082",
        НомСпр=document_id[:20],
        ДатаСостСпр=day(as_of).strftime("%d.%m.%Y"),
        ВремяСостСпр="18:00",
        СальдоЕНС=major(earmarked),
        СальдоЕНС2023="0.00",
    )
    taxpayer(doc, with_name=True)
    parent = element(doc, "ИнфПрОбязЗачт")
    forthcoming = element(parent, "ПредОбяз", СуммаВс=major(gross), СумЗачтОбязВс=major(earmarked))
    attributes = {
        "КБК": "18210501011011000110",
        "ОКТМО": "45382000",
        "КПП": KPP,
        "Сумма": major(gross),
        "СумЗачтОбяз": major(earmarked),
    }
    if due is not None:
        attributes["СрокУплат"] = day(due).strftime("%d.%m.%Y")
    element(forthcoming, "ИнфПредНачисл", **attributes)
    return root


def notification(document_id, as_of, amount):
    root = ET.Element(
        "Файл", {"ИдФайл": document_id, "ВерсПрог": "CashflowMock", "ВерсФорм": "5.03"}
    )
    doc = element(
        root, "Документ", КНД="1110355", ДатаДок=day(as_of).strftime("%d.%m.%Y"), КодНО="7700"
    )
    taxpayer(doc)
    signer = element(doc, "Подписант", ПрПодп="1")
    element(signer, "ФИО", Фамилия="Учебный", Имя="Подписант")
    element(
        doc,
        "УвИсчСумНалог",
        КППДекл=KPP,
        ОКТМО="45382000",
        КБК="18210501011011000110",
        СумНалогАванс=major(amount),
        Период="34",
        НомерМесКварт="01",
        Год=str(day(as_of).year),
    )
    return root


def generate(directory, seed=42, as_of="2026-08-31"):
    as_of = day(as_of)
    if not isinstance(seed, int) or not 0 <= seed <= 2**32 - 1:
        raise ImportFailure("INVALID_SEED", "Seed должен быть целым от 0 до 2^32−1")
    if as_of > dt.date(2099, 10, 1) or as_of < dt.date(1900, 1, 1):
        raise ImportFailure("INVALID_DATE", "Дата вне диапазона демонстрационных форматов")
    root = Path(directory)
    if root.exists() and any(root.iterdir()):
        raise ImportFailure("OUTPUT_NOT_EMPTY", "Для генерации нужен новый или пустой каталог")
    documents = root / "documents"
    documents.mkdir(parents=True, exist_ok=True)

    def identity(label):
        return str(uuid.uuid5(uuid.NAMESPACE_URL, f"cashflow-demo/{seed}/{as_of}/{label}"))

    def due(offset):
        return (as_of + dt.timedelta(days=offset)).isoformat()

    known_date = as_of.isoformat()
    manifest = {
        "schema_version": "0.1.0",
        "demo": True,
        "seed": seed,
        "company_id": "co_demo",
        "account_id": "acc_demo",
        "source_company_id": "DEMO-001",
        "company_inn": INN,
        "currency": "RUB",
        "cutoff_date": known_date,
        "horizon_days": 30,
        "opening_balance": money(20_000_000),
        "assumptions": [
            "Все компании, суммы, даты, остатки исполнения и документы синтетические.",
            "ED/ЕНС: ограниченные документные профили, не полный обмен с реальной системой.",
            "Мок остатка исполнения 1С задаётся явно в манифесте, не выводится из суммы этапа.",
            "Зачёт в строке ЕНС считается уже обеспеченным деньгами; в план входит незачтённая часть.",
            "Все перечисленные выплаты в демо помечены обязательными самим сценарием.",
        ],
        "documents": [],
    }

    def save(label, profile, xml, assumptions):
        document_id = identity(label)
        encoding = "windows-1251" if profile.startswith("fns_") else "utf-8"
        content = ET.tostring(xml, encoding=encoding, xml_declaration=True)
        relative = f"documents/{label}.xml"
        (root / relative).write_bytes(content)
        manifest["documents"].append(
            {
                "id": document_id,
                "profile": profile,
                "path": relative,
                "sha256": digest(content),
                "known_as_of_date": known_date,
                "mandatory": profile != "ed_order_1_25_subset",
                "record_assumptions": assumptions,
            }
        )

    row_id = identity("receipt-stage")
    save(
        "receipt",
        "ed_order_1_25_subset",
        order(identity("receipt"), row_id, known_date, due(20)),
        {row_id: {"paid_minor": "0"}},
    )
    for label, offset, amount, paid, description in [
        ("rent", 5, 8_000_000, 2_000_000, "Аренда: общий платёж 80 000, оплачено 20 000"),
        ("supplier", 10, 6_000_000, 0, "Обязательная оплата поставщику"),
        ("payroll", 15, 10_000_000, 0, "Обязательная выплата зарплаты"),
    ]:
        save(
            label,
            "ed_expense_1_25_subset",
            expense(identity(label), known_date, due(offset), amount, description),
            {identity(label): {"paid_minor": str(paid), "date_choice": "approved"}},
        )
    save(
        "ens",
        "fns_ens_5_03_subset",
        ens(identity("ens"), known_date, due(28), 6_000_000, 4_000_000),
        {},
    )
    save(
        "ens-no-date",
        "fns_ens_5_03_subset",
        ens(identity("ens-no-date"), known_date, None, 500_000, 0),
        {},
    )
    save(
        "notification",
        "fns_notification_5_03",
        notification(identity("notification"), known_date, 4_500_000),
        {},
    )
    save(
        "notification-correction",
        "fns_notification_5_03",
        notification(identity("notification-correction"), known_date, -1_500_000),
        {},
    )
    write_json(root / "manifest.json", manifest)
    return manifest
