"""Train and evaluate the fixed compact financial pool on temporal CSV windows."""

import argparse
import hashlib
import json
import math
import platform
import statistics
import time
from collections import defaultdict
from datetime import date, timedelta
from fractions import Fraction
from pathlib import Path

from cashflow_baseline.data import load_companies, sha256
from cashflow_baseline.evaluation import Config, ForecastRecord, aggregate, evaluate, first_gap
from cashflow_baseline.models import Prediction
from cashflow_rpc.trained_pool import DENOMINATOR, SEED, feature_numerators, parse_artifact

TRAIN_THROUGH = date(2025, 9, 30)
WEIGHTS = sorted(((a, b, 4 - a - b) for a in range(5) for b in range(5 - a)), reverse=True)
SOURCE_SHA = {
    "companies.csv": "0669f211a241ce6e38bb0fd373ea1d84a5a17cfab3833741662382c0669c2cd6",
    "all_operations.csv": "28d7a2ebc6d4d65f31f93e59e24659a04c76990d20833337b9351316fd445472",
}


def save(path, value):
    path.write_text(
        json.dumps(value, ensure_ascii=False, indent=2, allow_nan=False) + "\n", encoding="utf-8"
    )


def basis(record):
    return feature_numerators([(d.day, {"all": (d.inflow, d.outflow)}) for d in record.history])


def basis_flows(features, direction, weights):
    return [
        (sum(a * b for a, b in zip(d["all"][direction], weights, strict=True)) + DENOMINATOR // 2)
        // DENOMINATOR
        for d in features
    ]


def fit(records):
    """Exact rational objective, one fixed 225-point grid; only mature train rows."""
    train = [r for r in records if r.truth[-1].day <= TRAIN_THROUGH]
    if len(train) < 100 or len({r.company for r in train}) < 20:
        raise ValueError("Not enough matured training companies/windows")
    features = [basis(r) for r in train]
    incoming = [[basis_flows(b, 0, w) for b in features] for w in WEIGHTS]
    outgoing = [[basis_flows(b, 1, w) for b in features] for w in WEIGHTS]
    baseline_fp = aggregate(train)["new_gap"]["fp"]
    losses, chosen = [], None
    for ii, iw in enumerate(WEIGHTS):
        for oi, ow in enumerate(WEIGHTS):
            companies = defaultdict(list)
            fp = 0
            for index, r in enumerate(train):
                balance, error, minimum = r.initial_balance, 0, r.initial_balance
                for inc, out, truth in zip(
                    incoming[ii][index], outgoing[oi][index], r.truth, strict=True
                ):
                    balance += inc - out
                    error += abs(balance - truth.balance)
                    minimum = min(minimum, balance)
                fp += r.initial_balance >= 0 and first_gap(r.truth) is None and minimum < 0
                past_turnover = max(90, sum(d.inflow + d.outflow for d in r.history))
                companies[r.company].append(Fraction(error * 90, 30 * past_turnover))
            objective = sum(sum(v) / len(v) for v in companies.values()) / len(companies)
            rank = objective, -(iw[0] + ow[0]), iw, ow
            allowed = fp <= baseline_fp
            losses.append(
                dict(
                    inflow=iw,
                    outflow=ow,
                    normalized_company_mae=float(objective),
                    fp=fp,
                    allowed=allowed,
                )
            )
            if allowed and (chosen is None or rank < chosen[0]):
                chosen = rank, dict(inflow=list(iw), outflow=list(ow))
    if chosen is None:
        raise ValueError("Baseline corner missing from the admissible grid")
    return (
        chosen[1],
        losses,
        dict(
            windows=len(train),
            companies=len({r.company for r in train}),
            max_target_date=max(r.truth[-1].day for r in train).isoformat(),
            baseline_fp=baseline_fp,
            chosen_loss=float(chosen[0][0]),
        ),
    )


def candidate(record, weights):
    features = basis(record)
    inc = basis_flows(features, 0, weights["inflow"])
    out = basis_flows(features, 1, weights["outflow"])
    predicted, balance = [], record.initial_balance
    for step, (incoming, outgoing) in enumerate(zip(inc, out, strict=True), 1):
        balance += incoming - outgoing
        predicted.append(
            Prediction(record.cutoff + timedelta(days=step), incoming, outgoing, balance)
        )
    return ForecastRecord(
        record.company,
        record.industry,
        record.cutoff,
        record.split,
        "linear_pool_90",
        record.initial_balance,
        record.history,
        record.truth,
        predicted,
    )


def metrics(records):
    result = aggregate(records)
    if not records:
        return result
    squared = sum(
        (a.balance - p.balance) ** 2
        for r in records
        for a, p in zip(r.truth, r.predictions, strict=True)
    )
    result["balance_rmse"] = math.sqrt(squared / (30 * len(records))) / 100
    result["closing_balance_rmse"] = (
        math.sqrt(
            sum((r.truth[-1].balance - r.predictions[-1].balance) ** 2 for r in records)
            / len(records)
        )
        / 100
    )
    found, total, false_months = set(), set(), set()
    for r in records:
        if r.initial_balance < 0:
            continue
        actual, predicted = first_gap(r.truth), first_gap(r.predictions)
        if actual:
            total.add((r.company, actual.isoformat()))
            if predicted:
                found.add((r.company, actual.isoformat()))
        elif predicted:
            false_months.add((r.company, r.cutoff.strftime("%Y-%m")))
    gap = result["new_gap"]
    denom = 2 * gap["tp"] + gap["fp"] + gap["fn"]
    gap["f1"] = 2 * gap["tp"] / denom if denom else None
    result.update(
        episodes_found=len(found), episodes_total=len(total), fp_company_months=len(false_months)
    )
    return result


def quality_gate(tables):
    checks = {}
    for split in ("development", "validation"):
        base, learned = tables[split]["mean_90"], tables[split]["linear_pool_90"]
        checks[split] = bool(
            base["windows"]
            and learned["balance_mae"] < base["balance_mae"]
            and learned["new_gap"]["fp"] <= base["new_gap"]["fp"]
            and learned["episodes_found"] >= base["episodes_found"]
        )
    return dict(
        recommend_replacement=all(checks.values()),
        checks=checks,
        limitation="Previously studied dataset; no independent quality confirmation",
    )


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--data-dir", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    args = parser.parse_args(argv)
    output = args.output_dir.resolve()
    if output.exists():
        parser.error("Output directory must be new")
    sources = {n: sha256(args.data_dir / n) for n in SOURCE_SHA}
    if sources != SOURCE_SHA:
        parser.error("This frozen protocol requires the pinned participant dataset")
    here = Path(__file__).resolve().parent
    code = {
        str(p): sha256(p)
        for p in (
            Path(__file__),
            here / "cashflow_rpc/trained_pool.py",
            here / "TRAINED_MODEL_PLAN.md",
        )
    }
    output.mkdir(parents=True)
    save(
        output / "manifest_before_fit.json",
        dict(
            source_sha256=sources,
            code_sha256=code,
            python=platform.python_version(),
            training_through=TRAIN_THROUGH.isoformat(),
            seed=SEED,
            grid_points=225,
            grid=list(WEIGHTS),
            validation_used_for_selection=False,
        ),
    )
    for path in code:
        p = Path(path)
        (output / ("frozen_" + p.name)).write_bytes(p.read_bytes())
    config = Config(date(2025, 1, 1), date(2026, 8, 31))
    records, skipped, eligibility = evaluate(load_companies(args.data_dir), config)
    baseline = [r for r in records if r.method == "mean_90"]
    save(output / "eligibility.json", eligibility)
    print(f"eligible={len(baseline)}; fitting 225 fixed combinations", flush=True)
    weights, losses, train_info = fit(baseline)
    artifact = dict(
        schema_version=1,
        algorithm="convex_flow_pool_quarters_v1",
        trained_through=TRAIN_THROUGH.isoformat(),
        weights=weights,
        source_sha256=sources,
        seed=SEED,
    )
    payload = (json.dumps(artifact, sort_keys=True, indent=2) + "\n").encode()
    checksum = hashlib.sha256(payload).hexdigest()
    parse_artifact(payload, checksum)
    (output / "linear_pool_v1.json").write_bytes(payload)
    (output / "linear_pool_v1.sha256").write_text(checksum + "\n", encoding="ascii")
    save(output / "training.json", dict(train_info, weights=weights, grid=losses))
    print(f"frozen weights={weights}; sha256={checksum}", flush=True)
    # Both the model and its selection are fixed before any evaluation metrics.
    later = [r for r in records if r.cutoff > TRAIN_THROUGH]
    learned = [candidate(r, weights) for r in later if r.method == "mean_90"]
    all_records = later + learned
    tables = {}
    for split in ("development", "validation"):
        tables[split] = {
            m: metrics([r for r in all_records if r.split == split and r.method == m])
            for m in ("mean_90", "weekday_8w", "linear_pool_90")
        }
    save(output / "verdict_before_late.json", quality_gate(tables))
    tables["evaluation"] = {
        m: metrics([r for r in all_records if r.split == "evaluation" and r.method == m])
        for m in ("mean_90", "weekday_8w", "linear_pool_90")
    }
    per_company = {
        company: {
            s: {
                m: metrics(
                    [
                        r
                        for r in all_records
                        if r.company == company and r.split == s and r.method == m
                    ]
                )
                for m in ("mean_90", "weekday_8w", "linear_pool_90")
            }
            for s in tables
        }
        for company in sorted({r.company for r in later})
    }
    latencies = []
    for r in [v for v in later if v.method == "mean_90"]:
        started = time.perf_counter()
        candidate(r, weights)
        latencies.append((time.perf_counter() - started) * 1000)
    save(output / "metrics.json", tables)
    save(output / "per_company.json", per_company)
    save(
        output / "coverage.json",
        dict(
            eligible_baseline=len(baseline),
            evaluated=len(learned),
            model_unavailable_before_training=len(baseline) - len(learned),
            exclusions=skipped,
            latency_core_only_ms=dict(
                p50=statistics.median(latencies),
                p95=sorted(latencies)[math.ceil(0.95 * len(latencies)) - 1],
            ),
        ),
    )
    # Detailed financial rows are private artifacts, never part of the PR.
    save(
        output / "predictions.json",
        [
            dict(
                company=r.company,
                cutoff=r.cutoff.isoformat(),
                split=r.split,
                initial=r.initial_balance,
                actual=[[d.day.isoformat(), d.inflow, d.outflow, d.balance] for d in r.truth],
                forecast=[
                    [d.day.isoformat(), d.inflow, d.outflow, d.balance] for d in r.predictions
                ],
            )
            for r in learned
        ],
    )
    for path, expected in {
        **{str(args.data_dir / n): h for n, h in sources.items()},
        **code,
    }.items():
        if sha256(Path(path)) != expected:
            raise ValueError("Sources/protocol changed during run")
    print("Completed; sources and frozen code unchanged", flush=True)


if __name__ == "__main__":
    main()
