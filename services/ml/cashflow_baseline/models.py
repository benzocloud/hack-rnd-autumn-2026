"""Two deterministic flow forecasts with integer minor-unit arithmetic."""

from dataclasses import dataclass
from datetime import date, timedelta
from typing import List

from .data import Daily, DataError


@dataclass(frozen=True)
class Prediction:
    day: date
    inflow: int
    outflow: int
    balance: int


def rounded_mean(values: List[int]) -> int:
    if not values:
        raise DataError("Cannot forecast from an empty sample")
    if any(value < 0 for value in values):
        raise DataError("Flow amounts cannot be negative")
    return (sum(values) + len(values) // 2) // len(values)


def predict(history: List[Daily], horizon: int, method: str, weekly_weeks: int = 8):
    if not history or horizon < 1:
        raise DataError("Nonempty history and positive horizon are required")
    if any(day.gap or day.duplicates for day in history):
        raise DataError("History contains a reconciliation gap or duplicate candidate")
    if any((b.day - a.day).days != 1 for a, b in zip(history, history[1:])):
        raise DataError("History must contain consecutive calendar days")
    if history[-1].balance is None or not history[-1].observed_balance:
        raise DataError("Cutoff requires an observed closing balance")
    if method == "mean_90":
        if len(history) < 90:
            raise DataError("mean_90 requires at least 90 days")
        samples = [history[-90:]] * horizon
    elif method == "weekday_8w":
        if weekly_weeks < 1 or len(history) < weekly_weeks * 7:
            raise DataError("Insufficient complete weeks")
        recent = history[-weekly_weeks * 7 :]
        samples = [
            [
                day
                for day in recent
                if day.day.weekday() == (history[-1].day + timedelta(days=i)).weekday()
            ]
            for i in range(1, horizon + 1)
        ]
    else:
        raise DataError(f"Unknown method: {method}")
    result = []
    balance = history[-1].balance
    for step, sample in enumerate(samples, 1):
        incoming = rounded_mean([day.inflow for day in sample])
        outgoing = rounded_mean([day.outflow for day in sample])
        balance += incoming - outgoing
        result.append(
            Prediction(history[-1].day + timedelta(days=step), incoming, outgoing, balance)
        )
    return result
