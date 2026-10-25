"""Bounded private gRPC endpoint with mTLS or explicit local smoke mode."""

from concurrent.futures import ThreadPoolExecutor
from ipaddress import IPv4Address

import grpc
from ml.v1 import forecast_pb2 as pb
from ml.v1 import forecast_pb2_grpc as rpc

from .csv_forecast import CSV_VERSION, csv_forecast
from .financial import FINANCIAL_VERSION, financial_forecast
from .inference import InvalidSnapshot, UnsupportedContext, forecast


class ForecastService(rpc.ForecastServiceServicer):
    def __init__(self, allowed_peers, insecure_local=False):
        self.allowed_peers = frozenset(allowed_peers)
        self.insecure_local = insecure_local

    def Predict(self, request, context):
        if request.model.model_version == CSV_VERSION:
            return self._compute(request, context, lambda r: csv_forecast(r, pb))
        compute = (
            (lambda r: financial_forecast(r, pb))
            if request.model.model_version == FINANCIAL_VERSION
            else (lambda r: forecast(r, pb))
        )
        return self._compute(request, context, compute)

    def _compute(self, request, context, compute):
        if not self.insecure_local:
            auth = context.auth_context()
            names = auth.get("x509_subject_alternative_name", ())
            if auth.get("transport_security_type") != [b"ssl"] or not any(
                name in {peer.encode("utf-8") for peer in self.allowed_peers} for name in names
            ):
                context.abort(grpc.StatusCode.PERMISSION_DENIED, "Core service identity required")
        remaining = context.time_remaining()
        if remaining is None or remaining > 30:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "Set a client deadline of at most 30s")
        if remaining <= 0:
            context.abort(grpc.StatusCode.DEADLINE_EXCEEDED, "Request deadline expired")
        if not context.is_active():
            context.abort(grpc.StatusCode.CANCELLED, "Request cancelled")
        try:
            result = compute(request)
        except InvalidSnapshot as exc:
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, str(exc))
        except UnsupportedContext as exc:
            context.abort(grpc.StatusCode.FAILED_PRECONDITION, str(exc))
        if not context.is_active():
            context.abort(grpc.StatusCode.CANCELLED, "Request cancelled")
        return result


def create_server(
    host="127.0.0.1",
    port=50051,
    *,
    insecure_local=False,
    certificate=None,
    private_key=None,
    client_ca=None,
    allowed_peers=("saldo-core",),
):
    address = IPv4Address(host)
    if not 0 <= port <= 65535:
        raise ValueError("Port outside 0..65535")
    if insecure_local:
        if str(address) != "127.0.0.1" or any((certificate, private_key, client_ca)):
            raise ValueError("Insecure mode requires 127.0.0.1 and no TLS material")
    elif not all((certificate, private_key, client_ca, allowed_peers)):
        raise ValueError("mTLS certificate, key, client CA and peer allowlist are required")
    server = grpc.server(
        ThreadPoolExecutor(max_workers=2),
        maximum_concurrent_rpcs=4,
        options=(
            ("grpc.max_receive_message_length", 2 * 1024 * 1024),
            ("grpc.max_send_message_length", 2 * 1024 * 1024),
        ),
    )
    rpc.add_ForecastServiceServicer_to_server(
        ForecastService(allowed_peers, insecure_local), server
    )
    target = f"{address}:{port}"
    if insecure_local:
        bound = server.add_insecure_port(target)
    else:
        credentials = grpc.ssl_server_credentials(
            ((private_key, certificate),), root_certificates=client_ca, require_client_auth=True
        )
        bound = server.add_secure_port(target, credentials)
    if not bound:
        raise RuntimeError("Cannot bind ML endpoint")
    return server, bound
