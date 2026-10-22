#!/usr/bin/env python3
"""Trusted comment-only publisher; no Git writes or credentials in model jobs."""

import base64
import json
import os
import re
import sys
import urllib.parse
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "runner"))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "planning"))
from conversation_policy import command, validate_context, validate_result
from github_api import REPOSITORY, GitHub, event, invoke_json, pull_request_discussion
from github_plan import default_source, output
from plan_policy import fingerprint, identity, source_links


def selected(api, payload):
    parsed = command(
        payload,
        os.environ.get("GITHUB_EVENT_NAME", "issue_comment"),
        os.environ.get("GITHUB_RUN_ATTEMPT", "1"),
    )
    if parsed is None:
        return None
    actor = urllib.parse.quote(payload["comment"]["user"]["login"], safe="")
    if api.request(f"/collaborators/{actor}/permission").get("permission") not in (
        "admin",
        "maintain",
        "write",
    ):
        return None
    current = api.request(f"/issues/comments/{int(payload['comment']['id'])}")
    if (
        current["body"] != payload["comment"]["body"]
        or current["user"]["login"] != payload["comment"]["user"]["login"]
    ):
        raise ValueError("Command comment changed")
    number = int(payload["issue"]["number"])
    task = api.request(f"/issues/{number}")
    pr = api.request(f"/pulls/{number}") if "pull_request" in task else None
    if task["state"] != "open" or (
        pr and (pr["state"] != "open" or pr["head"].get("repo", {}).get("full_name") != REPOSITORY)
    ):
        raise ValueError("Expected an open Issue or same-repository PR")
    # Re-evaluate routing against the current body, not an obsolete webhook marker.
    fresh = {**payload, "issue": task}
    if command(fresh, "issue_comment") != parsed:
        raise ValueError("Conversation routing changed")
    return parsed, task, pr


def authorize():
    output(requested=str(selected(GitHub(), event()) is not None).lower())


def marker(payload):
    return f"<!-- agent-conversation-request:{int(payload['comment']['id'])} -->"


def status_comment(api, payload):
    comments = [
        c
        for c in api.pages(f"/issues/{int(payload['issue']['number'])}/comments")
        if c.get("user", {}).get("login") == "github-actions[bot]"
        and (c.get("body") or "").startswith(marker(payload) + "\n")
    ]
    if len(comments) > 1:
        raise ValueError("Duplicate conversation status comments")
    return comments[0] if comments else None


def run_link():
    return f"[Запуск GitHub Actions](https://github.com/{REPOSITORY}/actions/runs/{int(os.environ['GITHUB_RUN_ID'])})"


def update_status(api, payload, body):
    comment = status_comment(api, payload)
    if comment is None:
        raise ValueError("Missing conversation status; refusing a duplicate reply")
    api.request(
        f"/issues/comments/{int(comment['id'])}",
        {"body": marker(payload) + "\n\n" + body + "\n\n" + run_link()},
        method="PATCH",
    )


def start():
    api, payload = GitHub(), event()
    if selected(api, payload) is None:
        raise ValueError("Conversation request is no longer authorized")
    if status_comment(api, payload) is None:
        api.request(
            f"/issues/{int(payload['issue']['number'])}/comments",
            {
                "body": marker(payload)
                + "\n\n⏳ **Запрос принят** — собираю обсуждение и контекст исходников.\n\n"
                + run_link()
            },
        )


def discussion(api, number, pr):
    return (
        pull_request_discussion(api, number)
        if pr
        else {
            "issue_comments": api.pages(f"/issues/{number}/comments"),
            "pr_comments": [],
            "pr_reviews": [],
            "pr_review_comments": [],
        }
    )


def linked_issues(api, task, comments):
    text = "\n".join(
        [
            task.get("body") or "",
            *(c.get("body") or "" for items in comments.values() for c in items),
        ]
    )
    pattern = (
        r"(?<![\w/#])#([1-9][0-9]*)\b"
        r"|https://github\.com/" + re.escape(REPOSITORY) + r"/issues/([1-9][0-9]*)"
        r"|<!-- agent-plan:issue=([1-9][0-9]*) -->"
    )
    numbers = {int(next(n for n in groups if n)) for groups in re.findall(pattern, text, re.I)}
    numbers.discard(task["number"])
    if len(numbers) > 10:
        raise ValueError("Too many linked references")
    result = []
    for number in sorted(numbers):
        issue = api.request(f"/issues/{number}")
        if "pull_request" in issue:
            # Issues and PRs share GitHub's number space; a PR mention is not a task.
            continue
        result.append({"task": issue, "comments": api.pages(f"/issues/{number}/comments")})
    return result


def collect(api, payload, request):
    (mode, question), task, pr = request
    branch, sha = default_source(api)
    files, plan = [], []
    if pr:
        sha = pr["head"]["sha"]
        changed = api.pages(f"/pulls/{task['number']}/files")
        if len(changed) != pr["changed_files"]:
            raise ValueError("Incomplete changed-file list")
        files = [item["filename"] for item in changed]
        match = re.search(r"<!-- agent-plan:issue=([1-9][0-9]*) -->", task.get("body") or "")
        if match:
            plan = [identity(int(match[1]))[1]]
    comments = {"issue_comments": [], **discussion(api, task["number"], pr)}
    context = {
        "repository": REPOSITORY,
        "mode": mode,
        "question": question,
        "thread_number": task["number"],
        "comment_id": payload["comment"]["id"],
        "request_body": payload["comment"]["body"],
        "task": task,
        "task_fingerprint": fingerprint(task),
        "title": task["title"],
        "source_sha": sha,
        "base_sha": pr["base"]["sha"] if pr else sha,
        "base_branch": branch,
        "files": files,
        "existing_plan": plan,
        "issues": linked_issues(api, task, comments),
        **comments,
    }
    validate_context(context)
    return context


def prepare():
    api, payload = GitHub(), event()
    output(requested="false")
    request = selected(api, payload)
    if request is None:
        return
    context = collect(api, payload, request)
    policies = {
        name: Path(name).read_text()
        for name in (
            "AGENTS.md",
            ".agent/instructions.md",
            ".agent/decisions.md",
            ".agent/workflows/issue-planning.md",
            ".agent/workflows/context-and-memory.md",
            ".agent/workflows/git-conventions.md",
            ".agent/references/go-services.md",
        )
    }
    archive = api.request(f"/tarball/{context['source_sha']}", raw=True)
    Path("request.json").write_text(
        json.dumps(
            {
                "context": context,
                "policies": policies,
                "archive": base64.b64encode(archive).decode(),
            }
        )
    )
    Path("context.json").write_text(json.dumps(context))
    output(requested="true")


def invoke():
    context = json.loads(Path("context.json").read_text())
    validate_context(context)
    result = invoke_json("agent-" + context["mode"])
    validate_result(result, context)
    Path("conversation-result.json").write_text(json.dumps({"context": context, "result": result}))


def substantive_item(item):
    """Keep text, chronology and review locations, excluding API display metadata."""
    fields = (
        "id",
        "number",
        "title",
        "body",
        "state",
        "created_at",
        "submitted_at",
        "author_association",
        "path",
        "line",
        "start_line",
        "side",
        "start_side",
        "original_line",
        "original_start_line",
        "position",
        "original_position",
        "commit_id",
        "original_commit_id",
        "in_reply_to_id",
        "pull_request_review_id",
        "diff_hunk",
        "subject_type",
    )
    return {
        **{key: item[key] for key in fields if key in item},
        "user": {key: item.get("user", {}).get(key) for key in ("id", "login", "type")},
    }


def substantive_context(context):
    result = {**context, "task": substantive_item(context["task"])}
    for key in ("issue_comments", "pr_comments", "pr_reviews", "pr_review_comments"):
        result[key] = [substantive_item(item) for item in context[key]]
    result["issues"] = [
        {
            "task": substantive_item(item["task"]),
            "comments": [substantive_item(c) for c in item["comments"]],
        }
        for item in context["issues"]
    ]
    return result


def publish_result(api, payload):
    package = json.loads(Path("conversation-result.json").read_text())
    context, result = package["context"], package["result"]
    validate_result(result, context)
    request = selected(api, payload)
    if request is None:
        raise ValueError("Conversation no longer authorized")
    current = collect(api, payload, request)
    if substantive_context(current) != substantive_context(context):
        raise ValueError("Source, task or discussion changed; request a fresh answer")
    report = result["report"]
    usage = result["usage"]
    tokens = (
        f"входные {usage['input_tokens']}, из кеша {usage['cached_input_tokens']}, выходные {usage['output_tokens']}"
        if usage
        else "CLI не предоставил данные"
    )
    sources = (
        "\n\nПереданные исходники: " + ", ".join(f"`{p}`" for p in result["source_files"])
        if context["mode"] == "question"
        else ""
    )
    if context["mode"] == "question":
        omitted = [
            path
            for path in context["files"]
            if path not in result["source_files"] and path not in result["skipped_source_files"]
        ]
        if omitted:
            sources += "\n\nНе переданы изменённые файлы: " + ", ".join(f"`{p}`" for p in omitted)
            sources += ". Для их изучения используйте `/discuss` или явно укажите нужные пути."
    if result["skipped_source_files"]:
        sources += "\n\nНе прочитаны (бинарные или не UTF-8): " + ", ".join(
            f"`{p}`" for p in result["skipped_source_files"]
        )
    update_status(
        api,
        payload,
        "✅ **Ответ готов**\n\n"
        + source_links(report["answer"], context["source_sha"])
        + sources
        + f"\n\nМодель: `{result['model']}`, глубина рассуждений: низкая. Токены: {tokens}."
        + "\nИспользуется общий лимит Codex / ChatGPT Work подписки Plus. GBrain недоступен."
        + f"\nИсточник: `{context['source_sha']}`.",
    )


def publish():
    api, payload = GitHub(), event()
    job_result = os.environ.get("CONVERSATION_JOB_RESULT", "success")
    if job_result != "success" or os.environ.get("CONVERSATION_REQUESTED") == "false":
        label = "отменён" if job_result == "cancelled" else "завершился ошибкой"
        update_status(
            api, payload, f"⚠️ **Запрос {label}**. Ответ не получен; подробности в Actions."
        )
        return
    try:
        publish_result(api, payload)
    except Exception:
        update_status(
            api,
            payload,
            "⚠️ **Ответ не опубликован** — контекст изменился, ответ не прошёл проверку или публикация завершилась ошибкой. Отправьте новую команду; подробности в Actions.",
        )
        raise


if __name__ == "__main__":
    {
        "authorize": authorize,
        "start": start,
        "prepare": prepare,
        "invoke": invoke,
        "publish": publish,
    }[sys.argv[1]]()
