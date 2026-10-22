"""Issue-to-plan protocol: no source implementation or arbitrary write paths."""

import hashlib
import json
import re
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "review"))
from review_policy import REPOSITORY, SHA, safe_path

MODES = {"scope", "create-plan", "discuss-plan"}
LISTS = (
    "in_scope",
    "out_of_scope",
    "affected_files",
    "evidence",
    "steps",
    "acceptance",
    "risks",
    "questions",
)
SCHEMA = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "source_sha": {"type": "string"},
        "issue_number": {"type": "integer"},
        "status": {"type": "string", "enum": ["ready", "needs-clarification"]},
        "summary": {"type": "string"},
        "branch_description": {
            "type": "string",
            "pattern": r"^[a-z0-9]+(?:_[a-z0-9]+)*$",
            "minLength": 3,
            "maxLength": 48,
        },
        **{name: {"type": "array", "items": {"type": "string"}} for name in LISTS},
    },
    "required": ["source_sha", "issue_number", "status", "summary", "branch_description", *LISTS],
}

SCHEMA["properties"]["affected_files"]["items"]["pattern"] = r"^[^\s]+$"


def command(payload, event_name, attempt="1"):
    if attempt != "1":
        return None
    issue = payload.get("issue", {})
    if event_name == "issues" and payload.get("action") == "opened" and "pull_request" not in issue:
        return "scope"
    comment = payload.get("comment", {})
    if (
        event_name != "issue_comment"
        or payload.get("action") != "created"
        or comment.get("user", {}).get("type") != "User"
    ):
        return None
    body = comment.get("body", "")
    if "pull_request" not in issue and body == "/create-plan":
        return "create-plan"
    if "pull_request" in issue and body.startswith("/discuss-plan"):
        rest = body[len("/discuss-plan") :]
        if rest and rest[0].isspace() and rest.strip():
            return "discuss-plan"
    return None


def identity(number, description=None):
    if type(number) is not int or number < 1:
        raise ValueError("Invalid Issue number")
    branch = None
    if description is not None:
        if (
            not isinstance(description, str)
            or not 3 <= len(description) <= 48
            or not re.fullmatch(r"[a-z0-9]+(?:_[a-z0-9]+)*", description)
        ):
            raise ValueError("Invalid short branch description")
        branch = f"runner/fix/{description}/issue_{number}"
    return branch, f".agent/plans/issue-{number}-fix.md"


def is_plan_branch(branch, number):
    identity(number)
    if branch == f"codex/issue-{number}-plan":
        return True
    if not isinstance(branch, str):
        return False
    parts = branch.split("/")
    if len(parts) != 4:
        return False
    try:
        return identity(number, parts[2])[0] == branch
    except ValueError:
        return False


def fingerprint(issue):
    return hashlib.sha256(
        json.dumps(
            [issue["title"], issue.get("body") or "", issue["state"]], ensure_ascii=False
        ).encode()
    ).hexdigest()


def validate_context(context):
    if (
        context["repository"] != REPOSITORY
        or context["mode"] not in MODES
        or not SHA.fullmatch(context["source_sha"])
    ):
        raise ValueError("Invalid planning context")
    identity(context["issue_number"])
    if context["issue"]["number"] != context["issue_number"] or context[
        "issue_fingerprint"
    ] != fingerprint(context["issue"]):
        raise ValueError("Issue context mismatch")
    if context["issue"]["state"] != "open":
        raise ValueError("Issue must remain open")
    if context["mode"] == "discuss-plan":
        _, path = identity(context["issue_number"])
        if (
            not is_plan_branch(context["branch"], context["issue_number"])
            or context["plan_path"] != path
            or not context.get("existing_plan")
        ):
            raise ValueError("Not a managed plan")


def validate_report(report, context):
    validate_context(context)
    if (
        set(report) != set(SCHEMA["required"])
        or report["source_sha"] != context["source_sha"]
        or report["issue_number"] != context["issue_number"]
    ):
        raise ValueError("Planning report does not match its source/Issue")
    if not isinstance(report["branch_description"], str):
        raise ValueError("Branch description must be a string")
    identity(context["issue_number"], report["branch_description"])
    if (
        context["mode"] == "discuss-plan"
        and context["branch"].startswith("runner/")
        and report["branch_description"] != context["branch"].split("/")[2]
    ):
        raise ValueError("Plan revisions must keep the existing branch")
    if (
        report["status"] not in ("ready", "needs-clarification")
        or not isinstance(report["summary"], str)
        or not report["summary"].strip()
    ):
        raise ValueError("Invalid planning status/summary")
    for name in LISTS:
        if (
            not isinstance(report[name], list)
            or len(report[name]) > 40
            or any(not isinstance(v, str) or not v.strip() or len(v) > 3000 for v in report[name])
        ):
            raise ValueError("Invalid planning details")
    for path in report["affected_files"]:
        safe_path(path)
        if any(c.isspace() for c in path):
            raise ValueError("Affected files must be exact paths without descriptions")
    if not report["in_scope"] or not report["steps"] or not report["acceptance"]:
        raise ValueError("Scope, proposed steps and acceptance criteria are required")
    if len(json.dumps(report)) > 45000:
        raise ValueError("Plan is too large")


def render(report, context):
    validate_report(report, context)
    lines = [
        f"# План исправления Issue #{context['issue_number']}",
        report["summary"],
        f"Исходники: `{report['source_sha']}`. Статус: `{report['status']}`.",
        "Это оценка и план. Реализация требует отдельного решения разработчика. GBrain недоступен; анализ статический, тесты не запускались.",
    ]
    labels = (
        "Граница исправления",
        "Вне скоупа",
        "Затрагиваемые файлы (оценка)",
        "Основания в исходниках",
        "Шаги исправления",
        "Критерии приёмки",
        "Риски",
        "Открытые вопросы",
    )
    for name, label in zip(LISTS, labels):
        lines += ["## " + label, "\n".join("- " + value for value in report[name]) or "Нет."]
    return "\n\n".join(lines) + "\n"
