"""Generate Python bindings locally from the canonical ML proto."""

import argparse
import os
import subprocess
import sys
import tempfile
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--buf", default="buf", help="Buf v1.75.0 executable")
    args = parser.parse_args()
    service = Path(__file__).resolve().parent
    root = service.parents[1]
    environment = dict(os.environ)
    environment.setdefault("BUF_CACHE_DIR", str(service / "artifacts/buf-cache"))
    buf = str(Path(args.buf).resolve()) if Path(args.buf).exists() else args.buf
    version = subprocess.check_output([buf, "--version"], text=True, env=environment).strip()
    if version != "1.75.0":
        parser.error("Generation requires pinned Buf 1.75.0")
    with tempfile.TemporaryDirectory() as temporary:
        descriptor = str(Path(temporary) / "ml.binpb")
        subprocess.run(
            [
                buf,
                "build",
                "--as-file-descriptor-set",
                "--path",
                "proto/ml/v1/forecast.proto",
                "--output",
                descriptor,
            ],
            cwd=root,
            env=environment,
            check=True,
        )
        subprocess.run(
            [
                sys.executable,
                "-m",
                "grpc_tools.protoc",
                "--descriptor_set_in=" + descriptor,
                "--python_out=" + str(service),
                "--grpc_python_out=" + str(service),
                "ml/v1/forecast.proto",
            ],
            check=True,
        )


if __name__ == "__main__":
    main()
