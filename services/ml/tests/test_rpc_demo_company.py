"""Independent obligation-conservation check for the demo rehearsal client."""

import contextlib
import io
import json
import tempfile
import unittest
from datetime import date
from pathlib import Path
from types import SimpleNamespace

from test_rpc import RPC_READY
from test_rpc_tls import TLS_READY, certificate, pem

if RPC_READY:
    import verify_demo_company_rpc as demo
    from cashflow_rpc.server import create_server

if TLS_READY:
    from cryptography import x509
    from cryptography.hazmat.primitives import hashes


@unittest.skipUnless(RPC_READY, "Install RPC requirements and generate the current ML proto")
class DemoCompanyRPCTests(unittest.TestCase):
    @unittest.skipUnless(TLS_READY, "Install requirements-rpc-dev.txt")
    def test_demo_client_uses_mtls_with_the_core_spiffe_identity(self):
        ca = certificate("ephemeral-demo-root", ca=True)
        root, _ = pem(ca)
        server_cert, server_key = pem(certificate("localhost", ca, server=True))
        original, core_key = certificate("saldo-core", ca)
        builder = (
            x509.CertificateBuilder()
            .subject_name(original.subject)
            .issuer_name(original.issuer)
            .public_key(core_key.public_key())
            .serial_number(original.serial_number)
            .not_valid_before(original.not_valid_before_utc)
            .not_valid_after(original.not_valid_after_utc)
        )
        for extension in original.extensions:
            value = extension.value
            if isinstance(value, x509.SubjectAlternativeName):
                value = x509.SubjectAlternativeName(
                    [x509.UniformResourceIdentifier("spiffe://saldo.local/core")]
                )
            builder = builder.add_extension(value, critical=extension.critical)
        core_cert, core_private_key = pem((builder.sign(ca[1], hashes.SHA256()), core_key))
        server, port = create_server(
            port=0,
            certificate=server_cert,
            private_key=server_key,
            client_ca=root,
            allowed_peers=("spiffe://saldo.local/core",),
        )
        server.start()
        self.addCleanup(lambda: server.stop(0).wait(timeout=3))
        with tempfile.TemporaryDirectory() as folder:
            directory = Path(folder)
            for name, data in (
                ("ca.pem", root),
                ("core.pem", core_cert),
                ("core.key", core_private_key),
            ):
                (directory / name).write_bytes(data)
            args = SimpleNamespace(
                output_dir=directory / "result",
                cutoff=date(2026, 9, 30),
                main_sha="synthetic-mtls-test",
                insecure_local=False,
                ca=directory / "ca.pem",
                certificate=directory / "core.pem",
                private_key=directory / "core.key",
                server_name="localhost",
            )
            with contextlib.redirect_stdout(io.StringIO()):
                demo.run(f"127.0.0.1:{port}", args)
            summary = json.loads((args.output_dir / "summary.json").read_text(encoding="utf-8"))
            self.assertTrue(summary["passed"])
            self.assertEqual(
                summary["steps"]["C_partial_paid_reconciled"]["closing_minor"], -5_000_000
            )

    def test_partial_settlement_preserves_total_obligation(self):
        server, port = create_server(port=0, insecure_local=True)
        server.start()
        self.addCleanup(lambda: server.stop(0).wait(timeout=3))
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / "result"
            args = SimpleNamespace(
                output_dir=output,
                cutoff=date(2026, 9, 30),
                main_sha="synthetic-test",
                insecure_local=True,
            )
            with contextlib.redirect_stdout(io.StringIO()):
                demo.run(f"127.0.0.1:{port}", args)
            summary = json.loads((output / "summary.json").read_text(encoding="utf-8"))
            self.assertTrue(summary["passed"])
            steps = summary["steps"]
            self.assertEqual(steps["A_core_shape_no_payment"]["closing_minor"], -4_000_000)
            self.assertEqual(steps["B_manual_payment_added"]["closing_minor"], -5_000_000)
            partial = steps["C_partial_paid_reconciled"]
            self.assertEqual(partial["closing_minor"], -5_000_000)
            self.assertEqual(partial["first_negative"], "2026-10-14")

            request = json.loads(
                (output / "C_partial_paid_reconciled-request.json").read_text(encoding="utf-8")
            )
            remaining = sum(
                int(p.get("remaining_outflow_minor", 0)) for p in request["calendar"]["payments"]
            )
            # 10,000 RUB obligation = 3,000 paid + 7,000 remaining.
            self.assertEqual(remaining, 700_000)
            self.assertEqual(
                5_000_000 - int(request["snapshot"]["initial_balance_minor"]) + remaining, 1_000_000
            )

            before = json.loads(
                (output / "B_manual_payment_added-response.json").read_text(encoding="utf-8")
            )["days"]
            after = json.loads(
                (output / "C_partial_paid_reconciled-response.json").read_text(encoding="utf-8")
            )["days"]
            for day, (original, settled) in enumerate(zip(before, after), 1):
                difference = int(settled["reference_balance_minor"]) - int(
                    original["reference_balance_minor"]
                )
                self.assertEqual(difference, -300_000 if day < 5 else 0)
