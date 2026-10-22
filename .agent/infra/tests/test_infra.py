import importlib.util
import io
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch


def module(name):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).parent.parent / {"bootstrap": "bootstrap/bootstrap.py", "backup": "gbrain/backup.py", "configure": "gbrain/configure.py"}[name])
    loaded = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(loaded)
    return loaded


class InfraSafetyTests(unittest.TestCase):
    def test_bad_checksum_never_installs_binary(self):
        bootstrap = module("bootstrap")
        with tempfile.TemporaryDirectory() as temporary:
            bootstrap.ROOT = Path(temporary)
            (bootstrap.ROOT / "bin").mkdir()
            with patch.object(bootstrap.urllib.request, "urlopen", return_value=io.BytesIO(b"untrusted")):
                with self.assertRaisesRegex(RuntimeError, "Checksum mismatch"):
                    bootstrap.install_binary("example", {"version": "1", "url": "https://example.test/bin", "sha256": "0" * 64})
            self.assertFalse((bootstrap.ROOT / "bin/example").exists())
            self.assertFalse((bootstrap.ROOT / "releases/example-1/example").exists())

    def test_failed_backup_still_restarts_only_our_service(self):
        backup = module("backup")
        with tempfile.TemporaryFile() as lock, patch("builtins.open", return_value=lock):
            with patch.object(backup.subprocess, "run", side_effect=[None, subprocess.CalledProcessError(1, ["gbrain"]), None]) as run:
                with self.assertRaises(subprocess.CalledProcessError):
                    backup.main()
        self.assertEqual(run.call_args_list[0].args[0], ["systemctl", "stop", "hackrnd-gbrain.service"])
        self.assertEqual(run.call_args_list[-1].args[0], ["systemctl", "start", "hackrnd-gbrain.service"])

    def test_credentials_are_private_and_never_overwritten(self):
        configure = module("configure")
        with tempfile.TemporaryDirectory() as temporary:
            target = Path(temporary) / "token"
            configure.private_file(target, "first")
            self.assertEqual(target.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(FileExistsError):
                configure.private_file(target, "second")
            self.assertEqual(target.read_text(), "first")


if __name__ == "__main__":
    unittest.main()
