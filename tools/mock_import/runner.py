"""Local import receipt, idempotent mock state and transparent calendar arithmetic."""

import datetime as dt
import fcntl
import re
from collections import defaultdict
from pathlib import Path

from .adapters import ADAPTERS, adapt
from .common import (
    MAX_FILE_BYTES,
    SCHEMA_VERSION,
    ImportFailure,
    bound_path,
    day,
    digest,
    integer_minor,
    money,
    read_json,
    write_json,
)

BINDING_FIELDS = (
    "company_id",
    "account_id",
    "source_company_id",
    "company_inn",
    "currency",
    "cutoff_date",
    "horizon_days",
    "opening_balance",
)


def manifest_at(directory):
    manifest = read_json(Path(directory) / "manifest.json")
    if not isinstance(manifest, dict) or manifest.get("demo") is not True:
        raise ImportFailure("NOT_A_DEMO_BUNDLE", "Ожидается явно помеченный демо-набор")
    if manifest.get("schema_version") != SCHEMA_VERSION or manifest.get("currency") != "RUB":
        raise ImportFailure("UNSUPPORTED_CONVENTION", "Не поддерживается версия или валюта набора")
    if type(manifest.get("seed")) is not int or not 0 <= manifest["seed"] <= 2**32 - 1:
        raise ImportFailure("INVALID_SEED", "В манифесте нужен seed от 0 до 2^32−1")
    for key in ["company_id", "account_id", "source_company_id"]:
        value = manifest.get(key)
        if not isinstance(value, str) or not 1 <= len(value) <= 128:
            raise ImportFailure("INVALID_MANIFEST", f"Некорректное поле {key}")
    if not isinstance(manifest.get("company_inn"), str) or not re.fullmatch(
        r"\d{10}", manifest["company_inn"]
    ):
        raise ImportFailure("INVALID_MANIFEST", "Демо ожидает строковый ИНН организации")
    day(manifest.get("cutoff_date"))
    horizon = manifest.get("horizon_days")
    if type(horizon) is not int or not 30 <= horizon <= 90:
        raise ImportFailure("INVALID_HORIZON", "Горизонт должен быть целым от 30 до 90 дней")
    opening = manifest.get("opening_balance")
    if not isinstance(opening, dict) or opening.get("currency") != "RUB":
        raise ImportFailure("INVALID_AMOUNT", "Нет начального остатка в RUB")
    amount = opening.get("amount_minor")
    if (
        not isinstance(amount, str)
        or not re.fullmatch(r"-?(?:0|[1-9]\d*)", amount)
        or len(amount) > 18
    ):
        raise ImportFailure("INVALID_AMOUNT", "Некорректный начальный остаток")
    documents = manifest.get("documents")
    if not isinstance(documents, list) or not 1 <= len(documents) <= 1000:
        raise ImportFailure("INVALID_MANIFEST", "Нужен список от 1 до 1000 документов")
    ids, paths = set(), set()
    for document in documents:
        if not isinstance(document, dict):
            raise ImportFailure("INVALID_MANIFEST", "Документ манифеста должен быть объектом")
        identifier = document.get("id")
        if not isinstance(identifier, str) or not 1 <= len(identifier) <= 64 or identifier in ids:
            raise ImportFailure("INVALID_MANIFEST", "Некорректный/повторный ID документа")
        ids.add(identifier)
        path = document.get("path")
        bound_path(directory, path)
        if not path.startswith("documents/") or path in paths or not path.endswith(".xml"):
            raise ImportFailure("INVALID_PATH", "Нужен уникальный XML-путь в documents/")
        paths.add(path)
        if not isinstance(document.get("profile"), str) or document["profile"] not in ADAPTERS:
            raise ImportFailure("UNSUPPORTED_PROFILE", "Неизвестный профиль в манифесте")
        if not isinstance(document.get("sha256"), str) or not re.fullmatch(
            r"[a-f0-9]{64}", document["sha256"]
        ):
            raise ImportFailure("INVALID_MANIFEST", "Нет SHA-256 документа")
        if type(document.get("mandatory")) is not bool or not isinstance(
            document.get("record_assumptions"), dict
        ):
            raise ImportFailure("INVALID_MANIFEST", "Нет явных допущений/признака обязательности")
        day(document.get("known_as_of_date"))
    return manifest


def validate_record(record):
    """Only records built by these adapters; validates persisted mock state on read."""
    if not isinstance(record, dict) or record.get("direction") not in {"inflow", "outflow"}:
        raise ImportFailure("INVALID_STATE", "Некорректная запись в состоянии")
    day(record.get("scheduled_date"))
    try:
        total, remaining = record["amount"], record["remaining_amount"]
        if total["currency"] != "RUB" or remaining["currency"] != "RUB":
            raise ImportFailure("UNSUPPORTED_CURRENCY", "В состоянии другая валюта")
        total, remaining = (
            integer_minor(total["amount_minor"]),
            integer_minor(remaining["amount_minor"]),
        )
        if total == 0 or remaining > total:
            raise ImportFailure("INVALID_REMAINDER", "Некорректный остаток записи")
        expected = (
            "matched"
            if remaining == 0
            else "planned"
            if remaining == total
            else "partially_matched"
        )
        if record["status"] not in {expected, "cancelled"}:
            raise ImportFailure("INVALID_STATE", "Статус и остаток не согласованы")
    except (KeyError, TypeError) as exc:
        raise ImportFailure("INVALID_STATE", "Неполная запись состояния") from exc


def cash_plan(entries, manifest):
    start = day(manifest["cutoff_date"]) + dt.timedelta(days=1)
    end = start + dt.timedelta(days=manifest["horizon_days"] - 1)
    by_day, outside = defaultdict(list), []
    for entry in entries:
        record = entry["record"]
        validate_record(record)
        if record["status"] in {"matched", "cancelled"}:
            continue
        due = day(record["scheduled_date"])
        if not start <= due <= end:
            outside.append(
                {
                    "external_payment_id": record["external_payment_id"],
                    "scheduled_date": due.isoformat(),
                    "reason": "overdue_requires_rescheduling" if due < start else "beyond_horizon",
                }
            )
            continue
        by_day[due].append(entry)
    balance = int(manifest["opening_balance"]["amount_minor"])
    points, first_gap, minimum = [], None, None
    for offset in range(manifest["horizon_days"]):
        date = start + dt.timedelta(days=offset)
        incoming = outgoing = mandatory = 0
        references = []
        for entry in by_day[date]:
            record = entry["record"]
            amount = int(record["remaining_amount"]["amount_minor"])
            if record["direction"] == "inflow":
                incoming += amount
            else:
                outgoing += amount
                if entry["mandatory"]:
                    mandatory += amount
            references.append(
                {
                    "source_system": entry["source_system"],
                    "document_id": entry["document_id"],
                    "external_payment_id": record["external_payment_id"],
                }
            )
        before = balance
        balance += incoming - outgoing
        minimum = balance if minimum is None else min(minimum, balance)
        if balance < 0 and first_gap is None:
            first_gap = {"date": date.isoformat(), "shortfall": money(-balance)}
        points.append(
            {
                "date": date.isoformat(),
                "opening_balance": money(before),
                "inflow": money(incoming),
                "outflow": money(outgoing),
                "mandatory_outflow": money(mandatory),
                "closing_balance": money(balance),
                "references": references,
            }
        )
    return {
        "points": points,
        "first_gap": first_gap,
        "minimum_balance": money(minimum),
        "max_shortfall": money(max(0, -minimum)),
        "closing_balance": money(balance),
        "outside_horizon": outside,
        "probability": None,
        "probability_unavailable_reason": "Детерминированный демо-календарь, ML не выполнялся",
    }


def normalized_batches(entries, manifest, manifest_sha256):
    groups = defaultdict(list)
    for entry in entries:
        groups[entry["source_system"]].append(entry["record"])
    result = []
    for source, records in sorted(groups.items()):
        result.append(
            {
                "schema_version": SCHEMA_VERSION,
                "source": {
                    "kind": "mock_external",
                    "source_system": source,
                    "source_company_id": manifest["source_company_id"],
                    "source_account_id": None,
                    "data_origin": "synthetic_mock",
                    "currency": "RUB",
                    "currency_basis": "source_declared"
                    if source == "demo_1c_ed_1_25"
                    else "adapter_assumption",
                    "retrieved_at": manifest["cutoff_date"] + "T18:00:00Z",
                    "artifact_ref": "manifest.json; synthetic replay time, not a historical source audit",
                    "artifact_sha256": manifest_sha256,
                },
                "record_kinds": ["planned_payments"],
                "factual_period": None,
                "transactions": [],
                "balance_observations": [],
                "planned_payments": records,
            }
        )
    return result


def run(directory):
    """A run is the user's explicit apply action in this isolated demo ledger."""
    directory = Path(directory)
    manifest = manifest_at(directory)
    with (directory / ".run.lock").open("a") as lock:
        try:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError as exc:
            raise ImportFailure("RUN_BUSY", "Другой прогон уже использует этот каталог") from exc
        return _run_locked(directory, manifest)


def _run_locked(directory, manifest):
    binding = {key: manifest[key] for key in BINDING_FIELDS}
    state_path = directory / "state.json"
    state = (
        read_json(state_path, max_bytes=None)
        if state_path.exists()
        else {"schema_version": SCHEMA_VERSION, "binding": binding, "records": {}}
    )
    if (
        not isinstance(state, dict)
        or state.get("binding") != binding
        or state.get("schema_version") != SCHEMA_VERSION
    ):
        raise ImportFailure(
            "BINDING_CONFLICT", "Состояние относится к другой компании, счёту или срезу"
        )
    records = state.get("records")
    if not isinstance(records, dict):
        raise ImportFailure("INVALID_STATE", "Повреждено состояние")
    for key, entry in records.items():
        if not isinstance(entry, dict) or any(
            k not in entry for k in ["record", "source_system", "document_id", "mandatory"]
        ):
            raise ImportFailure("INVALID_STATE", "Повреждена запись состояния")
        validate_record(entry["record"])
        if (
            not isinstance(entry["source_system"], str)
            or not isinstance(entry["record"].get("external_payment_id"), str)
            or type(entry["mandatory"]) is not bool
        ):
            raise ImportFailure("INVALID_STATE", "Повреждены идентификаторы/допущения состояния")
        if key != entry["source_system"] + ":" + entry["record"].get("external_payment_id", ""):
            raise ImportFailure("INVALID_STATE", "Ключ состояния не совпадает с внешним ID")
    before = cash_plan(records.values(), manifest)
    receipts, evidence = [], []
    for meta in manifest["documents"]:
        try:
            path = bound_path(directory, meta["path"])
            if path.stat().st_size > MAX_FILE_BYTES:
                raise ImportFailure("FILE_TOO_LARGE", "XML превышает 2 MiB")
            content = path.read_bytes()
            if digest(content) != meta["sha256"]:
                raise ImportFailure(
                    "DIGEST_MISMATCH", "Содержимое изменилось относительно манифеста"
                )
            source, parsed, details = adapt(content, meta, manifest)
            candidates = {}
            for record in parsed:
                validate_record(record)
                key = source + ":" + record["external_payment_id"]
                value = {
                    "record": record,
                    "source_system": source,
                    "document_id": meta["id"],
                    "mandatory": meta["mandatory"],
                }
                if key in candidates or (key in records and records[key] != value):
                    raise ImportFailure(
                        "CONFLICTING_EXTERNAL_ID",
                        "Изменённое содержание под прежним ID; документ не применён",
                    )
                candidates[key] = value
            existing = sum(key in records for key in candidates)
            # All records in this source document are validated before any are applied.
            records.update(candidates)
            receipts.append(
                {
                    "document_id": meta["id"],
                    "path": meta["path"],
                    "status": "applied",
                    "new_records": len(candidates) - existing,
                    "already_present": existing,
                }
            )
            evidence.extend(details)
        except (ImportFailure, OSError) as exc:
            receipts.append(
                {
                    "document_id": meta["id"],
                    "path": meta["path"],
                    "status": "requires_review",
                    "code": exc.code if isinstance(exc, ImportFailure) else "FILE_UNAVAILABLE",
                    "message": str(exc),
                }
            )
    after = cash_plan(records.values(), manifest)
    report = {
        "demo": True,
        "mode": "calendar_simulation",
        "company_id": manifest["company_id"],
        "account_id": manifest["account_id"],
        "cutoff_date": manifest["cutoff_date"],
        "horizon_days": manifest["horizon_days"],
        "assumptions": manifest.get("assumptions", []),
        "documents": receipts,
        "tax_evidence": evidence,
        "imported_new": sum(r.get("new_records", 0) for r in receipts),
        "already_present": sum(r.get("already_present", 0) for r in receipts),
        "requires_review": sum(r["status"] == "requires_review" for r in receipts),
        "obligations": list(records.values()),
        "before_import": before,
        "after_import": after,
        "closing_balance_delta": money(
            int(after["closing_balance"]["amount_minor"])
            - int(before["closing_balance"]["amount_minor"])
        ),
    }
    write_json(state_path, state)
    write_json(
        directory / "normalized-batches.json",
        normalized_batches(
            records.values(), manifest, digest((directory / "manifest.json").read_bytes())
        ),
    )
    write_json(directory / "report.json", report)
    return report
