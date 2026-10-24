"""Synthetic known-payment request; no dataset or future facts are loaded."""

import argparse
import json
from datetime import date, timedelta
from uuid import UUID

import grpc
from cashflow_rpc.financial import FINANCIAL_PREPROCESSING, FINANCIAL_VERSION
from ml.v1 import financial_pb2 as fpb
from ml.v1 import financial_pb2_grpc as rpc
from smoke_rpc import example_request


def example_financial_request():
    base = example_request()
    request = fpb.PredictFinancialRequest(snapshot=base.snapshot, horizon_days=30, model=base.model)
    request.model.model_version = FINANCIAL_VERSION
    request.model.preprocessing_version = FINANCIAL_PREPROCESSING
    ids = []
    for i, day in enumerate(base.snapshot.history):
        request.operations.add(
            operation_id=str(UUID(int=2 * i + 1)),
            version=1,
            date=day.date,
            inflow_minor=3,
            category_code="revenue",
            category_source="manual",
        )
        identifier = str(UUID(int=2 * i + 2))
        ids.append(identifier)
        request.operations.add(
            operation_id=identifier,
            version=1,
            date=day.date,
            outflow_minor=5,
            category_code="salary",
            category_source="manual",
            regularity_key="salary-series",
        )
    request.calendar.snapshot_id = str(UUID(int=1000))
    request.calendar.data_version = base.snapshot.data_version
    request.calendar.as_of_business_date = base.snapshot.cutoff_date
    request.calendar.knowledge_mode = fpb.CALENDAR_KNOWLEDGE_MODE_CURRENT
    request.calendar.groups.add(
        group_id="salary", historical_operation_ids=ids, complete_for_horizon=True
    )
    due = (date.fromisoformat(base.snapshot.cutoff_date) + timedelta(days=5)).isoformat()
    request.calendar.payments.add(
        payment_id=str(UUID(int=2000)),
        version=1,
        effective_date=due,
        original_due_date=due,
        remaining_outflow_minor=450,
        category_code="salary",
        group_id="salary",
        source="manual",
        regularity_key="salary-series",
    )
    return request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", type=int, default=50051)
    args = parser.parse_args()
    with grpc.insecure_channel(f"127.0.0.1:{args.port}") as channel:
        result = rpc.FinancialForecastServiceStub(channel).PredictFinancial(
            example_financial_request(), timeout=3
        )
    assert len(result.forecast.days) == 30
    assert sum(d.outflow_minor for d in result.forecast.days) == 450
    assert result.forecast.days[-1].reference_balance_minor == -360
    print(
        json.dumps(
            {
                "rpc": "ml.v1.FinancialForecastService/PredictFinancial",
                "points": 30,
                "replaced_history_operations": len(result.replaced_historical_operation_ids),
                "known_outflow_minor": 450,
                "closing_balance_minor": -360,
                "uncertainty_bands": len(result.uncertainty.bands),
                "gap_probability": None,
                "requirement_open": True,
            }
        )
    )


if __name__ == "__main__":
    main()
