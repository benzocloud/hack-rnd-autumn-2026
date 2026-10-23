import importlib.util
import io
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


def module(name):
    spec = importlib.util.spec_from_file_location(
        name,
        Path(__file__).parent.parent
        / {
            "bootstrap": "bootstrap/bootstrap.py",
            "backup": "gbrain/backup.py",
            "deploy": "gbrain/deploy.py",
            "headers": "gbrain/headers.py",
        }[name],
    )
    loaded = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(loaded)
    return loaded


class InfraSafetyTests(unittest.TestCase):
    def test_bad_checksum_never_installs_binary(self):
        bootstrap = module("bootstrap")
        with tempfile.TemporaryDirectory() as temporary:
            bootstrap.ROOT = Path(temporary)
            (bootstrap.ROOT / "bin").mkdir()
            with (
                patch.object(
                    bootstrap.urllib.request, "urlopen", return_value=io.BytesIO(b"untrusted")
                ),
                self.assertRaisesRegex(RuntimeError, "Checksum mismatch"),
            ):
                bootstrap.install_binary(
                    "example",
                    {"version": "1", "url": "https://example.test/bin", "sha256": "0" * 64},
                )
            self.assertFalse((bootstrap.ROOT / "bin/example").exists())
            self.assertFalse((bootstrap.ROOT / "releases/example-1/example").exists())

    def test_failed_backup_still_restarts_only_our_service(self):
        backup = module("backup")
        with (
            tempfile.TemporaryFile() as lock,
            patch("builtins.open", return_value=lock),
            patch.object(
                backup.subprocess,
                "run",
                side_effect=[
                    subprocess.CompletedProcess([], 0),
                    None,
                    subprocess.CompletedProcess([], 1),
                    None,
                ],
            ) as run,
            self.assertRaisesRegex(SystemExit, "backup failed"),
        ):
            backup.main()
        self.assertEqual(
            run.call_args_list[1].args[0], ["systemctl", "stop", "hackrnd-gbrain.service"]
        )
        self.assertEqual(
            run.call_args_list[-1].args[0], ["systemctl", "start", "hackrnd-gbrain.service"]
        )

    def test_credentials_are_private_and_never_overwritten(self):
        deploy = module("deploy")
        with tempfile.TemporaryDirectory() as temporary:
            target = Path(temporary) / "token"
            deploy.private_file(target, "first")
            self.assertEqual(target.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(FileExistsError):
                deploy.private_file(target, "second")
            self.assertEqual(target.read_text(), "first")

    def test_header_helper_refuses_group_readable_credential(self):
        headers = module("headers")
        with tempfile.TemporaryDirectory() as temporary:
            token = Path(temporary) / ".agent/local/gbrain-developer.token"
            token.parent.mkdir(parents=True)
            token.write_text("gbrain_" + "fixture")
            token.chmod(0o640)
            with (
                patch.object(headers, "ROOT", Path(temporary)),
                self.assertRaisesRegex(SystemExit, "must be private"),
            ):
                headers.main()

    def test_backup_does_not_start_a_previously_stopped_brain(self):
        backup = module("backup")
        with (
            tempfile.TemporaryFile() as lock,
            patch("builtins.open", return_value=lock),
            patch.object(
                backup.subprocess, "run", return_value=subprocess.CompletedProcess([], 3)
            ) as run,
            self.assertRaisesRegex(SystemExit, "was not active"),
        ):
            backup.main()
        self.assertEqual(run.call_count, 1)


if __name__ == "__main__":
    unittest.main()
