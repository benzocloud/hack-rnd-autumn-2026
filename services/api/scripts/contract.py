#!/usr/bin/env python3
"""Validate captured real HTTP responses against the repository's unchanged OpenAPI."""

import json
import sys
from pathlib import Path

import yaml
from jsonschema import Draft202012Validator, FormatChecker


def validate(report, extra_reports=()):
    root = Path(__file__).resolve().parents[3]
    spec = yaml.safe_load((root / "docs/api/openapi.yaml").read_text())
    results = json.loads(report.read_text())
    for extra in extra_reports:
        results.extend(json.loads(extra.read_text()))
    checked = 0
    for result in results:
        operation = spec["paths"][result["path"].removeprefix("/api/v1")][result["method"].lower()]
        response = operation["responses"][str(result["status"])]
        while "$ref" in response:
            response = spec["components"]["responses"][response["$ref"].rsplit("/", 1)[1]]
        if "content" not in response:
            assert result["body"] is None
        else:
            schema = response["content"]["application/json"]["schema"] | {
                "components": spec["components"]
            }
            Draft202012Validator(schema, format_checker=FormatChecker()).validate(result["body"])
        checked += 1
    assert checked >= 20
    print(
        f"OpenAPI {spec['info']['version']}: {checked} real HTTP responses validated; no contract changes"
    )


if __name__ == "__main__":
    validate(
        Path(sys.argv[1] if len(sys.argv) > 1 else "artifacts/smoke.json"),
        [Path(p) for p in sys.argv[2:]],
    )
