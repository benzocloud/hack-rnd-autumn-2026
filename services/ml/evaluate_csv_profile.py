"""Fixed descriptive CSV-profile evaluation; no fitting or probability calibration."""

import argparse
import csv
import hashlib
import json
from collections import Counter, defaultdict
from datetime import date, timedelta
from decimal import Decimal
from pathlib import Path
from uuid import NAMESPACE_URL, uuid5

from cashflow_rpc.csv_forecast import (
    CSV_PREPROCESSING,
    CSV_VERSION,
    LIMITATIONS,
    QUALITY_VERSION,
    csv_forecast,
)
from cashflow_rpc.inference import InvalidSnapshot
from ml.v1 import forecast_pb2 as pb

CUTOFFS = (date(2026, 7, 31), date(2026, 8, 31))


def minor(raw):
    value = Decimal(raw) * 100
    if value != value.to_integral_value():
        raise ValueError("Money has fractional minor units")
    return int(value)


def request_for(rows, company, cutoff):
    start = cutoff - timedelta(days=89)
    past = [r for r in rows if r["operation_date"] <= cutoff.isoformat()]
    if not past or min(r["operation_date"] for r in past) > start.isoformat():
        raise InvalidSnapshot("history_window_too_short")
    by_day = defaultdict(lambda: [0, 0])
    eod = {}
    for r in past:
        day = r["operation_date"]
        balance = minor(r["balance_eod"])
        if day in eod and eod[day] != balance:
            raise InvalidSnapshot("conflicting_eod")
        eod[day] = balance
        by_day[day][0 if r["direction"] == "Поступление" else 1] += minor(r["amount"])
    days = sorted(eod)
    for prev, current in zip(days, days[1:]):
        if current >= start.isoformat() and eod[current] != eod[prev] + sum(
            inc - out for day, (inc, out) in by_day.items() if prev < day <= current
        ):
            raise InvalidSnapshot("history_reconciliation_failed")
    identifier = str(uuid5(NAMESPACE_URL, company))
    snapshot_id = str(uuid5(NAMESPACE_URL, company + cutoff.isoformat()))
    request = pb.PredictRequest(
        snapshot=pb.ForecastSnapshot(
            snapshot_id=snapshot_id,
            company_id=identifier,
            account_id=identifier,
            data_version=1,
            currency="RUB",
            cutoff_date=cutoff.isoformat(),
            initial_balance_minor=eod[cutoff.isoformat()],
        ),
        horizon_days=30,
        model=pb.ModelContext(
            model_name="mean_90",
            model_version=CSV_VERSION,
            preprocessing_version=CSV_PREPROCESSING,
            configuration_id=CSV_VERSION,
        ),
        csv_history=pb.CsvHistoryContext(
            rule_version=QUALITY_VERSION,
            from_date=start.isoformat(),
            through_date=cutoff.isoformat(),
            first_operation_date=min(eod),
            data_version=1,
        ),
        calendar=pb.FinancialCalendar(
            snapshot_id=snapshot_id,
            data_version=1,
            knowledge_mode=pb.CALENDAR_KNOWLEDGE_MODE_UNAVAILABLE,
        ),
        availability_limitations=LIMITATIONS,
    )
    for n in range(90):
        day = (start + timedelta(days=n)).isoformat()
        raw = request.snapshot.history.add(date=day, source=pb.DAY_SOURCE_UNAVAILABLE)
        if day in by_day:
            raw.inflow_minor, raw.outflow_minor = by_day[day]
            raw.closing_balance_minor = eod[day]
            raw.source = pb.DAY_SOURCE_PARTIAL
    for r in past:
        if r["operation_date"] < start.isoformat():
            continue
        amount = minor(r["amount"])
        request.operations.add(
            operation_id=str(uuid5(NAMESPACE_URL, company + r["operation_id"])),
            date=r["operation_date"],
            version=1,
            inflow_minor=amount if r["direction"] == "Поступление" else 0,
            outflow_minor=amount if r["direction"] == "Списание" else 0,
            category_code="unknown",
            category_source="unknown",
            factual_source_kind="imported",
            source_namespace="dataset_v1",
            external_operation_id=r["operation_id"],
        )
    return request


def evaluate(dataset):
    counts, windows, hashes = Counter(), [], []
    model_errors, naive_errors = [], []
    for path in sorted((dataset / "statements").glob("*.csv")):
        hashes.append(path.name + ":" + hashlib.sha256(path.read_bytes()).hexdigest())
        with path.open(encoding="utf-8-sig", newline="") as source:
            rows = list(csv.DictReader(source, delimiter=";"))
        for nominal in CUTOFFS:
            available = [
                r["operation_date"] for r in rows if r["operation_date"] <= nominal.isoformat()
            ]
            if not available:
                counts["no_past_operations"] += 1
                continue
            cutoff = date.fromisoformat(max(available))
            try:
                request = request_for(rows, path.stem, cutoff)
                result = csv_forecast(request, pb)
            except InvalidSnapshot as exc:
                counts[str(exc)] += 1
                continue
            truth = {
                r["operation_date"]: minor(r["balance_eod"])
                for r in rows
                if cutoff.isoformat()
                < r["operation_date"]
                <= (cutoff + timedelta(days=30)).isoformat()
            }
            if not truth:
                counts["no_observed_future_eod"] += 1
                continue
            errors = [
                abs(point.reference_balance_minor - truth[point.date])
                for point in result.days
                if point.date in truth
            ]
            naive = [abs(request.snapshot.initial_balance_minor - v) for v in truth.values()]
            model_errors.extend(errors)
            naive_errors.extend(naive)
            windows.append(
                {
                    "company": path.stem,
                    "cutoff": cutoff.isoformat(),
                    "nominal_cutoff": nominal.isoformat(),
                    "observed_truth_days": len(errors),
                    "absolute_error_minor_sum": sum(errors),
                }
            )
            counts["evaluated"] += 1

    def mean(values):
        return (
            str((Decimal(sum(values)) / len(values)).quantize(Decimal("0.01"))) if values else None
        )

    return {
        "protocol": "csv_mean90_observed_eod_descriptive_v1",
        "model_version": CSV_VERSION,
        "nominal_cutoffs": [d.isoformat() for d in CUTOFFS],
        "dataset_statements_sha256": hashlib.sha256("\n".join(hashes).encode()).hexdigest(),
        "companies": len(hashes),
        "evaluated_companies": len({w["company"] for w in windows}),
        "counts": dict(counts),
        "observed_truth_points": len(model_errors),
        "model_mae_minor": mean(model_errors),
        "constant_eod_mae_minor": mean(naive_errors),
        "limitations": [
            "observed_future_eod_only",
            "no_zero_filled_truth",
            "unequal_company_weights",
            "no_blind_holdout",
            "no_promotion_gate",
            "no_probability_calibration",
        ],
        "windows": windows,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dataset", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    result = evaluate(args.dataset)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps({k: v for k, v in result.items() if k != "windows"}, indent=2))


if __name__ == "__main__":
    main()
