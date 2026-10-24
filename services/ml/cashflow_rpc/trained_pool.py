"""Small learned convex flow pool. Integer money; no dataset access at inference."""

import hashlib
import json
from collections import defaultdict
from datetime import date, timedelta
from functools import lru_cache
from pathlib import Path

from .inference import UnsupportedContext, _date

MODEL_NAME = "linear_pool_90"
MODEL_VERSION = "linear_pool_components_v1"
PREPROCESSING_VERSION = "calendar_replacement_v1"
DENOMINATOR = 1440  # lcm(90, 30, 8) * four weight units
SEED = 20261024
ASSETS = Path(__file__).with_name("model_assets")


def parse_artifact(payload, expected_hash):
    """Pinned bytes and strict tiny schema, never arbitrary pickled code."""
    if len(payload) > 16384 or hashlib.sha256(payload).hexdigest() != expected_hash:
        raise UnsupportedContext("Learned model artifact checksum mismatch")
    try:
        obj = json.loads(payload)
        valid = (
            set(obj)
            == {
                "schema_version",
                "algorithm",
                "trained_through",
                "weights",
                "source_sha256",
                "seed",
            }
            and type(obj["schema_version"]) is int
            and obj["schema_version"] == 1
            and obj["algorithm"] == "convex_flow_pool_quarters_v1"
            and obj["seed"] == SEED
            and date.fromisoformat(obj["trained_through"]).isoformat() == obj["trained_through"]
            and set(obj["weights"]) == {"inflow", "outflow"}
        )
        for values in obj["weights"].values():
            valid = valid and isinstance(values, list) and len(values) == 3
            valid = (
                valid and all(type(v) is int and 0 <= v <= 4 for v in values) and sum(values) == 4
            )
        valid = valid and set(obj["source_sha256"]) == {"companies.csv", "all_operations.csv"}
        valid = valid and all(
            isinstance(v, str) and len(v) == 64 and int(v, 16) >= 0
            for v in obj["source_sha256"].values()
        )
        if not valid:
            raise ValueError("Invalid fields")
    except (ValueError, KeyError, TypeError, AttributeError) as exc:
        raise UnsupportedContext("Invalid learned model artifact schema") from exc
    return obj


@lru_cache(maxsize=1)
def load_artifact():
    try:
        expected = (ASSETS / "linear_pool_v1.sha256").read_text(encoding="ascii").strip()
        payload = (ASSETS / "linear_pool_v1.json").read_bytes()
    except (OSError, UnicodeError) as exc:
        raise UnsupportedContext("Pinned learned model artifact is unavailable") from exc
    return parse_artifact(payload, expected), expected


def configuration_id(checksum):
    return "linear_pool_v1_sha256:" + checksum


def validate_context(request):
    model = request.model
    if (model.model_name, model.model_version, model.preprocessing_version) != (
        MODEL_NAME,
        MODEL_VERSION,
        PREPROCESSING_VERSION,
    ):
        raise UnsupportedContext("Requested learned model context is unavailable")
    artifact, checksum = load_artifact()
    if model.configuration_id != configuration_id(checksum):
        raise UnsupportedContext("Learned configuration must pin the artifact checksum")
    if (
        not model.HasField("random_seed")
        or model.random_seed != str(SEED)
        or not model.HasField("trajectory_count")
        or model.trajectory_count != 512
    ):
        raise UnsupportedContext("Learned model requires explicit seed and trajectory context")
    if _date(request.snapshot.cutoff_date) < date.fromisoformat(artifact["trained_through"]):
        raise UnsupportedContext("Model was not available at the requested historical cutoff")
    return artifact


def allocate_numerators(totals):
    result = {key: amount // DENOMINATOR for key, amount in totals.items()}
    target = (sum(totals.values()) + DENOMINATOR // 2) // DENOMINATOR
    left = target - sum(result.values())
    ranked = sorted(totals, key=lambda key: (-(totals[key] % DENOMINATOR), key))
    for key in ranked[:left]:
        result[key] += 1
    return result


def feature_numerators(history):
    """Three exact basis numerators per future day and direction; 90 covered days."""
    if len(history) != 90 or any((b[0] - a[0]).days != 1 for a, b in zip(history, history[1:])):
        raise ValueError("Require 90 consecutive historical dates")
    totals = defaultdict(lambda: [[0, 0, [0] * 7] for _ in range(2)])
    for index, (day, flows) in enumerate(history):
        for key, pair in flows.items():
            for direction, amount in enumerate(pair):
                if type(amount) is not int or amount < 0:
                    raise ValueError("Nonnegative integer flow amounts required")
                totals[key][direction][0] += amount
                if index >= 60:
                    totals[key][direction][1] += amount
                if index >= 34:
                    totals[key][direction][2][day.weekday()] += amount
    return [
        {
            key: [
                [v[0] * 4, v[1] * 12, v[2][(history[-1][0] + timedelta(days=step)).weekday()] * 45]
                for v in sides
            ]
            for key, sides in totals.items()
        }
        for step in range(1, 31)
    ]


def allocations_from_features(features, weights):
    return [
        [
            allocate_numerators(
                {
                    key: sum(a * b for a, b in zip(values[d], weights[side], strict=True))
                    for key, values in day.items()
                }
            )
            for d, side in enumerate(("inflow", "outflow"))
        ]
        for day in features
    ]


def predict_allocations(history, weights):
    return allocations_from_features(feature_numerators(history), weights)
