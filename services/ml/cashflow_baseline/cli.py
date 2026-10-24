"""Command line entry point; all outputs stay in a caller-selected local directory."""

import argparse
import csv
import json
import platform
import sys
from datetime import date, datetime, timezone
from importlib.metadata import PackageNotFoundError, version
from pathlib import Path

from .data import DataError, load_companies, sha256
from .evaluation import Config, all_metrics, evaluate
from .report import (
    config_dict,
    dump_json,
    render_examples,
    write_eligibility,
    write_predictions,
    write_reports,
)

ROOT = Path(__file__).resolve().parents[3]


def read_config(path):
    values = json.loads(path.read_text(encoding="utf-8"))
    for name in ["start", "end", "validation_start", "evaluation_start", "evaluation_end"]:
        values[name] = date.fromisoformat(values[name])
    config = Config(**values)
    config.validate()
    return config


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="Local deterministic cash-flow baseline evaluation"
    )
    parser.add_argument("--data-dir", type=Path, default=ROOT / "data/cashflow/raw")
    parser.add_argument("--output-dir", type=Path, default=ROOT / "services/ml/artifacts/baseline")
    parser.add_argument(
        "--config", type=Path, default=Path(__file__).parents[1] / "baseline_config.json"
    )
    parser.add_argument(
        "--company", action="append", help="Optional company ID; repeat for multiple companies"
    )
    parser.add_argument(
        "--no-plots", action="store_true", help="Run with the standard library only"
    )
    args = parser.parse_args(argv)
    data_dir, output = args.data_dir.resolve(), args.output_dir.resolve()
    if data_dir == output or data_dir in output.parents or output in data_dir.parents:
        parser.error("Input and output directories must not overlap")
    try:
        config = read_config(args.config)
        companies = load_companies(data_dir)
        if args.company:
            known = {company.company_id for company in companies}
            if set(args.company) - known:
                parser.error("Unknown requested company ID")
            companies = [company for company in companies if company.company_id in args.company]
        print(f"Loaded {len(companies)} companies; evaluating two baselines...", flush=True)
        records, skipped, eligibility = evaluate(companies, config)
        if not records:
            print(
                json.dumps({"error": "No eligible forecasts", "skipped": skipped}), file=sys.stderr
            )
            return 2
        metrics = all_metrics(records)
        output.mkdir(parents=True, exist_ok=True)
        write_predictions(output, records)
        write_eligibility(output, eligibility)
        dump_json(output / "metrics.json", metrics)
        dependencies = {}
        for name in ["matplotlib", "numpy", "ruff"]:
            try:
                dependencies[name] = version(name)
            except PackageNotFoundError:
                dependencies[name] = None
        manifest = {
            "generated_at_utc": datetime.now(timezone.utc).isoformat(),
            "python": platform.python_version(),
            "dependencies": dependencies,
            "config": config_dict(config),
            "config_sha256": sha256(args.config),
            "implementation_sha256": {
                path.name: sha256(path) for path in sorted(Path(__file__).parent.glob("*.py"))
            },
            "source_sha256": {
                name: sha256(data_dir / name) for name in ["all_operations.csv", "companies.csv"]
            },
            "companies_loaded": len(companies),
            "operations": sum(len(c.operations) for c in companies),
            "forecast_records": len(records),
            "paired_windows": len(records) // 2,
            "skipped": skipped,
            "gap_probability": None,
            "probability_unavailable_reason": "deterministic_baseline",
            "evaluation_is_untouched_holdout": False,
        }
        images = [] if args.no_plots else render_examples(output, records)
        manifest["examples"] = images
        dump_json(output / "manifest.json", manifest)
        write_reports(output, metrics, skipped, manifest, images)
        # Verify that exported forecast rows were not silently lost.
        with (output / "predictions.csv").open(encoding="utf-8-sig", newline="") as source:
            exported = sum(1 for _ in csv.DictReader(source, delimiter=";"))
        expected = sum(len(record.predictions) for record in records)
        if exported != expected:
            raise DataError("Forecast CSV failed its row-count check")
        print(
            json.dumps(
                {
                    "output": str(output),
                    "paired_windows": len(records) // 2,
                    "evaluation": [
                        {
                            key: item[key]
                            for key in [
                                "method",
                                "windows",
                                "companies",
                                "balance_mae",
                                "horizon_inflow_wape",
                                "horizon_outflow_wape",
                                "new_gap",
                            ]
                        }
                        for item in metrics["by_split"]
                        if item["split"] == "evaluation"
                    ],
                },
                ensure_ascii=True,
                indent=2,
            )
        )
        return 0
    except (DataError, ValueError, OSError, ImportError) as exc:
        print(f"Baseline failed: {exc}", file=sys.stderr)
        return 1
