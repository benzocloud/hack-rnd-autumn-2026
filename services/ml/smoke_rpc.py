"""Synthetic client for the explicitly enabled local ML smoke endpoint."""

import argparse
import json
from datetime import date, timedelta

import grpc
from ml.v1 import forecast_pb2 as pb
from ml.v1 import forecast_pb2_grpc as rpc


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=50051)
    args = parser.parse_args()
    request = pb.PredictRequest(
        horizon_days=30,
        model=pb.ModelContext(
            model_name="mean_90",
            model_version="mean_90_v1",
            preprocessing_version="daily_snapshot_v1",
        ),
    )
    snapshot = request.snapshot
    snapshot.snapshot_id = "019a1234-0000-7000-8000-000000000001"
    snapshot.company_id = "019a1234-0000-7000-8000-000000000002"
    snapshot.account_id = "019a1234-0000-7000-8000-000000000003"
    snapshot.data_version = 1
    snapshot.currency = "RUB"
    cutoff = date(2026, 10, 23)
    snapshot.cutoff_date = cutoff.isoformat()
    snapshot.initial_balance_minor = 0
    balance = 180
    for step in range(90):
        balance -= 2
        snapshot.history.add(
            date=(cutoff - timedelta(days=89 - step)).isoformat(),
            inflow_minor=3,
            outflow_minor=5,
            closing_balance_minor=balance,
            source=pb.DAY_SOURCE_OBSERVED,
        )
    with grpc.insecure_channel(f"127.0.0.1:{args.port}") as channel:
        result = rpc.ForecastServiceStub(channel).Predict(request, timeout=3)
    assert len(result.days) == 30
    assert result.days[-1].reference_balance_minor == -60
    assert result.first_negative_balance_date == "2026-10-24"
    print(
        json.dumps(
            {
                "days": len(result.days),
                "closing_balance_minor": -60,
                "first_negative_balance_date": result.first_negative_balance_date,
                "model_version": result.model.model_version,
            }
        )
    )


if __name__ == "__main__":
    main()
