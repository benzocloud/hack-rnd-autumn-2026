"""Check complete REST routing and transport presence against compiled proto."""

import json
import sys
from pathlib import Path

import yaml
from google.protobuf import descriptor_pb2


def check(descriptor_path):
    root = Path(__file__).resolve().parents[1]
    spec = yaml.safe_load((root / "docs/api/openapi.yaml").read_text())
    routes = json.loads((root / "docs/core/rest-rpc-map.json").read_text())
    operations = {
        operation["operationId"]
        for path in spec["paths"].values()
        for operation in path.values()
        if isinstance(operation, dict) and "operationId" in operation
    }
    if operations != routes.keys():
        raise ValueError(f"REST routing mismatch: {operations ^ routes.keys()}")
    descriptor = descriptor_pb2.FileDescriptorSet.FromString(Path(descriptor_path).read_bytes())
    methods, messages = {}, {}
    for file in descriptor.file:
        for service in file.service:
            for method in service.method:
                methods[f"{file.package}.{service.name}.{method.name}"] = method
        for message in file.message_type:
            messages[f".{file.package}.{message.name}"] = message
    ml_methods = {name for name in methods if name.startswith("ml.v1.")}
    if ml_methods != {"ml.v1.ForecastService.Predict"}:
        raise ValueError(f"Exactly one ML Predict RPC required: {ml_methods}")
    for name, rpc_names in routes.items():
        if not rpc_names or any(rpc not in methods for rpc in rpc_names):
            raise ValueError(f"Missing RPC for {name}: {rpc_names}")
    for full_name, method in methods.items():
        if not full_name.startswith("core.v1."):
            continue
        request = messages[method.input_type]
        if method.name in ("CreateCompanyBootstrap", "GetCompanyCreationResult"):
            continue
        if method.client_streaming:
            request = messages[".core.v1.UploadImportMetadata"]
        context = next((f for f in request.field if f.name == "context"), None)
        read = method.name.startswith(("Get", "Read", "List", "Compare"))
        expected = ".auth.v1.BrowserContext" if read else ".auth.v1.WriteContext"
        if context is None or context.type_name != expected:
            raise ValueError(f"Wrong authorization context on {full_name}")
    for message_name, oneof_name, branches in (
        ("core.v1.Probability", "estimate", {"value", "unavailable_reason"}),
        ("core.v1.Forecast", "state", {"pending", "succeeded", "failed"}),
        ("core.v1.CreateScenarioRequest", "scenario", {"independent", "response"}),
        (
            "core.v1.ScenarioChange",
            "change",
            {"delay_receipt", "reduce_sales", "one_off_expense", "increase_recurring_expenses"},
        ),
        ("core.v1.UpdateCompanyRequest", "inn_change", {"inn", "clear_inn"}),
        ("core.v1.UpdatePaymentRequest", "category_change", {"category_id", "clear_category"}),
        ("ml.v1.FinancialProbability", "estimate", {"calibrated", "unavailable_reason"}),
    ):
        message = messages["." + message_name]
        index = next(i for i, item in enumerate(message.oneof_decl) if item.name == oneof_name)
        actual = {
            f.name for f in message.field if f.HasField("oneof_index") and f.oneof_index == index
        }
        if actual != branches:
            raise ValueError(f"Wrong union {message_name}.{oneof_name}: {actual}")
    money = messages[".core.v1.Money"]
    if money.field[0].type != descriptor_pb2.FieldDescriptorProto.TYPE_INT64:
        raise ValueError("Money must use exact signed int64")
    upload = methods["core.v1.ImportsService.UploadImport"]
    if not upload.client_streaming or upload.server_streaming:
        raise ValueError("File import must be client streaming")
    for message_name, field_name in (
        ("core.v1.UpdateCompanyRequest", "name"),
        ("core.v1.UpdatePaymentRequest", "direction"),
        ("core.v1.ForecastModelContext", "random_seed"),
        ("core.v1.Comparison", "delta_gap_probability"),
        ("ml.v1.ForecastSnapshot", "initial_balance_minor"),
    ):
        field = next(f for f in messages["." + message_name].field if f.name == field_name)
        if not field.proto3_optional:
            raise ValueError(f"Lost presence: {message_name}.{field_name}")
    print(
        f"{len(operations)} REST operations mapped; contexts, money, streaming and unions verified"
    )


if __name__ == "__main__":
    check(sys.argv[1])
