"""Additive v2 wire contract; current runtime deliberately remains v1-only."""

import unittest

from test_rpc import RPC_READY, make_request
from test_rpc_financial import financial_request

if RPC_READY:
    import grpc
    from cashflow_rpc.financial import financial_forecast
    from cashflow_rpc.server import create_server
    from google.protobuf import (
        descriptor_pb2,
        descriptor_pool,
        empty_pb2,
        message_factory,
        timestamp_pb2,
    )
    from ml.v1 import forecast_pb2 as pb
    from ml.v1 import forecast_pb2_grpc as rpc


def legacy_pool():
    """Build the prior wire shape by removing only this additive change."""
    source = descriptor_pb2.FileDescriptorProto.FromString(pb.DESCRIPTOR.serialized_pb)
    limits = {"PredictResponse": 22, "FinancialOperation": 16, "FinancialCalendar": 7}
    legacy_messages = {
        "SyntheticScenarioContext",
        "AnalysisContext",
        "OperationVersionReference",
        "RecurringHypothesis",
    }
    for index in range(len(source.message_type) - 1, -1, -1):
        message = source.message_type[index]
        if message.name in legacy_messages:
            del source.message_type[index]
            continue
        for field in range(len(message.field) - 1, -1, -1):
            if message.field[field].number > limits.get(message.name, 1000):
                del message.field[field]
    for index in range(len(source.enum_type) - 1, -1, -1):
        enum = source.enum_type[index]
        if enum.name.startswith("Recurring"):
            del source.enum_type[index]
        elif enum.name == "CalendarKnowledgeMode":
            del enum.value[-1]
    pool = descriptor_pool.DescriptorPool()
    pool.AddSerializedFile(empty_pb2.DESCRIPTOR.serialized_pb)
    pool.AddSerializedFile(timestamp_pb2.DESCRIPTOR.serialized_pb)
    pool.Add(source)
    return pool


@unittest.skipUnless(RPC_READY, "Install RPC requirements and generate the current ML proto")
class FinancialContractV2Tests(unittest.TestCase):
    def test_additive_numbers_and_typed_presence(self):
        self.assertEqual(pb.CALENDAR_KNOWLEDGE_MODE_SYNTHETIC_SCENARIO, 4)
        expected = {
            pb.FinancialCalendar: {"scenario_context": 8},
            pb.FinancialOperation: {"category_rule_id": 17},
            pb.PredictResponse: {
                "analysis_context": 23,
                "recurring_hypotheses": 24,
                "scenario_context": 25,
            },
        }
        for message, fields in expected.items():
            for name, number in fields.items():
                self.assertEqual(message.DESCRIPTOR.fields_by_name[name].number, number)
        context = pb.SyntheticScenarioContext()
        self.assertFalse(context.HasField("recorded_at"))
        context.recorded_at.FromJsonString("2026-10-24T20:00:00Z")
        self.assertTrue(context.HasField("recorded_at"))
        hypothesis = pb.RecurringHypothesis(day_of_month=15)
        self.assertEqual(hypothesis.WhichOneof("monthly_anchor"), "day_of_month")
        hypothesis.month_end.SetInParent()
        self.assertEqual(hypothesis.WhichOneof("monthly_anchor"), "month_end")
        self.assertEqual(hypothesis.day_of_month, 0)

    def test_legacy_payload_has_no_synthetic_or_analysis_presence(self):
        pool = legacy_pool()
        for name, current in (
            ("PredictRequest", pb.PredictRequest),
            ("PredictResponse", pb.PredictResponse),
        ):
            old_type = message_factory.GetMessageClass(pool.FindMessageTypeByName("ml.v1." + name))
            request = financial_request()
            original = request if name == "PredictRequest" else financial_forecast(request, pb)
            old = old_type.FromString(original.SerializeToString(deterministic=True))
            restored = current.FromString(old.SerializeToString(deterministic=True))
            self.assertEqual(restored, original)
            if name == "PredictRequest":
                self.assertFalse(restored.calendar.HasField("scenario_context"))
            else:
                self.assertFalse(restored.HasField("scenario_context"))
                self.assertFalse(restored.HasField("analysis_context"))
                self.assertEqual(len(restored.recurring_hypotheses), 0)

    def test_unimplemented_version_and_modes_fail_without_baseline_success(self):
        server, port = create_server(port=0, insecure_local=True)
        server.start()
        self.addCleanup(lambda: server.stop(0).wait(timeout=3))
        with grpc.insecure_channel(f"127.0.0.1:{port}") as channel:
            stub = rpc.ForecastServiceStub(channel)
            for mode in (pb.CALENDAR_KNOWLEDGE_MODE_SYNTHETIC_SCENARIO, 999):
                with self.subTest(mode=mode):
                    request = financial_request()
                    request.calendar.knowledge_mode = mode
                    request.calendar.ClearField("as_of_business_date")
                    with self.assertRaises(grpc.RpcError) as failure:
                        stub.Predict(request, timeout=3)
                    self.assertEqual(failure.exception.code(), grpc.StatusCode.INVALID_ARGUMENT)
            request = financial_request()
            request.model.model_version = "mean_90_components_v2"
            request.model.preprocessing_version = "calendar_analysis_v2"
            request.model.configuration_id = "mean_90_components_analysis90_v2"
            with self.assertRaises(grpc.RpcError) as failure:
                stub.Predict(request, timeout=3)
            self.assertEqual(failure.exception.code(), grpc.StatusCode.FAILED_PRECONDITION)

    def test_v1_rejects_new_field_presence_without_activating_v2(self):
        server, port = create_server(port=0, insecure_local=True)
        server.start()
        self.addCleanup(lambda: server.stop(0).wait(timeout=3))
        with grpc.insecure_channel(f"127.0.0.1:{port}") as channel:
            stub = rpc.ForecastServiceStub(channel)
            for field in ("scenario_context", "category_rule_id"):
                with self.subTest(profile="financial_v1", field=field):
                    request = financial_request()
                    if field == "scenario_context":
                        # Even an empty message has presence and cannot be ignored.
                        request.calendar.scenario_context.SetInParent()
                    else:
                        request.operations[0].category_rule_id = "unimplemented-rule"
                    with self.assertRaises(grpc.RpcError) as failure:
                        stub.Predict(request, timeout=3)
                    self.assertEqual(failure.exception.code(), grpc.StatusCode.INVALID_ARGUMENT)
                with self.subTest(profile="baseline_v1", field=field):
                    baseline = make_request()
                    if field == "scenario_context":
                        baseline.calendar.scenario_context.SetInParent()
                    else:
                        baseline.operations.add(category_rule_id="unimplemented-rule")
                    with self.assertRaises(grpc.RpcError) as failure:
                        stub.Predict(baseline, timeout=3)
                    self.assertEqual(failure.exception.code(), grpc.StatusCode.FAILED_PRECONDITION)
