#!/usr/bin/env python3
"""Generate/check API-local bindings from canonical Auth/Core protobuf, without touching other modules."""

import argparse
import re
import subprocess
import tempfile
from pathlib import Path


def normalize(text):
    return re.sub(r"^//\s*(?:-\s*)?protoc\s+v[^\n]+\n", "", text, flags=re.MULTILINE)


def generate(check):
    root = Path(__file__).resolve().parents[3]
    target = root / "services/api/internal/gen"
    sources = [
        Path("proto/auth/v1/auth.proto"),
        *sorted(Path(root / "proto/core/v1").glob("*.proto")),
    ]
    sources = [p.relative_to(root) if p.is_absolute() else p for p in sources]
    mappings = [
        f"M{p.relative_to('proto')}=github.com/benzocloud/hack-rnd-autumn-2026/services/api/internal/gen/{p.relative_to('proto').parent}"
        for p in sources
    ]
    with tempfile.TemporaryDirectory(prefix="saldo-api-proto-") as temporary:
        subprocess.run(
            [
                "protoc",
                "-I",
                "proto",
                f"--go_out={temporary}",
                "--go_opt=paths=source_relative",
                *[f"--go_opt={mapping}" for mapping in mappings],
                f"--go-grpc_out={temporary}",
                "--go-grpc_opt=paths=source_relative",
                *[f"--go-grpc_opt={mapping}" for mapping in mappings],
                *map(str, sources),
            ],
            cwd=root,
            check=True,
        )
        for generated in Path(temporary).rglob("*.go"):
            destination = target / generated.relative_to(temporary)
            if check:
                # protoc versions add only a version comment with these plugins;
                # compare all other bytes, including plugin versions/descriptors.
                assert normalize(destination.read_text()) == normalize(generated.read_text()), (
                    f"stale binding: {destination}"
                )
            else:
                destination.parent.mkdir(parents=True, exist_ok=True)
                destination.write_bytes(generated.read_bytes())
    print(
        "Auth/Core bindings match canonical protobuf"
        if check
        else "API Auth/Core bindings generated"
    )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--check", action="store_true")
    generate(parser.parse_args().check)
