"""Real ephemeral mTLS certificates; no user certificate store is modified."""

import importlib.util
import ipaddress
import unittest
from datetime import datetime, timedelta, timezone

from test_rpc import RPC_READY, make_request

TLS_READY = RPC_READY and importlib.util.find_spec("cryptography") is not None

if TLS_READY:
    import grpc
    from cashflow_rpc.server import create_server
    from cryptography import x509
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import rsa
    from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID
    from ml.v1 import forecast_pb2_grpc as rpc


def certificate(name, issuer=None, ca=False, server=False):
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, name or "saldo-core")])
    now = datetime.now(timezone.utc)
    builder = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer[0].subject if issuer else subject)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - timedelta(minutes=1))
        .not_valid_after(now + timedelta(days=1))
        .add_extension(x509.BasicConstraints(ca=ca, path_length=None), critical=True)
    )
    if not ca:
        if name:
            names = [x509.DNSName(name)]
            if server:
                names.append(x509.IPAddress(ipaddress.ip_address("127.0.0.1")))
            builder = builder.add_extension(x509.SubjectAlternativeName(names), critical=False)
        builder = builder.add_extension(
            x509.ExtendedKeyUsage(
                [
                    ExtendedKeyUsageOID.SERVER_AUTH if server else ExtendedKeyUsageOID.CLIENT_AUTH,
                ]
            ),
            critical=False,
        )
    cert = builder.sign(issuer[1] if issuer else key, hashes.SHA256())
    return cert, key


def pem(pair):
    cert, key = pair
    return cert.public_bytes(serialization.Encoding.PEM), key.private_bytes(
        serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()
    )


@unittest.skipUnless(TLS_READY, "Install requirements-rpc-dev.txt and generate bindings")
class MutualTLSTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.ca = certificate("test-root", ca=True)
        cls.root, _ = pem(cls.ca)
        cls.server_cert, cls.server_key = pem(certificate("localhost", cls.ca, server=True))
        cls.core = pem(certificate("saldo-core", cls.ca))
        cls.wrong_peer = pem(certificate("saldo-api", cls.ca))
        cls.no_san = pem(certificate(None, cls.ca))
        other_ca = certificate("untrusted-root", ca=True)
        cls.untrusted = pem(certificate("saldo-core", other_ca))

    def setUp(self):
        self.server, self.port = create_server(
            port=0, certificate=self.server_cert, private_key=self.server_key, client_ca=self.root
        )
        self.server.start()

    def tearDown(self):
        self.server.stop(grace=0).wait(timeout=3)

    def call(self, identity):
        cert, key = identity if identity else (None, None)
        credentials = grpc.ssl_channel_credentials(
            self.root, private_key=key, certificate_chain=cert
        )
        with grpc.secure_channel(f"127.0.0.1:{self.port}", credentials) as channel:
            return rpc.ForecastServiceStub(channel).Predict(make_request(), timeout=2)

    def test_trusted_core_certificate_succeeds(self):
        self.assertEqual(len(self.call(self.core).days), 30)

    def test_financial_rpc_uses_the_same_service_identity_guard(self):
        from ml.v1 import financial_pb2_grpc as frpc
        from test_rpc_financial import financial_request

        for identity, allowed in ((self.core, True), (self.wrong_peer, False)):
            cert, key = identity
            credentials = grpc.ssl_channel_credentials(
                self.root, private_key=key, certificate_chain=cert
            )
            with grpc.secure_channel(f"127.0.0.1:{self.port}", credentials) as channel:
                call = frpc.FinancialForecastServiceStub(channel).PredictFinancial
                if allowed:
                    self.assertEqual(len(call(financial_request(), timeout=2).forecast.days), 30)
                else:
                    with self.assertRaises(grpc.RpcError) as error:
                        call(financial_request(), timeout=2)
                    self.assertEqual(error.exception.code(), grpc.StatusCode.PERMISSION_DENIED)

    def test_trusted_other_service_and_cn_only_are_denied(self):
        for identity in (self.wrong_peer, self.no_san):
            with self.subTest(identity=identity[0][:30]), self.assertRaises(grpc.RpcError) as error:
                self.call(identity)
            self.assertEqual(error.exception.code(), grpc.StatusCode.PERMISSION_DENIED)

    def test_untrusted_or_missing_client_certificate_cannot_connect(self):
        for identity in (self.untrusted, None):
            with self.subTest(identity=identity is None), self.assertRaises(grpc.RpcError) as error:
                self.call(identity)
            self.assertIn(
                error.exception.code(),
                (grpc.StatusCode.UNAVAILABLE, grpc.StatusCode.DEADLINE_EXCEEDED),
            )


if __name__ == "__main__":
    unittest.main()
