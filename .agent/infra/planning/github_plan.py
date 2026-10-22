#!/usr/bin/env python3
"""Trusted Issue planning orchestration. Model output never selects write paths."""

import base64
import json
import os
import re
import sys
import urllib.error
import urllib.parse
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "runner"))
from github_api import REPOSITORY, GitHub, event, invoke_json, pull_request_discussion
from github_dialogue import (
    dialogue_root,
    related_pull_requests,
    substantive_history,
    substantive_item,
)
from plan_policy import (
    PLAN_FILE_LIMIT,
    command,
    fingerprint,
    identity,
    is_plan_branch,
    render,
    validate_context,
    validate_response,
)

MARKER = re.compile(r"<!-- agent-plan:issue=(\d+) -->")


def selected(api, payload):
    mode = command(
        payload,
        os.environ.get("GITHUB_EVENT_NAME", "issue_comment"),
        os.environ.get("GITHUB_RUN_ATTEMPT", "1"),
    )
    if not mode:
        return None
    if mode != "scope":
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
    pr = None
    if mode == "discuss-plan":
        pr = api.request(f"/pulls/{number}")
        match = MARKER.search(pr.get("body") or "")
        if not match:
            raise ValueError("/discuss requires a managed plan PR")
        number = int(match[1])
        if dialogue_root(pr, pr) != number:
            raise ValueError("Plan PR does not identify its canonical Issue")
        _, path = identity(number)
        if (
            pr["state"] != "open"
            or pr["head"].get("repo", {}).get("full_name") != REPOSITORY
            or not is_plan_branch(pr["head"]["ref"], number)
        ):
            raise ValueError("Only open same-repository managed plan PRs are accepted")
        files = api.pages(f"/pulls/{pr['number']}/files")
        if len(files) != 1 or files[0]["filename"] != path or pr["changed_files"] != 1:
            raise ValueError("Plan PR contains implementation changes; edit it manually")
    issue = api.request(f"/issues/{number}")
    if "pull_request" in issue or issue["state"] not in (
        ("open", "closed") if mode == "discuss-plan" else ("open",)
    ):
        raise ValueError("Expected an open Issue, or historical Issue in an open plan PR")
    return mode, issue, pr


def output(**values):
    with Path(os.environ["GITHUB_OUTPUT"]).open("a") as out:
        for key, value in values.items():
            out.write(f"{key}={value}\n")


def authorize():
    request = selected(GitHub(), event())
    output(requested=str(request is not None).lower())


def scope_marker(payload):
    return f"<!-- agent-scope-request:{int(payload['issue']['id'])} -->"


def scope_status(api, payload, body=None):
    marker = scope_marker(payload)
    comments = [
        item
        for item in api.pages(f"/issues/{int(payload['issue']['number'])}/comments")
        if item.get("user", {}).get("login") == "github-actions[bot]"
        and (item.get("body") or "").startswith(marker + "\n")
    ]
    if len(comments) > 1:
        raise ValueError("Duplicate Issue scope status comments")
    if body is None:
        return comments[0] if comments else None
    if not comments:
        raise ValueError("Issue scope status comment is missing; refusing a duplicate result")
    api.request(
        f"/issues/comments/{int(comments[0]['id'])}",
        {"body": marker + "\n\n" + body + "\n\n" + scope_run_link()},
        method="PATCH",
    )


def scope_run_link():
    return f"[Запуск GitHub Actions](https://github.com/{REPOSITORY}/actions/runs/{int(os.environ['GITHUB_RUN_ID'])})"


def start():
    api, payload = GitHub(), event()
    request = selected(api, payload)
    if request is None or request[0] not in ("scope", "discuss-plan"):
        raise ValueError("Expected an authorized scope or plan discussion request")
    if request[0] == "discuss-plan":
        plan_status(
            api,
            payload,
            "⏳ **Начал обновление плана** — изучаю запрос и обсуждение PR.",
            create=True,
        )
        return
    if scope_status(api, payload) is None:
        api.request(
            f"/issues/{request[1]['number']}/comments",
            {
                "body": scope_marker(payload)
                + "\n\n⏳ **Начал оценку границ исправления** — собираю контекст Issue и изучаю исходники.\n\n"
                + scope_run_link()
            },
        )


def plan_status(api, payload, body, create=False):
    marker = f"<!-- agent-plan-discussion:{int(payload['comment']['id'])} -->"
    comments = [
        c
        for c in api.pages(f"/issues/{int(payload['issue']['number'])}/comments")
        if c.get("user", {}).get("login") == "github-actions[bot]"
        and (c.get("body") or "").startswith(marker + "\n")
    ]
    if len(comments) > 1:
        raise ValueError("Duplicate plan discussion status comments")
    rendered = {"body": marker + "\n\n" + body + "\n\n" + scope_run_link()}
    if comments:
        if not create:
            api.request(f"/issues/comments/{int(comments[0]['id'])}", rendered, method="PATCH")
    elif create:
        api.request(f"/issues/{int(payload['issue']['number'])}/comments", rendered)
    else:
        raise ValueError("Missing plan discussion status; refusing a duplicate result")


def default_source(api):
    branch = api.request("")["default_branch"]
    sha = api.request("/commits/" + urllib.parse.quote(branch, safe=""))["sha"]
    return branch, sha


def managed_prs(api, number):
    identity(number)
    candidates = set()
    # Plan PRs include Refs #N; the Issue timeline retains closed PR references.
    for item in api.pages(f"/issues/{number}/timeline"):
        if item.get("event") != "cross-referenced":
            continue
        source = item.get("source", {}).get("issue", {})
        candidate = source.get("number")
        if (
            type(candidate) is int
            and candidate > 0
            and "pull_request" in source
            and source.get("url") == f"https://api.github.com/repos/{REPOSITORY}/issues/{candidate}"
        ):
            candidates.add(candidate)
    plans = []
    for candidate in sorted(candidates):
        pr = api.request(f"/pulls/{candidate}")
        if (
            f"<!-- agent-plan:issue={number} -->" in (pr.get("body") or "")
            and pr["head"].get("repo", {}).get("full_name") == REPOSITORY
            and is_plan_branch(pr["head"]["ref"], number)
        ):
            plans.append(pr)
    return plans


def existing_plan(api, pr, path):
    content = api.request(f"/contents/{path}?ref={pr['head']['sha']}")
    if content.get("encoding") != "base64" or content.get("size", 0) > PLAN_FILE_LIMIT:
        raise ValueError("Unsupported plan file")
    markdown = base64.b64decode(content["content"], validate=False).decode()
    if len(markdown.encode()) > PLAN_FILE_LIMIT:
        raise ValueError("Unsupported plan file")
    return markdown


def discussion_context(api, issue, pr):
    return {
        "dialogue_root": issue["number"],
        "issue_comments": api.pages(f"/issues/{issue['number']}/comments"),
        "related_prs": related_pull_requests(
            api, issue["number"], pr["number"] if pr else issue["number"]
        ),
        "pr_state": pr["state"] if pr else None,
        "pr_body": pr.get("body") if pr else None,
        **(
            pull_request_discussion(api, pr["number"])
            if pr
            else {"pr_comments": [], "pr_reviews": [], "pr_review_comments": []}
        ),
    }


def planning_history(context):
    return {
        **substantive_history(context),
        "issue": substantive_item(context["issue"]),
        "pr_state": context["pr_state"],
        "pr_body": context["pr_body"],
    }


def prepare():
    api, payload = GitHub(), event()
    request = selected(api, payload)
    output(requested="false")
    if request is None:
        return
    mode, issue, pr = request
    _, path = identity(issue["number"])
    base_branch, sha = default_source(api)
    existing = None
    if mode == "create-plan" and managed_prs(api, issue["number"]):
        # Do not spend another model run or create duplicate branches/PRs.
        print("A plan PR already exists for this Issue; use /discuss there.")
        return
    if pr:
        sha = pr["head"]["sha"]
        existing = existing_plan(api, pr, path)
    context = {
        "repository": REPOSITORY,
        "mode": mode,
        "issue_number": issue["number"],
        "issue": issue,
        "issue_fingerprint": fingerprint(issue),
        "source_sha": sha,
        "base_branch": base_branch,
        "branch": pr["head"]["ref"] if pr else None,
        "plan_path": path,
        "request_id": payload.get("comment", {}).get("id", issue["id"]),
        "request_body": payload.get("comment", {}).get("body", ""),
        "open_prs": [
            {
                "number": p["number"],
                "title": p["title"],
                "body": p.get("body"),
                "head": p["head"]["ref"],
            }
            for p in api.pages("/pulls?state=open")
        ],
        "pr_number": pr["number"] if pr else None,
        "existing_plan": existing,
        **discussion_context(api, issue, pr),
    }
    validate_context(context)
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
    archive = api.request(f"/tarball/{sha}", raw=True)
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
    report = invoke_json("agent-" + context["mode"])
    validate_response(report, context)
    Path("plan-result.json").write_text(json.dumps({"context": context, "report": report}))


def write_plan_commit(api, parent, path, markdown, message):
    # Fixed caller-derived path, never a path supplied by the model.
    if not re.fullmatch(r"\.agent/plans/issue-[1-9][0-9]*-fix\.md", path):
        raise ValueError("Refusing a source implementation write")
    commit = api.request(f"/git/commits/{parent}")
    tree = api.request(
        "/git/trees",
        {
            "base_tree": commit["tree"]["sha"],
            "tree": [{"path": path, "mode": "100644", "type": "blob", "content": markdown}],
        },
    )
    return api.request(
        "/git/commits",
        {
            "message": message,
            "tree": tree["sha"],
            "parents": [parent],
        },
    )["sha"]


def publish():
    api, payload = GitHub(), event()
    mode = command(
        payload,
        os.environ.get("GITHUB_EVENT_NAME", "issue_comment"),
        os.environ.get("GITHUB_RUN_ATTEMPT", "1"),
    )
    scope = mode == "scope"
    result = os.environ.get("PLAN_JOB_RESULT", "success")
    if result != "success" or os.environ.get("PLAN_REQUESTED") == "false":
        if not scope and mode != "discuss-plan":
            raise ValueError("No completed plan was produced")
        status = "отменён" if result == "cancelled" else "завершился ошибкой"
        if mode == "discuss-plan":
            plan_status(
                api, payload, f"⚠️ **Запрос обновления плана {status}**. Подробности в Actions."
            )
            return
        scope_status(
            api,
            payload,
            f"⚠️ **Анализ Issue {status}**. Итоговая оценка не опубликована; подробности в Actions.",
        )
        return
    try:
        publish_result(api, payload)
    except Exception:
        if scope:
            scope_status(
                api,
                payload,
                "⚠️ **Оценка Issue не опубликована** — ответ runner не прошёл проверку, контекст изменился или публикация завершилась ошибкой. Подробности в Actions.",
            )
        elif mode == "discuss-plan":
            plan_status(
                api,
                payload,
                "⚠️ **Обновление плана не подтверждено** — контекст изменился, ответ не прошёл проверку или публикация завершилась ошибкой. Проверьте ветку и Actions перед повторным запросом.",
            )
        raise


def publish_result(api, payload):
    package = json.loads(Path("plan-result.json").read_text())
    context, report = package["context"], package["report"]
    validate_response(report, context)
    request = selected(api, payload)
    if request is None:
        raise ValueError("Request no longer authorized")
    mode, issue, pr = request
    request_id = payload.get("comment", {}).get("id", issue["id"])
    if (
        mode != context["mode"]
        or issue["number"] != context["issue_number"]
        or request_id != context["request_id"]
        or fingerprint(issue) != context["issue_fingerprint"]
        or payload.get("comment", {}).get("body", "") != context["request_body"]
    ):
        raise ValueError("Issue/request changed; discard stale plan")
    branch, path = identity(issue["number"], report["branch_description"])
    if (pr and pr["head"]["ref"] != context["branch"]) or path != context["plan_path"]:
        raise ValueError("Plan destination differs from trusted policy")
    if pr:
        branch = context["branch"]
    base_branch, current_sha = default_source(api)
    if (pr["head"]["sha"] if pr else current_sha) != context[
        "source_sha"
    ] or base_branch != context["base_branch"]:
        raise ValueError("Source changed; request a fresh plan")
    if pr and existing_plan(api, pr, path) != context["existing_plan"]:
        raise ValueError("Saved plan state changed; discard stale plan")
    current = {"issue": issue, **discussion_context(api, issue, pr)}
    if planning_history(current) != planning_history(context):
        raise ValueError("Issue/PR discussion changed; discard stale plan")
    markdown = render(report, context)
    marker = f"<!-- agent-plan-request:{mode}:{request_id} -->"
    destination = pr["number"] if pr else issue["number"]
    comments = api.pages(f"/issues/{destination}/comments")
    if any(
        marker in (c.get("body") or "") and c.get("user", {}).get("type") == "Bot" for c in comments
    ):
        print("Already published this request")
        return
    if mode == "scope":
        scope_status(
            api,
            payload,
            "✅ **Оценка границ исправления завершена**\n\n"
            + markdown
            + "\nДля ветки и черновика PR с планом: `/create-plan`. Для изменения плана в его PR: `/discuss Поправь план: <изменения>`.",
        )
        return
    description = report["branch_description"].replace("_", " ")
    verb = "create" if mode == "create-plan" else "update"
    subject = f"feat: {verb} fix plan for #{issue['number']}: {description}"
    actor = payload["comment"]["user"]["login"]
    request_command = "/create-plan" if mode == "create-plan" else "/discuss"
    request_url = (
        f"https://github.com/{REPOSITORY}/{'pull' if pr else 'issues'}/{destination}"
        f"#issuecomment-{request_id}"
    )
    request_trailer = f"Request: {request_url}"
    message = (
        subject + f"\n\nRefs #{issue['number']}\n"
        f"Requested by @{actor} via {request_command}.\n"
        f"\n{request_trailer}"
    )
    if mode == "create-plan":
        existing = managed_prs(api, issue["number"])
        if existing:
            api.request(
                f"/issues/{issue['number']}/comments",
                {"body": marker + "\nПлан уже существует: " + existing[0]["html_url"]},
            )
            return
        try:
            api.request("/git/ref/heads/" + branch)
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
        else:
            raise ValueError(
                "Plan branch already exists without a managed PR; refusing to overwrite"
            )
        sha = write_plan_commit(api, current_sha, path, markdown, message)
        api.request("/git/refs", {"ref": "refs/heads/" + branch, "sha": sha})
        prefix = next(
            (p for p in ("INFRA/ML:", "ML:", "INFRA:") if issue["title"].startswith(p)), "INFRA:"
        )
        created = api.request(
            "/pulls",
            {
                "title": f"{prefix} {subject}",
                "head": branch,
                "base": base_branch,
                "draft": True,
                "body": f"<!-- agent-plan:issue={issue['number']} -->\nRefs #{issue['number']}\n\n"
                f"План запрошен [комментарием @{actor}]({request_url}) (`{request_command}`).\n\n"
                "Только план исправления. Реализация отдельно после решения разработчика.\n\n"
                "Для изменения плана: `/discuss Поправь план: <изменения>`. Ревью — только `/agent-review`.\n"
                "CI не подтверждён; создание PR через GITHUB_TOKEN само по себе не запускает другие workflow.",
            },
        )
        api.request(
            f"/issues/{issue['number']}/comments",
            {"body": marker + "\nСоздан черновик PR с планом: " + created["html_url"]},
        )
    else:
        # New commit has the exact reviewed head as parent. force=false makes a
        # concurrent branch update fail rather than overwriting another edit.
        current = api.request("/git/commits/" + context["source_sha"])
        if request_trailer in current["message"].splitlines():
            print("This request already updated the plan commit")
        else:
            sha = write_plan_commit(api, context["source_sha"], path, markdown, message)
            api.request("/git/refs/heads/" + branch, {"sha": sha, "force": False}, method="PATCH")
        plan_status(api, payload, "✅ План обновлён в `" + path + "`. Исходники не изменялись.")


if __name__ == "__main__":
    {
        "authorize": authorize,
        "start": start,
        "prepare": prepare,
        "invoke": invoke,
        "publish": publish,
    }[sys.argv[1]]()
