"""Rolling-origin evaluation; prediction never receives future observations."""

from collections import Counter, defaultdict
from dataclasses import dataclass
from datetime import date, timedelta
from statistics import mean, median
from typing import Dict, List, Optional

from .data import Company, Daily, daily_as_of, history_at
from .models import Prediction, predict

METHODS = ("mean_90", "weekday_8w")


@dataclass(frozen=True)
class Config:
    start: date
    end: date
    horizon: int = 30
    stride: int = 30
    lookback: int = 90
    weekly_weeks: int = 8
    validation_start: date = date(2026, 1, 1)
    evaluation_start: date = date(2026, 4, 1)
    evaluation_end: date = date(2026, 9, 30)

    def validate(self):
        if self.start > self.end or self.horizon < 1 or self.stride < self.horizon:
            raise ValueError("Ordered dates and stride >= positive horizon are required")
        if self.weekly_weeks != 8 or self.lookback < 90:
            raise ValueError("Fixed methods require 8 weeks and at least 90 days of history")
        if not self.validation_start < self.evaluation_start <= self.evaluation_end:
            raise ValueError("Invalid temporal split boundaries")


def split_for(cutoff: date, config: Config) -> Optional[str]:
    start, end = cutoff + timedelta(days=1), cutoff + timedelta(days=config.horizon)
    if end < config.validation_start:
        return "development"
    if start >= config.validation_start and end < config.evaluation_start:
        return "validation"
    if start >= config.evaluation_start and end <= config.evaluation_end:
        return "evaluation"
    return None


@dataclass
class ForecastRecord:
    company: str
    industry: str
    cutoff: date
    split: str
    method: str
    initial_balance: int
    history: List[Daily]
    truth: List[Daily]
    predictions: List[Prediction]


def truth_window(company: Company, full: Dict[date, Daily], cutoff: date, horizon: int):
    end = cutoff + timedelta(days=horizon)
    if end > company.last_date:
        return [], "future_not_fully_observed"
    truth = [full[cutoff + timedelta(days=i)] for i in range(1, horizon + 1)]
    if any(day.gap or day.balance is None for day in truth):
        return [], "future_reconciliation_gap"
    if any(day.duplicates for day in truth):
        return [], "future_duplicate_candidate"
    return truth, ""


def evaluate(companies: List[Company], config: Config):
    config.validate()
    records = []
    skipped = Counter()
    eligibility = []
    for company in companies:
        # Complete-history reconstruction is used ONLY to audit ground truth.
        # It is never passed to history_at() or predict().
        full = daily_as_of(company, company.last_date)
        cutoff = config.start
        while cutoff <= config.end:
            split = split_for(cutoff, config)
            reason = "split_boundary" if split is None else ""
            history, truth = [], []
            if not reason:
                history, reason = history_at(company, cutoff, config.lookback)
            if not reason:
                truth, reason = truth_window(company, full, cutoff, config.horizon)
            eligibility.append(
                {
                    "company_id": company.company_id,
                    "cutoff_date": cutoff.isoformat(),
                    "split": split or "excluded",
                    "eligible": not bool(reason),
                    "reason": reason,
                }
            )
            if reason:
                skipped[reason] += 1
            else:
                for method in METHODS:
                    forecasts = predict(history, config.horizon, method, config.weekly_weeks)
                    records.append(
                        ForecastRecord(
                            company.company_id,
                            company.industry,
                            cutoff,
                            split,
                            method,
                            history[-1].balance,
                            history,
                            truth,
                            forecasts,
                        )
                    )
            cutoff += timedelta(days=config.stride)
    return records, dict(skipped), eligibility


def safe_ratio(numerator: float, denominator: float):
    return numerator / denominator if denominator else None


def first_gap(days):
    return next((day.day for day in days if day.balance is not None and day.balance < 0), None)


def classification(records):
    counts = {"tp": 0, "fp": 0, "fn": 0, "tn": 0}
    date_errors, available_leads = [], []
    for record in records:
        actual, predicted = first_gap(record.truth), first_gap(record.predictions)
        name = ("tp" if predicted else "fn") if actual else ("fp" if predicted else "tn")
        counts[name] += 1
        if actual and predicted:
            date_errors.append(abs((predicted - actual).days))
            available_leads.append((actual - record.cutoff).days)
    return {
        **counts,
        "windows": len(records),
        "actual_positive_windows": counts["tp"] + counts["fn"],
        "positive_companies": len({r.company for r in records if first_gap(r.truth)}),
        "precision": safe_ratio(counts["tp"], counts["tp"] + counts["fp"]),
        "recall": safe_ratio(counts["tp"], counts["tp"] + counts["fn"]),
        "false_positive_rate": safe_ratio(counts["fp"], counts["fp"] + counts["tn"]),
        "mean_first_gap_date_error_days_tp": mean(date_errors) if date_errors else None,
        "mean_available_warning_days_tp": mean(available_leads) if available_leads else None,
    }


def aggregate(records):
    if not records:
        return {"windows": 0}
    errors = Counter()
    company_flow = defaultdict(Counter)
    company_balance = defaultdict(list)
    closing_errors, shortfall_errors = [], []
    inferred_history, inferred_truth, one_cent_history = 0, 0, 0
    for record in records:
        for stream in ("inflow", "outflow"):
            actual_total = sum(getattr(day, stream) for day in record.truth)
            predicted_total = sum(getattr(day, stream) for day in record.predictions)
            errors[stream + "_horizon_abs"] += abs(actual_total - predicted_total)
        for actual, predicted in zip(record.truth, record.predictions):
            errors["inflow_abs"] += abs(actual.inflow - predicted.inflow)
            errors["outflow_abs"] += abs(actual.outflow - predicted.outflow)
            errors["balance_abs"] += abs(actual.balance - predicted.balance)
            errors["inflow_actual"] += actual.inflow
            errors["outflow_actual"] += actual.outflow
            errors["days"] += 1
            totals = company_flow[record.company]
            totals["inflow_abs"] += abs(actual.inflow - predicted.inflow)
            totals["outflow_abs"] += abs(actual.outflow - predicted.outflow)
            totals["inflow_actual"] += actual.inflow
            totals["outflow_actual"] += actual.outflow
        company_balance[record.company].append(
            mean(abs(a.balance - p.balance) / 100 for a, p in zip(record.truth, record.predictions))
        )
        closing_errors.append(abs(record.truth[-1].balance - record.predictions[-1].balance) / 100)
        actual_shortfall = max(0, -min(day.balance for day in record.truth))
        predicted_shortfall = max(0, -min(day.balance for day in record.predictions))
        if record.initial_balance >= 0 and actual_shortfall:
            shortfall_errors.append(abs(actual_shortfall - predicted_shortfall) / 100)
        inferred_history += sum(not day.observed_balance for day in record.history)
        inferred_truth += sum(not day.observed_balance for day in record.truth)
        one_cent_history += sum(abs(day.reconciliation or 0) == 1 for day in record.history)
    result = {
        "windows": len(records),
        "companies": len(company_flow),
        "forecast_days": errors["days"],
        "inflow_wape": safe_ratio(errors["inflow_abs"], errors["inflow_actual"]),
        "outflow_wape": safe_ratio(errors["outflow_abs"], errors["outflow_actual"]),
        "horizon_inflow_wape": safe_ratio(errors["inflow_horizon_abs"], errors["inflow_actual"]),
        "horizon_outflow_wape": safe_ratio(errors["outflow_horizon_abs"], errors["outflow_actual"]),
        "inflow_mae": errors["inflow_abs"] / errors["days"] / 100,
        "outflow_mae": errors["outflow_abs"] / errors["days"] / 100,
        "balance_mae": errors["balance_abs"] / errors["days"] / 100,
        "closing_balance_mae": mean(closing_errors),
        "company_median_balance_mae": median(mean(v) for v in company_balance.values()),
        "new_gap_shortfall_mae": mean(shortfall_errors) if shortfall_errors else None,
        "new_gap": classification([r for r in records if r.initial_balance >= 0]),
        "continuing_gap": classification([r for r in records if r.initial_balance < 0]),
        "inferred_history_day_uses": inferred_history,
        "inferred_truth_day_uses": inferred_truth,
        "one_cent_history_warning_uses": one_cent_history,
    }
    for stream in ("inflow", "outflow"):
        company_wapes = [
            values[stream + "_abs"] / values[stream + "_actual"]
            for values in company_flow.values()
            if values[stream + "_actual"]
        ]
        result[stream + "_company_median_wape"] = median(company_wapes) if company_wapes else None
        result[stream + "_companies_with_wape"] = len(company_wapes)
    return result


def all_metrics(records):
    grouped = defaultdict(list)
    by_company, by_industry = defaultdict(list), defaultdict(list)
    for record in records:
        grouped[(record.split, record.method)].append(record)
        by_company[(record.split, record.method, record.company)].append(record)
        by_industry[(record.split, record.method, record.industry)].append(record)
    return {
        "by_split": [
            {"split": split, "method": method, **aggregate(values)}
            for (split, method), values in sorted(grouped.items())
        ],
        "by_company": [
            {"split": split, "method": method, "company_id": company, **aggregate(values)}
            for (split, method, company), values in sorted(by_company.items())
        ],
        "by_industry": [
            {"split": split, "method": method, "industry": industry, **aggregate(values)}
            for (split, method, industry), values in sorted(by_industry.items())
        ],
    }
