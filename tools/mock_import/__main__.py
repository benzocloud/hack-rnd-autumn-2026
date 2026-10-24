"""python3 -m tools.mock_import generate|run|serve."""

import argparse
import sys
from contextlib import suppress
from pathlib import Path

from .common import ImportFailure, major
from .fixtures import generate
from .runner import manifest_at, run
from .server import make_server


def main(argv=None):
    parser = argparse.ArgumentParser(description="Локальная имитация импорта документов 1С/ФНС")
    commands = parser.add_subparsers(dest="command", required=True)
    for name in ["generate", "run"]:
        command = commands.add_parser(name)
        command.add_argument(
            "--out", type=Path, required=True, help="Каталог демо-набора и результатов"
        )
        command.add_argument("--seed", type=int)
        command.add_argument("--as-of", help="Дата среза YYYY-MM-DD; по умолчанию 2026-08-31")
    serve = commands.add_parser("serve")
    serve.add_argument("--bundle", type=Path, required=True)
    serve.add_argument("--port", type=int, default=8765)
    args = parser.parse_args(argv)
    try:
        if args.command == "serve":
            if not 0 <= args.port <= 65535:
                raise ImportFailure("INVALID_PORT", "Порт должен быть от 0 до 65535")
            with make_server(args.bundle, args.port) as server:
                print(
                    f"Демо-мок: http://127.0.0.1:{server.server_address[1]}/manifest.json",
                    flush=True,
                )
                with suppress(KeyboardInterrupt):
                    server.serve_forever()
            return 0
        if args.command == "generate":
            manifest = generate(
                args.out, args.seed if args.seed is not None else 42, args.as_of or "2026-08-31"
            )
            print(
                f"Сгенерировано демо-документов: {len(manifest['documents'])}. {args.out / 'manifest.json'}"
            )
            return 0
        if not (args.out / "manifest.json").exists():
            generate(
                args.out, args.seed if args.seed is not None else 42, args.as_of or "2026-08-31"
            )
        else:
            manifest = manifest_at(args.out)
            if (args.seed is not None and args.seed != manifest["seed"]) or (
                args.as_of is not None and args.as_of != manifest["cutoff_date"]
            ):
                raise ImportFailure(
                    "EXISTING_BUNDLE", "Новый seed/срез требуют отдельного каталога"
                )
        report = run(args.out)
        print("ДЕМО: календарный расчёт, без ML и подключения к настоящим 1С/ФНС")
        print(
            f"Новых обязательств/поступлений: {report['imported_new']}; уже присутствуют: {report['already_present']}"
        )
        print(f"Документов требуют уточнения: {report['requires_review']}")
        result = report["after_import"]
        print(
            f"Расчётный остаток в конце горизонта: {major(int(result['closing_balance']['amount_minor']))} RUB"
        )
        gap = result["first_gap"]
        if gap:
            print(
                f"Первый отрицательный остаток: {gap['date']}, нехватка {major(int(gap['shortfall']['amount_minor']))} RUB"
            )
        else:
            print("Отрицательный дневной остаток не выявлен в выбранном горизонте и условиях")
        print(f"Результаты: {args.out / 'report.json'}")
        return 0
    except (ImportFailure, OSError) as exc:
        code = exc.code if isinstance(exc, ImportFailure) else "IO_ERROR"
        print(f"{code}: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
