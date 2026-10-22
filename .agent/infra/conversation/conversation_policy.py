"""Read-only question/discussion protocol; plan writes use the planning publisher."""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "planning"))
from plan_policy import command as planning_command
from plan_policy import discussion_command, fingerprint
from review_policy import REPOSITORY, SHA, safe_path, validate_history

MODELS = {"question": "gpt-6-luna", "discuss": "gpt-6.1-sol"}
SCHEMA = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "source_sha": {"type": "string"},
        "thread_number": {"type": "integer"},
        "comment_id": {"type": "integer"},
        "mode": {"type": "string", "enum": list(MODELS)},
        "status": {"type": "string", "enum": ["complete", "needs-clarification"]},
        "answer": {"type": "string"},
    },
    "required": ["source_sha", "thread_number", "comment_id", "mode", "status", "answer"],
}


def command(payload, event_name, attempt="1"):
    parsed = discussion_command(payload, event_name, attempt)
    if not parsed or parsed[0] not in MODELS or planning_command(payload, event_name, attempt):
        return None
    return parsed


def validate_context(context):
    if (
        context["repository"] != REPOSITORY
        or context["mode"] not in MODELS
        or not SHA.fullmatch(context["source_sha"])
        or not SHA.fullmatch(context["base_sha"])
    ):
        raise ValueError("Invalid conversation context")
    for key in ("thread_number", "comment_id"):
        if type(context[key]) is not int or context[key] < 1:
            raise ValueError("Invalid conversation identity")
    if context["task_fingerprint"] != fingerprint(context["task"]):
        raise ValueError("Conversation task mismatch")
    if context["task"]["state"] != "open" or context["task"]["number"] != context["thread_number"]:
        raise ValueError("Expected an open conversation task")
    if not isinstance(context["question"], str) or not context["question"].strip():
        raise ValueError("Empty conversation request")
    for key in ("issues", "files"):
        if not isinstance(context[key], list):
            raise ValueError("Missing complete conversation context")
    validate_history(context, context["thread_number"])
    root = context["dialogue_root"]
    if root is not None and "pull_request" in context["task"]:
        roots = [item["task"] for item in context["issues"] if item["task"]["number"] == root]
        if len(roots) != 1 or "pull_request" in roots[0]:
            raise ValueError("Missing canonical Issue dialogue")
    elif root != context["thread_number"] and "pull_request" not in context["task"]:
        raise ValueError("Issue dialogue must use its own root")
    for path in [*context["files"], *context["existing_plan"]]:
        safe_path(path)


def validate_report(report, context):
    validate_context(context)
    if not isinstance(report, dict) or set(report) != set(SCHEMA["required"]):
        raise ValueError("Invalid conversation report schema")
    if any(
        report[key] != context[key] for key in ("source_sha", "thread_number", "comment_id", "mode")
    ):
        raise ValueError("Conversation report belongs to another request/source")
    if report["status"] not in ("complete", "needs-clarification"):
        raise ValueError("Invalid conversation status")
    if not isinstance(report["answer"], str) or not 1 <= len(report["answer"].strip()) <= 16000:
        raise ValueError("Invalid conversation answer")


def validate_result(result, context):
    if not isinstance(result, dict) or set(result) != {
        "report",
        "model",
        "reasoning",
        "usage",
        "source_files",
        "skipped_source_files",
    }:
        raise ValueError("Invalid worker result")
    validate_report(result["report"], context)
    if result["model"] != MODELS[context["mode"]] or result["reasoning"] != "low":
        raise ValueError("Worker used an unexpected model/effort")
    for key in ("source_files", "skipped_source_files"):
        if not isinstance(result[key], list) or any(not isinstance(p, str) for p in result[key]):
            raise ValueError("Invalid source receipts")
        for path in result[key]:
            safe_path(path)
    if result["usage"] is not None and (
        not isinstance(result["usage"], dict)
        or set(result["usage"]) != {"input_tokens", "cached_input_tokens", "output_tokens"}
        or any(type(v) is not int or v < 0 for v in result["usage"].values())
    ):
        raise ValueError("Invalid measured usage")
