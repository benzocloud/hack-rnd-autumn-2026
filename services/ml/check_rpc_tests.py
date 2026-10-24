"""Require the RPC transport/security suite to execute rather than skip in CI."""

import sys
import unittest
from pathlib import Path


def main():
    suite = unittest.defaultTestLoader.discover(
        str(Path(__file__).parent / "tests"), pattern="test_rpc*.py"
    )
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    if not result.wasSuccessful() or result.skipped or result.testsRun == 0:
        sys.exit("RPC checks must pass with zero skips")


if __name__ == "__main__":
    main()
