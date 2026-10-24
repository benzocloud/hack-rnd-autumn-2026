FROM python:3.12-slim AS build
WORKDIR /app
COPY services/ml/requirements-rpc.txt services/ml/requirements-rpc-dev.txt ./
RUN pip install --no-cache-dir -r requirements-rpc-dev.txt
COPY proto/ml/v1/forecast.proto /proto/ml/v1/forecast.proto
RUN python -m grpc_tools.protoc -I/proto --python_out=/app --grpc_python_out=/app /proto/ml/v1/forecast.proto

FROM python:3.12-slim
WORKDIR /app
COPY services/ml/requirements-rpc.txt ./
RUN pip install --no-cache-dir -r requirements-rpc.txt && useradd --uid 65532 --no-create-home --shell /usr/sbin/nologin ml
COPY --from=build /app/ml ./ml
COPY services/ml/cashflow_rpc ./cashflow_rpc
COPY services/ml/cashflow_baseline ./cashflow_baseline
COPY services/ml/run_server.py ./
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1
USER 65532:65532
ENTRYPOINT ["python", "run_server.py"]
