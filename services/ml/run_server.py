"""Run the ML-only private endpoint; Core integration is a separate stage."""

import argparse
from pathlib import Path

from cashflow_rpc.server import create_server


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=50051)
    parser.add_argument("--insecure-local", action="store_true")
    parser.add_argument("--certificate", type=Path)
    parser.add_argument("--private-key", type=Path)
    parser.add_argument("--client-ca", type=Path)
    parser.add_argument("--allowed-peer", action="append")
    args = parser.parse_args()
    try:
        server, port = create_server(
            args.host,
            args.port,
            insecure_local=args.insecure_local,
            certificate=args.certificate.read_bytes() if args.certificate else None,
            private_key=args.private_key.read_bytes() if args.private_key else None,
            client_ca=args.client_ca.read_bytes() if args.client_ca else None,
            allowed_peers=args.allowed_peer or ("saldo-core",),
        )
    except (ValueError, OSError) as exc:
        parser.error(str(exc))
    server.start()
    print(f"ML gRPC listening on {args.host}:{port}", flush=True)
    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        server.stop(grace=5).wait(timeout=6)


if __name__ == "__main__":
    main()
