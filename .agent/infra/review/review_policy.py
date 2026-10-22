"""Shared validation for the GitHub orchestrator and fixed SSH entrypoint."""

import json
import re
from pathlib import PurePosixPath

REPOSITORY = "benzocloud/hack-rnd-autumn-2026"
BASE_SKILLS = ["caveman-review", "ponytail-review"]
SHA = re.compile(r"[0-9a-f]{40}\Z")


def is_request(event, event_name="issue_comment", attempt="1"):
    return (
        event_name == "issue_comment"
        and attempt == "1"
        and event.get("action") == "created"
        and bool(event.get("issue", {}).get("pull_request"))
        and event.get("comment", {}).get("body") == "/agent-review"
        and event.get("comment", {}).get("user", {}).get("type") == "User"
    )


def skills_for(title, files):
    go = any(
        p.endswith(".go") or PurePosixPath(p).name in {"go.mod", "go.sum", "go.work", "go.work.sum"}
        for p in files
    )
    if title.startswith("INFRA/ML:"):
        scope, require_go = "INFRA/ML", True
    elif title.startswith("ML:"):
        scope, require_go = "ML", False
    else:
        scope, require_go = ("INFRA" if title.startswith("INFRA:") else "changed-files"), go
    return scope, BASE_SKILLS + (["golang-how-to"] if require_go else [])


def scope_conflict(context):
    return context["title"].startswith("ML:") and any(
        p.endswith(".go") or PurePosixPath(p).name in {"go.mod", "go.sum", "go.work", "go.work.sum"}
        for p in context["files"]
    )


def safe_path(value):
    p = PurePosixPath(value)
    if not value or p.is_absolute() or ".." in p.parts or "\\" in value or ".git" in p.parts:
        raise ValueError("Invalid source path")
    return p


def validate_history(context, origin_number):
    root = context["dialogue_root"]
    if root is not None and (type(root) is not int or root < 1):
        raise ValueError("Invalid Issue dialogue root")
    for key in ("issue_comments", "pr_comments", "pr_reviews", "pr_review_comments", "related_prs"):
        if not isinstance(context[key], list):
            raise ValueError("Missing complete discussion context")
    if root is None and context["related_prs"]:
        raise ValueError("Related PRs require an Issue dialogue root")
    numbers = set()
    if len(context["related_prs"]) > 10:
        raise ValueError("Too many related PRs")
    for thread in context["related_prs"]:
        number = thread["task"]["number"]
        if type(number) is not int or number < 1 or number in numbers or number == origin_number:
            raise ValueError("Invalid related PR identity")
        numbers.add(number)
        if thread["task"]["state"] not in ("open", "closed") or any(
            not SHA.fullmatch(thread[key]) for key in ("head_sha", "base_sha")
        ):
            raise ValueError("Invalid related PR source/state")
        if any(
            not isinstance(thread[key], list)
            for key in ("pr_comments", "pr_reviews", "pr_review_comments")
        ):
            raise ValueError("Missing related PR discussion")
    if len(json.dumps(context, ensure_ascii=False)) > 160000:
        raise ValueError("Discussion exceeds context limit; refusing silent truncation")


def validate_context(context):
    if context["repository"] != REPOSITORY:
        raise ValueError("Unexpected repository")
    if not all(SHA.fullmatch(context[k]) for k in ("head_sha", "base_sha")):
        raise ValueError("Invalid commit SHA")
    if not all(type(context[k]) is int and context[k] > 0 for k in ("pr_number", "comment_id")):
        raise ValueError("Invalid request identifiers")
    for name in context["files"]:
        safe_path(name)
    scope, skills = skills_for(context["title"], context["files"])
    if context["scope"] != scope or context["required_skills"] != skills:
        raise ValueError("Skill routing differs from trusted policy")
    if not isinstance(context.get("issues"), list) or not isinstance(context.get("ci"), dict):
        raise ValueError("Missing required task/CI context; issues must be a list (possibly empty)")
    validate_history(context, context["pr_number"])
    root = context["dialogue_root"]
    if [item["number"] for item in context["issues"]] != ([] if root is None else [root]):
        raise ValueError("Review Issue dialogue root mismatch")


FINDING = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "severity": {"type": "string", "enum": ["BLOCKER", "MAJOR", "MINOR"]},
        "file": {"type": "string"},
        "line": {"type": "integer"},
        "evidence": {"type": "string"},
        "impact": {"type": "string"},
        "fix": {"type": "string"},
    },
    "required": ["severity", "file", "line", "evidence", "impact", "fix"],
}
REPORT_SCHEMA = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "head_sha": {"type": "string"},
        "base_sha": {"type": "string"},
        "scope": {"type": "string"},
        "status": {"type": "string", "enum": ["complete", "incomplete"]},
        "memory": {"type": "string", "enum": ["unavailable"]},
        "skills": {"type": "array", "items": {"type": "string"}},
        "checks": {"type": "array", "items": {"type": "string"}},
        "limitations": {"type": "array", "items": {"type": "string"}},
        "findings": {"type": "array", "items": FINDING},
    },
    "required": [
        "head_sha",
        "base_sha",
        "scope",
        "status",
        "memory",
        "skills",
        "checks",
        "limitations",
        "findings",
    ],
}


def validate_report(report, context):
    validate_context(context)
    if set(report) != set(REPORT_SCHEMA["required"]):
        raise ValueError("Unexpected report shape")
    for key in ("head_sha", "base_sha", "scope"):
        if report[key] != context[key]:
            raise ValueError(f"Report {key} mismatch")
    if report["status"] not in ("complete", "incomplete") or report["memory"] != "unavailable":
        raise ValueError("Invalid report status")
    for key in ("skills", "checks", "limitations"):
        if not isinstance(report[key], list) or any(not isinstance(v, str) for v in report[key]):
            raise ValueError("Invalid report list")
    if context["scope"] == "ML" and any(s.startswith("golang-") for s in report["skills"]):
        raise ValueError("Go skills are forbidden for standalone ML:")
    if report["status"] == "complete" and scope_conflict(context):
        raise ValueError("ML: conflicts with Go changes; review must be incomplete")
    if report["status"] == "complete" and not set(context["required_skills"]).issubset(
        report["skills"]
    ):
        raise ValueError("Missing required skills")
    if not isinstance(report["findings"], list) or len(report["findings"]) > 30:
        raise ValueError("Invalid findings")
    for finding in report["findings"]:
        if set(finding) != set(FINDING["required"]):
            raise ValueError("Invalid finding shape")
        safe_path(finding["file"])
        if (
            finding["file"] not in context["files"]
            or type(finding["line"]) is not int
            or finding["line"] < 1
        ):
            raise ValueError("Finding must reference a changed file and positive line")
        if finding["severity"] not in ("BLOCKER", "MAJOR", "MINOR"):
            raise ValueError("Invalid severity")
        if any(
            not isinstance(finding[k], str) or not finding[k].strip() or len(finding[k]) > 3000
            for k in ("evidence", "impact", "fix")
        ):
            raise ValueError("Invalid finding details")
