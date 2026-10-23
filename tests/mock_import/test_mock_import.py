import copy
import fcntl
import hashlib
import io
import json
import tempfile
import threading
import unittest
import urllib.error
import urllib.request
import xml.etree.ElementTree as ET
from contextlib import redirect_stderr, redirect_stdout
from pathlib import Path

from tools.mock_import.__main__ import main
from tools.mock_import.adapters import parse_xml
from tools.mock_import.common import ED_NS, ImportFailure, fns_day, minor, money, write_json
from tools.mock_import.fixtures import generate
from tools.mock_import.runner import cash_plan, manifest_at, run
from tools.mock_import.server import make_server


class ImportTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name) / "bundle"
        self.manifest = generate(self.directory)

    def doc(self, stem):
        return next(d for d in self.manifest["documents"] if d["path"] == f"documents/{stem}.xml")

    def edit_xml(self, stem, change):
        meta = self.doc(stem)
        path = self.directory / meta["path"]
        root = ET.fromstring(path.read_bytes())
        change(root)
        content = ET.tostring(root, encoding="utf-8", xml_declaration=True)
        path.write_bytes(content)
        meta["sha256"] = hashlib.sha256(content).hexdigest()
        write_json(self.directory / "manifest.json", self.manifest)

    def test_reproducible_generation_and_seed_bound_ids(self):
        other = Path(self.temp.name) / "same"
        generate(other)
        original = {
            str(p.relative_to(self.directory)): p.read_bytes()
            for p in self.directory.rglob("*")
            if p.is_file()
        }
        repeated = {
            str(p.relative_to(other)): p.read_bytes() for p in other.rglob("*") if p.is_file()
        }
        self.assertEqual(original, repeated)
        different = generate(Path(self.temp.name) / "different", seed=43)
        self.assertNotEqual(self.manifest["documents"][0]["id"], different["documents"][0]["id"])

    def test_full_run_imports_five_records_and_reconciles_cash(self):
        result = run(self.directory)
        self.assertEqual(result["imported_new"], 5)
        self.assertEqual(result["requires_review"], 3)
        self.assertEqual(result["before_import"]["closing_balance"], money(20_000_000))
        plan = result["after_import"]
        self.assertEqual(plan["closing_balance"], money(11_000_000))
        self.assertEqual(plan["first_gap"], {"date": "2026-09-15", "shortfall": money(2_000_000)})
        self.assertEqual(plan["max_shortfall"], money(2_000_000))
        self.assertIsNone(plan["probability"])
        for point in plan["points"]:
            amounts = {
                key: int(point[key]["amount_minor"])
                for key in ["opening_balance", "inflow", "outflow", "closing_balance"]
            }
            self.assertEqual(
                amounts["closing_balance"],
                amounts["opening_balance"] + amounts["inflow"] - amounts["outflow"],
            )
        self.assertEqual(len(plan["points"]), 30)
        self.assertEqual(plan["points"][-1]["date"], "2026-09-30")

    def test_partial_execution_and_ens_earmark_are_counted_once(self):
        report = run(self.directory)
        obligations = report["obligations"]
        rent = next(o["record"] for o in obligations if "Аренда" in o["record"]["description"])
        self.assertEqual(rent["amount"], money(8_000_000))
        self.assertEqual(rent["remaining_amount"], money(6_000_000))
        tax = report["tax_evidence"][0]
        self.assertEqual(tax["already_earmarked"], money(4_000_000))
        self.assertEqual(tax["bank_cash_needed"], money(2_000_000))
        codes = {d.get("code") for d in report["documents"] if d["status"] == "requires_review"}
        self.assertEqual(codes, {"NEEDS_DUE_DATE", "TAX_ADJUSTMENT_REQUIRES_REVIEW"})

    def test_repeat_import_preserves_state_and_has_zero_delta(self):
        run(self.directory)
        initial = (self.directory / "state.json").read_bytes()
        report = run(self.directory)
        self.assertEqual(report["imported_new"], 0)
        self.assertEqual(report["already_present"], 5)
        self.assertEqual(report["closing_balance_delta"], money(0))
        self.assertEqual(initial, (self.directory / "state.json").read_bytes())

    def test_changed_economic_content_is_conflict_and_not_overwritten(self):
        run(self.directory)
        self.edit_xml(
            "supplier", lambda r: setattr(r.find(f".//{{{ED_NS}}}Сумма"), "text", "65000.00")
        )
        report = run(self.directory)
        outcome = next(d for d in report["documents"] if d["path"] == "documents/supplier.xml")
        self.assertEqual(outcome["code"], "CONFLICTING_EXTERNAL_ID")
        self.assertEqual(report["after_import"]["closing_balance"], money(11_000_000))

    def test_changed_file_without_updated_manifest_is_rejected(self):
        (self.directory / self.doc("supplier")["path"]).write_bytes(b"<changed/>")
        report = run(self.directory)
        outcome = next(d for d in report["documents"] if d["path"] == "documents/supplier.xml")
        self.assertEqual(outcome["code"], "DIGEST_MISMATCH")
        self.assertEqual(report["imported_new"], 4)

    def test_missing_source_amount_is_reviewed_without_dropping_other_documents(self):
        def remove_amount(root):
            common = root.find(f"{{{ED_NS}}}ОбщиеДанные/{{{ED_NS}}}ОбщиеДанные")
            common.remove(common.find(f"{{{ED_NS}}}Сумма"))

        self.edit_xml("supplier", remove_amount)
        report = run(self.directory)
        outcome = next(d for d in report["documents"] if d["path"] == "documents/supplier.xml")
        self.assertEqual(outcome["code"], "MISSING_FIELD")
        self.assertEqual(report["imported_new"], 4)
        self.assertEqual(report["after_import"]["closing_balance"], money(17_000_000))

    def test_two_runs_cannot_apply_to_same_state_concurrently(self):
        with (self.directory / ".run.lock").open("a") as lock:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
            with self.assertRaises(ImportFailure) as context:
                run(self.directory)
            self.assertEqual(context.exception.code, "RUN_BUSY")
        self.assertFalse((self.directory / "state.json").exists())

    def test_corrupt_state_fails_before_any_write(self):
        run(self.directory)
        path = self.directory / "state.json"
        state = json.loads(path.read_text())
        next(iter(state["records"].values()))["record"] = None
        write_json(path, state)
        original = path.read_bytes()
        with self.assertRaises(ImportFailure) as context:
            run(self.directory)
        self.assertEqual(context.exception.code, "INVALID_STATE")
        self.assertEqual(path.read_bytes(), original)

    def test_multirow_document_rejects_all_rows_on_duplicate_id(self):
        def duplicate(root):
            parent = root.find(f"{{{ED_NS}}}ЭтапыОплаты")
            parent.append(copy.deepcopy(parent[0]))

        self.edit_xml("receipt", duplicate)
        result = run(self.directory)
        self.assertEqual(result["imported_new"], 4)
        self.assertFalse(any(o["record"]["direction"] == "inflow" for o in result["obligations"]))
        self.assertEqual(
            next(d for d in result["documents"] if d["path"] == "documents/receipt.xml")["code"],
            "DUPLICATE_RECORD_ID",
        )

    def test_other_company_and_future_knowledge_are_rejected(self):
        self.edit_xml("rent", lambda r: setattr(r.find(f".//{{{ED_NS}}}ИНН"), "text", "0099999999"))
        self.doc("supplier")["known_as_of_date"] = "2026-09-01"
        write_json(self.directory / "manifest.json", self.manifest)
        result = run(self.directory)
        codes = {d.get("code") for d in result["documents"]}
        self.assertIn("COMPANY_MISMATCH", codes)
        self.assertIn("AFTER_CUTOFF", codes)

    def test_state_cannot_be_rebound_to_other_account(self):
        run(self.directory)
        original = (self.directory / "state.json").read_bytes()
        self.manifest["account_id"] = "other_account"
        write_json(self.directory / "manifest.json", self.manifest)
        with self.assertRaisesRegex(ImportFailure, "другой компании"):
            run(self.directory)
        self.assertEqual(original, (self.directory / "state.json").read_bytes())

    def test_unknown_currency_does_not_become_rubles(self):
        self.edit_xml("receipt", lambda r: setattr(r.find(f".//{{{ED_NS}}}Код"), "text", "840"))
        result = run(self.directory)
        self.assertEqual(
            next(d for d in result["documents"] if d["path"] == "documents/receipt.xml")["code"],
            "UNSUPPORTED_CURRENCY",
        )

    def test_missing_execution_assumption_and_invalid_earmark_are_rejected(self):
        self.doc("rent")["record_assumptions"] = {}
        write_json(self.directory / "manifest.json", self.manifest)
        self.edit_xml("ens", lambda r: r.find(".//ИнфПредНачисл").set("СумЗачтОбяз", "60000.01"))
        result = run(self.directory)
        codes = {d.get("code") for d in result["documents"]}
        self.assertIn("MISSING_EXECUTION_STATE", codes)
        self.assertIn("INVALID_REMAINDER", codes)

    def test_malformed_and_unsafe_xml_are_rejected(self):
        for data in [
            b"<broken>",
            b'<!DOCTYPE x [<!ENTITY a "bad">]><x>&a;</x>',
            "<!DOCTYPE x><x/>".encode("utf-16"),
        ]:
            with self.subTest(data=data), self.assertRaises(ImportFailure):
                parse_xml(data)

    def test_exact_decimal_and_strict_date_validation(self):
        self.assertEqual(minor("12345.67"), 1234567)
        self.assertEqual(minor("-15.20", allow_negative=True), -1520)
        self.assertEqual(fns_day("29.02.2024"), "2024-02-29")
        for value in ["1e3", "NaN", "0.001", "1,00", "-10.00"]:
            with self.subTest(value=value), self.assertRaises(ImportFailure):
                minor(value)
        with self.assertRaises(ImportFailure):
            fns_day("29.02.2026")

    def test_dates_beyond_horizon_preserve_the_obligation(self):
        report = run(self.directory)
        entries = copy.deepcopy(report["obligations"])
        entry = next(e for e in entries if e["record"]["direction"] == "inflow")
        entry["record"]["scheduled_date"] = "2026-10-01"
        plan = cash_plan(entries, self.manifest)
        self.assertEqual(len(plan["outside_horizon"]), 1)
        self.assertEqual(entry["record"]["amount"], money(15_000_000))
        self.assertEqual(plan["closing_balance"], money(-4_000_000))

    def test_full_execution_excludes_future_cash_and_keeps_record(self):
        report = run(self.directory)
        entries = copy.deepcopy(report["obligations"])
        entry = next(e for e in entries if e["record"]["direction"] == "inflow")
        entry["record"].update(remaining_amount=money(0), status="matched")
        self.assertEqual(len(entries), 5)
        self.assertEqual(cash_plan(entries, self.manifest)["closing_balance"], money(-4_000_000))

    def test_normalized_batches_have_required_contract_shape(self):
        run(self.directory)
        batches = json.loads((self.directory / "normalized-batches.json").read_text())
        schema = json.loads(
            Path("docs/data-contracts/normalized-source-batch.schema.json").read_text()
        )
        for batch in batches:
            self.assertEqual(set(batch), set(schema["required"]))
            self.assertEqual(batch["record_kinds"], ["planned_payments"])
            self.assertEqual(
                batch["source"]["artifact_sha256"],
                hashlib.sha256((self.directory / "manifest.json").read_bytes()).hexdigest(),
            )
            for record in batch["planned_payments"]:
                self.assertEqual(
                    set(record), set(schema["$defs"]["NormalizedPlannedPayment"]["required"])
                )
                self.assertEqual(record["availability"]["basis"], "demo_assumption")
                self.assertIsNone(record["source_category"])

    def test_manifest_traversal_does_not_read_external_file(self):
        self.doc("rent")["path"] = "../secret.xml"
        write_json(self.directory / "manifest.json", self.manifest)
        with self.assertRaises(ImportFailure):
            manifest_at(self.directory)

    def test_duplicate_document_metadata_is_rejected(self):
        self.manifest["documents"].append(copy.deepcopy(self.manifest["documents"][0]))
        write_json(self.directory / "manifest.json", self.manifest)
        with self.assertRaises(ImportFailure):
            manifest_at(self.directory)

    def test_existing_files_are_not_overwritten_by_generation(self):
        with self.assertRaises(ImportFailure):
            generate(self.directory)

    def test_cli_replay_and_invalid_arguments(self):
        directory = Path(self.temp.name) / "cli"
        with redirect_stdout(io.StringIO()), redirect_stderr(io.StringIO()):
            args = ["run", "--out", str(directory), "--seed", "3", "--as-of", "2025-03-15"]
            self.assertEqual(main(args), 0)
            self.assertEqual(main(args), 0)
            self.assertEqual(main(["run", "--out", str(directory), "--seed", "4"]), 2)
        report = json.loads((directory / "report.json").read_text())
        self.assertEqual(report["already_present"], 5)
        self.assertEqual(report["cutoff_date"], "2025-03-15")

    def test_future_native_document_is_not_an_input_fact(self):
        self.edit_xml(
            "supplier",
            lambda r: setattr(r.find(f".//{{{ED_NS}}}Дата"), "text", "2026-09-01T09:00:00"),
        )
        report = run(self.directory)
        self.assertEqual(
            next(d for d in report["documents"] if d["path"] == "documents/supplier.xml")["code"],
            "AFTER_CUTOFF",
        )


class HTTPTests(unittest.TestCase):
    def test_http_delivers_exact_documents_and_rejects_other_paths(self):
        with tempfile.TemporaryDirectory() as directory:
            manifest = generate(directory)
            run(directory)
            with make_server(directory) as server:
                thread = threading.Thread(target=server.serve_forever, daemon=True)
                thread.start()
                try:
                    base = f"http://127.0.0.1:{server.server_address[1]}"
                    with urllib.request.urlopen(base + "/manifest.json", timeout=3) as response:
                        self.assertEqual(json.load(response), manifest)
                    for document in manifest["documents"]:
                        with urllib.request.urlopen(
                            base + "/" + document["path"], timeout=3
                        ) as response:
                            self.assertEqual(
                                response.read(), (Path(directory) / document["path"]).read_bytes()
                            )
                    for route in ["/state.json", "/%2e%2e/manifest.json", "/documents/missing.xml"]:
                        with (
                            self.subTest(route=route),
                            self.assertRaises(urllib.error.HTTPError) as context,
                        ):
                            urllib.request.urlopen(base + route, timeout=3)
                        self.assertEqual(context.exception.code, 404)
                        context.exception.close()
                finally:
                    server.shutdown()
                    thread.join(timeout=3)


if __name__ == "__main__":
    unittest.main()
