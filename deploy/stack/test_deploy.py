"""Execute the real rollout script against isolated paths and failing tools."""

import os
import subprocess
import tempfile
import unittest
from pathlib import Path


class DeploymentFailureTest(unittest.TestCase):
    def test_failed_migration_rolls_back_without_marking_release(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            stack, front, tools = (root / name for name in ("stack", "front", "bin"))
            for folder in (stack, front / "nginx", tools, root / "lock"):
                folder.mkdir(parents=True, exist_ok=True)
            (stack / ".env").write_text("OLD_BACKEND=preserved\n")
            (front / ".env").write_text("FRONTEND_IMAGE=old\n")
            (front / "nginx/https.conf").write_text("old nginx\n")
            log = root / "calls"
            for name, body in {
                "docker": 'printf "%s\\n" "$*" >> "$CALLS"\ncase "$*" in *"/app/migrate"*) exit 42;; esac\nexit 0',
                "curl": 'echo unexpected-curl >> "$CALLS"; exit 0',
                "flock": "exit 0",
            }.items():
                file = tools / name
                file.write_text("#!/bin/sh\n" + body + "\n")
                file.chmod(0o755)
            source = Path(__file__).with_name("deploy.sh").read_text()
            source = source.replace("$EUID != 0", "0 != 0")
            source = (
                source.replace("/opt/saldo-stack", str(stack))
                .replace("/opt/saldo-frontend", str(front))
                .replace("/run/lock", str(root / "lock"))
            )
            script = root / "deploy.sh"
            script.write_text(source)
            digest = "sha256:" + "a" * 64
            result = subprocess.run(
                ["bash", str(script), "deploy-stack", "b" * 40, *([digest] * 5), "actor"],
                input="temporary-fixture-token\n",
                text=True,
                capture_output=True,
                env={
                    **os.environ,
                    "PATH": str(tools) + ":" + os.environ["PATH"],
                    "CALLS": str(log),
                },
            )
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse((stack / "deployed-revision").exists())
            self.assertEqual((stack / ".env").read_text(), "OLD_BACKEND=preserved\n")
            self.assertEqual((front / ".env").read_text(), "FRONTEND_IMAGE=old\n")
            calls = log.read_text()
            self.assertIn("/app/migrate", calls)
            self.assertNotIn("unexpected-curl", calls)
            self.assertNotIn("/app/goose", calls)


if __name__ == "__main__":
    unittest.main()
