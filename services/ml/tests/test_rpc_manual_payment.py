"""Canonical manual-payment lifecycle over gRPC, without the participant dataset."""

import contextlib
import io
import json
import tempfile
import unittest
from datetime import timedelta
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

from test_rpc import RPC_READY

if RPC_READY:
    import verify_manual_payment_rpc as demo
    from cashflow_baseline.data import Company, Operation
    from cashflow_rpc.server import create_server


@unittest.skipUnless(RPC_READY, "Install RPC requirements and generate the current ML proto")
class ManualPaymentRPCTests(unittest.TestCase):
    def test_lifecycle_preserves_paid_cash_and_changes_gap_date(self):
        operations, balance = [], 100_000
        for step in range(90):
            day = demo.CUTOFF - timedelta(days=89 - step)
            balance += 100
            for incoming, amount, name in ((True, 300, "in"), (False, 200, "out")):
                operations.append(
                    Operation(
                        "C002",
                        f"{step}-{name}",
                        day,
                        amount,
                        incoming,
                        "0012345678",
                        "Synthetic",
                        "test fact",
                        balance,
                    )
                )
        company = Company("C002", "synthetic", operations)
        server, port = create_server(port=0, insecure_local=True)
        server.start()
        self.addCleanup(lambda: server.stop(0).wait())
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / "result"
            args = SimpleNamespace(
                output_dir=output,
                data_dir=Path(folder) / "synthetic",
                main_sha="test-fixture",
                start_local_server=False,
                insecure_local=True,
            )
            with (
                patch.object(demo, "load_companies", return_value=[company]),
                patch.object(demo, "sha256", return_value="synthetic-no-source-file"),
                contextlib.redirect_stdout(io.StringIO()),
            ):
                demo.run(f"127.0.0.1:{port}", args)
            result = json.loads((output / "summary.json").read_text(encoding="utf-8"))
            steps = {item["step"]: item for item in result["steps"]}
            self.assertEqual(result["transport"], "external_insecure_loopback")
            self.assertEqual(result["rpc"], "ml.v1.ForecastService/Predict")
            self.assertEqual(steps["1_add_day10"]["first_negative"], "2026-10-10")
            self.assertEqual(steps["3_move_day20"]["first_negative"], "2026-10-20")
            self.assertEqual(
                steps["3_move_day20"]["closing_minor"],
                steps["4_partial_reconciled"]["closing_minor"],
            )
            self.assertEqual(
                steps["5_cancel_remaining"]["closing_minor"],
                steps["0_baseline"]["closing_minor"] - result["paid_minor"],
            )
            self.assertIsNone(steps["5_cancel_remaining"]["first_negative"])
            self.assertEqual(result["naive_partial_extra_outflow_minor"], result["paid_minor"] // 3)
            for name in ("duplicate_internal_id", "next_day_without_attestation"):
                self.assertEqual(steps[name]["status"], "INVALID_ARGUMENT")
            self.assertFalse(result["core_worker_storage_api_e2e_verified"])
            self.assertFalse(result["core_import_idempotence_verified"])
            self.assertIsNone(result["gap_probability"])
            self.assertEqual(len(list(output.glob("*-request.json"))), 6)
