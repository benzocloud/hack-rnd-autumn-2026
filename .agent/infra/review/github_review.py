#!/usr/bin/env python3
"""Trusted GitHub-hosted orchestration; never execute scripts from the PR."""

import base64
import json
import os
import re
import sys
import urllib.parse
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "runner"))
from github_api import GitHub, event, invoke_json, pull_request_discussion
from github_dialogue import (
    dialogue_root,
    related_pull_requests,
    substantive_history,
    substantive_item,
)
from review_policy import (
    REPOSITORY,
    is_request,
    scope_conflict,
    skills_for,
    validate_context,
    validate_report,
)


def select_request(api, payload):
    if not is_request(
        payload,
        os.environ.get("GITHUB_EVENT_NAME", "issue_comment"),
        os.environ.get("GITHUB_RUN_ATTEMPT", "1"),
    ):
        return None
    actor = payload["comment"]["user"]["login"]
    permission = api.request("/collaborators/" + urllib.parse.quote(actor, safe="") + "/permission")
    if permission.get("permission") not in ("admin", "maintain", "write"):
        print("Ignored /agent-review: requester lacks repository write permission")
        return None
    pr = api.request(f"/pulls/{int(payload['issue']['number'])}")
    if pr["state"] != "open" or pr["head"].get("repo", {}).get("full_name") != REPOSITORY:
        raise ValueError("Only open, same-repository PRs are enabled in this pilot")
    return pr


def discussion_context(api, pr):
    root = dialogue_root(pr, pr)
    issues = []
    if root is not None:
        issue = api.request(f"/issues/{root}")
        if "pull_request" in issue:
            raise ValueError("A dialogue root must be an Issue, not another PR")
        issues.append(
            {
                **substantive_item(issue),
                "comments": api.pages(f"/issues/{root}/comments"),
            }
        )
    return {
        "dialogue_root": root,
        "issue_comments": [],
        "issues": issues,
        "related_prs": related_pull_requests(api, root, pr["number"]) if root is not None else [],
        **pull_request_discussion(api, pr["number"]),
    }


def review_history(context):
    return {
        **substantive_history(context),
        "issues": [
            {
                **substantive_item(issue),
                "comments": [substantive_item(c) for c in issue["comments"]],
            }
            for issue in context["issues"]
        ],
    }


def prepare():
    api, payload = GitHub(), event()
    pr = select_request(api, payload)
    output = Path(os.environ["GITHUB_OUTPUT"])
    with output.open("a") as out:
        out.write("requested=false\n")
    if pr is None:
        return
    number, head, base = pr["number"], pr["head"]["sha"], pr["base"]["sha"]
    files = api.pages(f"/pulls/{number}/files")
    if len(files) != pr["changed_files"]:
        raise ValueError("Changed-file list is incomplete")
    names = [item["filename"] for item in files]
    scope, skills = skills_for(pr["title"], names)
    context = {
        "repository": REPOSITORY,
        "pr_number": number,
        "comment_id": payload["comment"]["id"],
        "head_sha": head,
        "base_sha": base,
        "title": pr["title"],
        "body": pr.get("body"),
        "scope": scope,
        "required_skills": skills,
        "files": names,
        **discussion_context(api, pr),
        "ci": {
            "checks": api.pages(f"/commits/{head}/check-runs", "check_runs"),
            "status": api.request(f"/commits/{head}/status"),
        },
    }
    validate_context(context)
    diff = api.request(f"/pulls/{number}", raw=True, accept="application/vnd.github.diff").decode()
    if len(diff.encode()) > 2 * 1024 * 1024:
        raise ValueError("Diff too large for this pilot")
    archive = api.request(f"/tarball/{head}", raw=True)
    current = api.request(f"/pulls/{number}")
    if (
        current["head"]["sha"] != head
        or current["base"]["sha"] != base
        or current["title"] != pr["title"]
    ):
        raise ValueError("PR changed while context was collected; request a fresh review")
    trusted = {
        name: Path(name).read_text()
        for name in (
            "AGENTS.md",
            ".agent/instructions.md",
            ".agent/decisions.md",
            ".agent/workflows/automated-review.md",
            ".agent/workflows/context-and-memory.md",
            ".agent/prompts/pr-review.md",
            ".agent/workflows/git-conventions.md",
            ".agent/references/go-services.md",
        )
    }
    package = {
        "context": context,
        "archive": base64.b64encode(archive).decode(),
        "diff": diff,
        "policies": trusted,
    }
    Path("request.json").write_text(json.dumps(package))
    Path("context.json").write_text(json.dumps(context))
    with output.open("a") as out:
        out.write("requested=true\n")


def authorize():
    # This job has no concurrency group: unauthorized comments cannot cancel a review.
    allowed = select_request(GitHub(), event()) is not None
    with Path(os.environ["GITHUB_OUTPUT"]).open("a") as out:
        out.write("requested=" + str(allowed).lower() + "\n")


def status_marker(payload):
    return f"<!-- agent-review-request:{int(payload['comment']['id'])} -->"


def run_url():
    return f"https://github.com/{REPOSITORY}/actions/runs/{int(os.environ['GITHUB_RUN_ID'])}"


def status_comment(api, payload):
    marker = status_marker(payload)
    matches = [
        item
        for item in api.pages(f"/issues/{int(payload['issue']['number'])}/comments")
        if item.get("user", {}).get("login") == "github-actions[bot]"
        and marker in (item.get("body") or "")
    ]
    if len(matches) > 1:
        raise ValueError("Duplicate status comments for one review request")
    return matches[0] if matches else None


def update_status(api, payload, body):
    comment = status_comment(api, payload)
    if comment is None:
        raise ValueError("Review status comment is missing; refusing a duplicate result")
    api.request(
        f"/issues/comments/{int(comment['id'])}",
        {
            "body": status_marker(payload)
            + "\n\n"
            + body
            + f"\n\n[Запуск GitHub Actions]({run_url()})"
        },
        method="PATCH",
    )


def start():
    api, payload = GitHub(), event()
    pr = select_request(api, payload)
    if pr is None:
        raise ValueError("Review request is no longer authorized")
    if status_comment(api, payload) is None:
        api.request(
            f"/issues/{pr['number']}/comments",
            {
                "body": status_marker(payload)
                + "\n\n⏳ **Начал ревью** — собираю контекст и готовлю статический анализ."
                + f"\n\n[Запуск GitHub Actions]({run_url()})"
            },
        )


def invoke():
    context = json.loads(Path("context.json").read_text())
    if scope_conflict(context):
        report = {
            "head_sha": context["head_sha"],
            "base_sha": context["base_sha"],
            "scope": context["scope"],
            "status": "incomplete",
            "memory": "unavailable",
            "skills": [],
            "checks": ["Проверен префикс заголовка и состав изменений; модель не запускалась"],
            "limitations": [
                "Префикс ML: противоречит изменениям Go. Исправьте заголовок или разделите PR, затем отправьте новый комментарий /agent-review."
            ],
            "findings": [],
        }
        validate_report(report, context)
        Path("report.json").write_text(
            json.dumps({"context": context, "report": report, "loaded_skills": []})
        )
        Path("request.json").unlink(missing_ok=True)
        return
    result = invoke_json("agent-review")
    validate_report(result["report"], context)
    receipts = result["loaded_skills"]
    if [v["name"] for v in receipts] != context["required_skills"] or any(
        not re.fullmatch(r"[0-9a-f]{64}", v["sha256"]) for v in receipts
    ):
        raise ValueError("Missing pinned skill loading receipts")
    Path("report.json").write_text(json.dumps({"context": context, **result}))


def publish():
    api, payload = GitHub(), event()
    job_result = os.environ.get("REVIEW_JOB_RESULT", "success")
    if job_result != "success" or os.environ.get("REVIEW_REQUESTED") == "false":
        if not is_request(payload):
            raise ValueError("Invalid review event")
        label = "отменено" if job_result == "cancelled" else "завершилось ошибкой"
        update_status(
            api,
            payload,
            f"⚠️ **Ревью {label}** — итоговый отчёт не получен. Подробности в GitHub Actions.",
        )
        return
    package = json.loads(Path("report.json").read_text())
    context, report = package["context"], package["report"]
    validate_report(report, context)
    if (
        context["pr_number"] != payload["issue"]["number"]
        or context["comment_id"] != payload["comment"]["id"]
    ):
        raise ValueError("Report does not belong to this comment request")
    try:
        pr = select_request(api, payload)
    except ValueError:
        pr = None
    if (
        pr is None
        or pr["head"]["sha"] != context["head_sha"]
        or pr["base"]["sha"] != context["base_sha"]
        or pr["title"] != context["title"]
    ):
        update_status(
            api,
            payload,
            "⚠️ **Результат ревью устарел** — PR изменился во время анализа. Отправьте новый комментарий `/agent-review`.",
        )
        return
    for comment in reversed(api.pages(f"/issues/{pr['number']}/comments")):
        if (
            comment["id"] > context["comment_id"]
            and comment.get("body") == "/agent-review"
            and comment.get("user", {}).get("type") == "User"
        ):
            login = urllib.parse.quote(comment["user"]["login"], safe="")
            if api.request("/collaborators/" + login + "/permission").get("permission") in (
                "admin",
                "maintain",
                "write",
            ):
                update_status(
                    api,
                    payload,
                    "⏭️ **Ревью заменено** более новым запросом от пользователя с правами запуска.",
                )
                return
    try:
        current_discussion = discussion_context(api, pr)
        validate_context({**context, **current_discussion})
    except Exception:
        update_status(
            api,
            payload,
            "⚠️ **Результат ревью не опубликован** — не удалось проверить полный контекст Issue/PR. Подробности в Actions.",
        )
        raise
    if pr.get("body") != context.get("body") or review_history(
        current_discussion
    ) != review_history(context):
        update_status(
            api,
            payload,
            "⚠️ **Результат ревью устарел** — описание или история Issue/PR изменились во время анализа. Отправьте новый комментарий `/agent-review`.",
        )
        return
    body = [
        "Ревью агента: **"
        + ("завершено" if report["status"] == "complete" else "не завершено")
        + "**",
        f"Коммит: `{context['head_sha']}`",
        f"Область изменений: `{report['scope']}`",
        "GBrain недоступен (развёртывание отложено).",
        "Навыки, указанные в отчёте: " + (", ".join(report["skills"]) or "нет"),
    ]
    if context["issues"]:
        body.append(
            "Связанные Issue: " + ", ".join(f"#{item['number']}" for item in context["issues"])
        )
        if context["related_prs"]:
            body.append(
                "Учтены связанные PR: "
                + ", ".join(
                    f"[#{p['task']['number']}](https://github.com/{REPOSITORY}/pull/{p['task']['number']})"
                    for p in context["related_prs"]
                )
            )
    else:
        body.append(
            "Связанный Issue не указан: ревью выполнено по описанию и обсуждению PR, изменениям, исходникам и CI."
        )
        if not (context.get("body") or "").strip():
            body.append(
                "Описание PR пустое; полноту проверки требований и критериев приёмки подтвердить нельзя."
            )
    ci = context["ci"]
    body.append(
        "Данные CI GitHub: общий статус `"
        + str(ci.get("status", {}).get("state", "unknown"))
        + "`; запусков проверок: "
        + str(len(ci.get("checks", [])))
        + ". Агент выполнил статическое ревью без локального запуска тестов."
    )
    for item in report["findings"]:
        link = f"https://github.com/{REPOSITORY}/blob/{context['head_sha']}/{urllib.parse.quote(item['file'])}#L{item['line']}"
        severity = {"BLOCKER": "Блокирующее", "MAJOR": "Существенное", "MINOR": "Незначительное"}[
            item["severity"]
        ]
        body += [
            f"\n**{severity}** — [{item['file']}:{item['line']}]({link})",
            item["evidence"],
            "Последствия: " + item["impact"],
            "Исправление: " + item["fix"],
        ]
    if not report["findings"]:
        body.append(
            "Замечаний к исправлению нет. Это не апрув и не подтверждение успешного прохождения тестов."
        )
    body += ["\nВыполненные проверки:"] + ["- " + v for v in report["checks"]]
    body += ["\nОграничения:"] + ["- " + v for v in report["limitations"]]
    rendered = "\n\n".join(body)
    if len(rendered) > 60000:
        raise ValueError("Review too large to publish safely")
    # Edit the original progress comment; never approve, merge or add a duplicate review.
    update_status(api, payload, rendered)


if __name__ == "__main__":
    {
        "authorize": authorize,
        "start": start,
        "prepare": prepare,
        "invoke": invoke,
        "publish": publish,
    }[sys.argv[1]]()
