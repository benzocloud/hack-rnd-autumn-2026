"""Shared primitives of the local mock harness."""

import datetime as dt
import hashlib
import json
import re
import tempfile
from decimal import Decimal, InvalidOperation
from pathlib import Path

ED_NS = "http://v8.1c.ru/edi/edi_stnd/EnterpriseData/1.25"
SCHEMA_VERSION = "0.1.0"
MAX_FILE_BYTES = 2 * 1024 * 1024


class ImportFailure(ValueError):
    def __init__(self, code, message):
        self.code = code
        super().__init__(message)


def day(value):
    if not isinstance(value, str) or not re.fullmatch(r"\d{4}-\d{2}-\d{2}", value):
        raise ImportFailure("INVALID_DATE", "Ожидается календарная дата YYYY-MM-DD")
    try:
        return dt.date.fromisoformat(value)
    except ValueError as exc:
        raise ImportFailure("INVALID_DATE", "Несуществующая календарная дата") from exc


def fns_day(value):
    if not isinstance(value, str) or not re.fullmatch(r"\d{2}\.\d{2}\.\d{4}", value):
        raise ImportFailure("INVALID_DATE", "Дата ФНС должна иметь формат ДД.ММ.ГГГГ")
    return day(f"{value[6:]}-{value[3:5]}-{value[:2]}").isoformat()


def minor(value, allow_negative=False):
    if not isinstance(value, str) or not re.fullmatch(r"-?\d+(?:\.\d{1,2})?", value):
        raise ImportFailure("INVALID_AMOUNT", "Сумма должна быть десятичной строкой без экспоненты")
    try:
        result = Decimal(value) * 100
    except InvalidOperation as exc:
        raise ImportFailure("INVALID_AMOUNT", "Некорректная сумма") from exc
    if abs(result) > 10**17 - 1 or (result < 0 and not allow_negative):
        raise ImportFailure("INVALID_AMOUNT", "Сумма вне поддерживаемого диапазона")
    return int(result)


def integer_minor(value):
    if not isinstance(value, str) or not re.fullmatch(r"(?:0|[1-9]\d*)", value):
        raise ImportFailure("INVALID_AMOUNT", "Ожидается неотрицательная строка минимальных единиц")
    if len(value) > 17:
        raise ImportFailure("INVALID_AMOUNT", "Слишком большая сумма")
    return int(value)


def money(value):
    return {"amount_minor": str(value), "currency": "RUB"}


def major(value):
    sign = "-" if value < 0 else ""
    value = abs(value)
    return f"{sign}{value // 100}.{value % 100:02d}"


def digest(data):
    return hashlib.sha256(data).hexdigest()


def canonical(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":"))


def write_json(path, value):
    """Atomic replacement; a failed write leaves the previous mock state intact."""
    path = Path(path)
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(
        mode="w", encoding="utf-8", dir=path.parent, delete=False
    ) as f:
        temporary = Path(f.name)
        try:
            json.dump(value, f, ensure_ascii=False, indent=2)
            f.write("\n")
            f.close()
            temporary.replace(path)
        finally:
            temporary.unlink(missing_ok=True)


def read_json(path):
    try:
        if Path(path).stat().st_size > MAX_FILE_BYTES:
            raise ImportFailure("FILE_TOO_LARGE", "Слишком большой файл")
        return json.loads(Path(path).read_text(encoding="utf-8"))
    except (OSError, ValueError) as exc:
        if isinstance(exc, ImportFailure):
            raise
        raise ImportFailure("INVALID_JSON", "Не удалось прочитать JSON") from exc


def bound_path(root, relative):
    root = Path(root).resolve()
    if not isinstance(relative, str) or Path(relative).is_absolute():
        raise ImportFailure("INVALID_PATH", "Путь должен быть относительным")
    candidate = (root / relative).resolve()
    if candidate == root or root not in candidate.parents:
        raise ImportFailure("INVALID_PATH", "Путь находится за пределами набора")
    return candidate
