"""Read-only loopback transport of our demo files, not a vendor API."""

import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import unquote, urlsplit

from .common import MAX_FILE_BYTES, ImportFailure, bound_path, digest
from .fixtures import PROFILES
from .runner import manifest_at


def make_server(directory, port=0):
    directory = Path(directory).resolve()
    manifest_at(directory)

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_args):
            pass

        def send_content(self, content, content_type, status=200):
            self.send_response(status)
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(len(content)))
            self.send_header("X-Content-Type-Options", "nosniff")
            self.end_headers()
            self.wfile.write(content)

        def do_GET(self):
            route = unquote(urlsplit(self.path).path)
            if route in {"/", "/health"}:
                body = {
                    "demo": True,
                    "vendor_api": False,
                    "profiles": PROFILES,
                    "manifest": "/manifest.json",
                    "report": "/report.json",
                }
                self.send_content(
                    json.dumps(body, ensure_ascii=False).encode(), "application/json; charset=utf-8"
                )
                return
            try:
                manifest = manifest_at(directory)
                documents = {"/" + d["path"]: d for d in manifest["documents"]}
                allowed = {"/manifest.json", "/report.json", "/normalized-batches.json"}
                if route not in allowed and route not in documents:
                    self.send_content(b'{"error":"NOT_FOUND"}', "application/json", 404)
                    return
                path = bound_path(directory, route[1:])
                # The input cap applies to sources, not accumulated local reports.
                if (
                    route in documents or route == "/manifest.json"
                ) and path.stat().st_size > MAX_FILE_BYTES:
                    raise ImportFailure("FILE_TOO_LARGE", "File too large")
                content = path.read_bytes()
                if route in documents and digest(content) != documents[route]["sha256"]:
                    raise ImportFailure("DIGEST_MISMATCH", "Document differs from manifest")
                content_type = (
                    "application/xml" if route in documents else "application/json; charset=utf-8"
                )
                self.send_content(content, content_type)
            except ImportFailure as exc:
                self.send_content(json.dumps({"error": exc.code}).encode(), "application/json", 409)
            except OSError:
                self.send_content(b'{"error":"FILE_UNAVAILABLE"}', "application/json", 404)

    return ThreadingHTTPServer(("127.0.0.1", port), Handler)
