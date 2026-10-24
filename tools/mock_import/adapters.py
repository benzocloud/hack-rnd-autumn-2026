"""Strict adapters for the documented synthetic source subsets."""

import datetime as dt
import xml.etree.ElementTree as ET

from .common import ED_NS, MAX_FILE_BYTES, ImportFailure, day, fns_day, integer_minor, minor, money


def qpath(path):
    return "/".join(f"{{{ED_NS}}}{part}" for part in path.split("/"))


def one(root, path, required=True):
    nodes = root.findall(path)
    if len(nodes) > 1:
        raise ImportFailure("AMBIGUOUS_FIELD", f"Повтор поля: {path}")
    if not nodes:
        if required:
            raise ImportFailure("MISSING_FIELD", f"Отсутствует поле: {path}")
        return None
    return nodes[0]


def text(root, path, required=True):
    node = one(root, path, required)
    value = node.text if node is not None else None
    if required and not value:
        raise ImportFailure("MISSING_FIELD", f"Пустое поле: {path}")
    return value


def parse_xml(content):
    if len(content) > MAX_FILE_BYTES:
        raise ImportFailure("FILE_TOO_LARGE", "XML превышает 2 MiB")
    compact = content.replace(b"\x00", b"").upper()
    if b"<!DOCTYPE" in compact or b"<!ENTITY" in compact:
        raise ImportFailure("UNSAFE_XML", "DTD и объявления сущностей запрещены")
    try:
        root = ET.fromstring(content)
    except (ET.ParseError, ValueError, LookupError) as exc:
        raise ImportFailure("INVALID_XML", "Не удалось прочитать XML") from exc
    if sum(1 for _ in root.iter()) > 10000:
        raise ImportFailure("DOCUMENT_TOO_COMPLEX", "Слишком много элементов XML")
    return root


def check_inn(inn, context):
    if inn != context["company_inn"]:
        raise ImportFailure("COMPANY_MISMATCH", "ИНН документа отличается от выбранной компании")


def payment(record_id, direction, total, remaining, due, name, description, meta):
    if total <= 0 or not 0 <= remaining <= total:
        raise ImportFailure("INVALID_REMAINDER", "Некорректная общая/оставшаяся сумма")
    day(due)
    if not isinstance(record_id, str) or not 1 <= len(record_id) <= 128:
        raise ImportFailure("INVALID_ID", "Некорректный ID платежа")
    if (
        not isinstance(name, str)
        or len(name) > 500
        or not isinstance(description, str)
        or len(description) > 2000
    ):
        raise ImportFailure("INVALID_FIELD", "Наименование/описание вне ограничений DTO")
    status = "planned" if remaining == total else "partially_matched"
    if remaining == 0:
        status = "matched"
    return {
        "external_payment_id": record_id,
        "direction": direction,
        "amount": money(total),
        "remaining_amount": money(remaining),
        "scheduled_date": due,
        "counterparty_name": name,
        "description": description,
        "source_category": None,
        "status": status,
        "availability": {
            "known_as_of_date": meta["known_as_of_date"],
            "basis": "demo_assumption",
            "evidence_ref": f"{meta['path']} + явные допущения manifest.json",
        },
    }


def check_ed(root, meta, context):
    identifier = text(root, qpath("КлючевыеСвойства/Ссылка"))
    if identifier != meta["id"]:
        raise ImportFailure("DOCUMENT_ID_MISMATCH", "Ссылка документа не совпадает с манифестом")
    check_inn(text(root, qpath("КлючевыеСвойства/Организация/ИНН")), context)
    raw_date = text(root, qpath("КлючевыеСвойства/Дата"))
    try:
        timestamp = dt.datetime.fromisoformat(raw_date)
    except ValueError as exc:
        raise ImportFailure("INVALID_DATE", "Некорректная дата документа 1С") from exc
    if timestamp.tzinfo is not None:
        raise ImportFailure(
            "UNSUPPORTED_TIMEZONE", "Демо-профиль ожидает явное местное время без offset"
        )
    if timestamp.date() > day(context["cutoff_date"]):
        raise ImportFailure("AFTER_CUTOFF", "Документ 1С появился после среза")
    return identifier


def check_currency(root):
    code = text(root, qpath("Валюта/Код"))
    if code not in {"643", "RUB"}:
        raise ImportFailure(
            "UNSUPPORTED_CURRENCY", "Демо поддерживает только явно указанную RUB/643"
        )


def paid_assumption(meta, record_id):
    assumption = meta["record_assumptions"].get(record_id)
    if not isinstance(assumption, dict) or "paid_minor" not in assumption:
        raise ImportFailure("MISSING_EXECUTION_STATE", "Нет явного демо-остатка исполнения")
    return integer_minor(assumption["paid_minor"]), assumption


def parse_order(root, meta, context):
    if root.tag != f"{{{ED_NS}}}Документ.ЗаказКлиента":
        raise ImportFailure("WRONG_PROFILE", "Ожидается заказ клиента ED 1.25")
    identifier = check_ed(root, meta, context)
    check_currency(root)
    name = text(root, qpath("Контрагент/Наименование"))
    parent = one(root, qpath("ЭтапыОплаты"))
    rows = parent.findall(qpath("Строка"))
    if not rows or len(rows) > 1000:
        raise ImportFailure("INVALID_ROW_COUNT", "Нет этапов оплаты или превышен лимит 1000")
    result, ids = [], set()
    for row in rows:
        row_id = text(row, qpath("ИдентификаторСтроки"))
        if row_id in ids:
            raise ImportFailure("DUPLICATE_RECORD_ID", "Повтор идентификатора этапа в документе")
        ids.add(row_id)
        total = minor(text(row, qpath("СуммаПлатежа")))
        paid, _ = paid_assumption(meta, row_id)
        result.append(
            payment(
                f"{identifier}:{row_id}",
                "inflow",
                total,
                total - paid,
                text(row, qpath("ДатаПлатежа")),
                name,
                "Оплата по учебному заказу клиента",
                meta,
            )
        )
    return result, []


def parse_expense(root, meta, context):
    if root.tag != f"{{{ED_NS}}}Документ.ЗаявкаНаРасходДСРасчетыСКонтрагентами":
        raise ImportFailure("WRONG_PROFILE", "Ожидается заявка на расход ED 1.25")
    identifier = check_ed(root, meta, context)
    common = one(root, qpath("ОбщиеДанные/ОбщиеДанные"))
    check_currency(common)
    if text(common, qpath("Статус")) != "КОплате":
        raise ImportFailure("EXPENSE_NOT_APPROVED", "Заявка не подтверждена к оплате")
    paid, assumption = paid_assumption(meta, identifier)
    approved = text(common, qpath("УтвержденнаяДатаПлатежа"), False)
    deadline = text(common, qpath("КрайнийСрокПлатежа"), False)
    if assumption.get("date_choice") != "approved" or not approved:
        raise ImportFailure("NEEDS_DUE_DATE", "Нужен явный выбор подтверждённой даты оплаты")
    if deadline and day(approved) > day(deadline):
        raise ImportFailure("DATE_AFTER_DEADLINE", "Дата оплаты позже крайнего срока")
    total = minor(text(common, qpath("Сумма")))
    record = payment(
        identifier,
        "outflow",
        total,
        total - paid,
        approved,
        text(root, qpath("Контрагент/Наименование")),
        text(common, qpath("НазначениеПлатежа")),
        meta,
    )
    return [record], []


def fns_document(root, meta, context, knd):
    if root.tag != "Файл" or root.get("ВерсФорм") != "5.03":
        raise ImportFailure("WRONG_PROFILE", "Ожидается файл ФНС версии 5.03")
    if root.get("ИдФайл") != meta["id"]:
        raise ImportFailure("DOCUMENT_ID_MISMATCH", "ID файла не совпадает с манифестом")
    doc = one(root, "Документ")
    if doc.get("КНД") != knd:
        raise ImportFailure("WRONG_PROFILE", "КНД не соответствует выбранному профилю")
    check_inn(one(doc, "СвНП/НПЮЛ").get("ИННЮЛ"), context)
    return doc


def parse_ens(root, meta, context):
    doc = fns_document(root, meta, context, "1160082")
    if day(fns_day(doc.get("ДатаСостСпр"))) > day(context["cutoff_date"]):
        raise ImportFailure("AFTER_CUTOFF", "Справка ЕНС сформирована после среза")
    parent = one(doc, "ИнфПрОбязЗачт/ПредОбяз")
    rows = parent.findall("ИнфПредНачисл")
    if not rows or len(rows) > 1000:
        raise ImportFailure(
            "INVALID_ROW_COUNT", "Нет строк предстоящих начислений или превышен лимит"
        )
    records, details = [], []
    for index, row in enumerate(rows, 1):
        if not row.get("СрокУплат"):
            raise ImportFailure(
                "NEEDS_DUE_DATE", "В строке ЕНС нет срока уплаты; дата не придумана"
            )
        gross, earmarked = minor(row.get("Сумма")), minor(row.get("СумЗачтОбяз"))
        if earmarked > gross:
            raise ImportFailure("INVALID_REMAINDER", "Зачтённая сумма превышает обязательство")
        record = payment(
            f"{meta['id']}:{index}",
            "outflow",
            gross,
            gross - earmarked,
            fns_day(row.get("СрокУплат")),
            "Учебное налоговое обязательство",
            "ЕНС: незачтённая часть предстоящей обязанности",
            meta,
        )
        records.append(record)
        details.append(
            {
                "external_payment_id": record["external_payment_id"],
                "gross": money(gross),
                "already_earmarked": money(earmarked),
                "bank_cash_needed": money(gross - earmarked),
            }
        )
    return records, details


def parse_notification(root, meta, context):
    doc = fns_document(root, meta, context, "1110355")
    if day(fns_day(doc.get("ДатаДок"))) > day(context["cutoff_date"]):
        raise ImportFailure("AFTER_CUTOFF", "Уведомление сформировано после среза")
    rows = doc.findall("УвИсчСумНалог")
    if not rows:
        raise ImportFailure("MISSING_FIELD", "Нет исчисленных налоговых сумм")
    sums = [minor(row.get("СумНалогАванс"), allow_negative=True) for row in rows]
    if any(amount < 0 for amount in sums):
        raise ImportFailure(
            "TAX_ADJUSTMENT_REQUIRES_REVIEW", "Отрицательная корректировка не стала расходом"
        )
    raise ImportFailure(
        "NEEDS_DUE_DATE", "Уведомление содержит сумму/период, но не срок: требуется уточнение"
    )


ADAPTERS = {
    "ed_order_1_25_subset": ("demo_1c_ed_1_25", parse_order),
    "ed_expense_1_25_subset": ("demo_1c_ed_1_25", parse_expense),
    "fns_ens_5_03_subset": ("demo_fns_ens_5_03", parse_ens),
    "fns_notification_5_03": ("demo_fns_notification_5_03", parse_notification),
}


def adapt(content, meta, context):
    profile = meta.get("profile")
    if profile not in ADAPTERS:
        raise ImportFailure("UNSUPPORTED_PROFILE", "Профиль документа не поддерживается")
    if day(meta["known_as_of_date"]) > day(context["cutoff_date"]):
        raise ImportFailure("AFTER_CUTOFF", "Доступность документа позже даты среза")
    source, adapter = ADAPTERS[profile]
    records, evidence = adapter(parse_xml(content), meta, context)
    return source, records, evidence
