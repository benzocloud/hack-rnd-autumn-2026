"""Explicit Issue/PR associations; GitHub-hosted reads only, never model jobs."""

import re

from github_api import REPOSITORY, pull_request_discussion


def issue_references(body):
    reference = (
        r"(?:#[1-9][0-9]*|https://github\.com/"
        + re.escape(REPOSITORY)
        + r"/issues/[1-9][0-9]*|"
        + re.escape(REPOSITORY)
        + r"#[1-9][0-9]*)(?![\w/])"
    )
    pattern = (
        r"(?<![\w/])(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?|refs?)(?:\s*:\s*|\s+)("
        + reference
        + r"(?:\s*(?:,|\band\b|&)\s*"
        + reference
        + r")*)"
    )
    numbers = {
        int(value.rsplit("/", 1)[-1].rsplit("#", 1)[-1])
        for match in re.finditer(pattern, body or "", re.I)
        for value in re.findall(reference, match[1], re.I)
    }
    numbers.update(
        int(n) for n in re.findall(r"<!-- agent-plan:issue=([1-9][0-9]*) -->", body or "")
    )
    return numbers


def dialogue_root(task, pr):
    if pr is None:
        return task["number"]
    numbers = issue_references(task.get("body"))
    if len(numbers) > 1:
        raise ValueError("Ambiguous Issue dialogue root")
    return next(iter(numbers), None)


def substantive_item(item):
    """Keep task text, authors and review positions, ignoring display metadata."""
    fields = (
        "id",
        "number",
        "title",
        "body",
        "state",
        "created_at",
        "submitted_at",
        "closed_at",
        "merged_at",
        "draft",
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


def substantive_history(context):
    return {
        "dialogue_root": context["dialogue_root"],
        **{
            key: [substantive_item(item) for item in context[key]]
            for key in ("issue_comments", "pr_comments", "pr_reviews", "pr_review_comments")
        },
        "related_prs": [
            {
                **thread,
                "task": substantive_item(thread["task"]),
                **{
                    key: [substantive_item(item) for item in thread[key]]
                    for key in ("pr_comments", "pr_reviews", "pr_review_comments")
                },
            }
            for thread in context["related_prs"]
        ],
    }


def related_pull_requests(api, issue_number, origin_number):
    candidates = set()
    for event in api.pages(f"/issues/{issue_number}/timeline"):
        if event.get("event") != "cross-referenced":
            continue
        source = event.get("source", {}).get("issue", {})
        number = source.get("number")
        if (
            type(number) is int
            and number > 0
            and number != origin_number
            and "pull_request" in source
            and source.get("url") == f"https://api.github.com/repos/{REPOSITORY}/issues/{number}"
        ):
            candidates.add(number)
    if len(candidates) > 10:
        raise ValueError("Too many related PR candidates; refusing partial dialogue")
    result = []
    for number in sorted(candidates):
        pr = api.request(f"/pulls/{number}")
        if (pr["head"].get("repo") or {}).get("full_name") != REPOSITORY:
            continue
        roots = issue_references(pr.get("body"))
        if issue_number not in roots:
            continue
        if len(roots) != 1:
            raise ValueError("Ambiguous Issue dialogue root in a related PR")
        result.append(
            {
                "task": {
                    key: pr[key]
                    for key in (
                        "number",
                        "title",
                        "body",
                        "state",
                        "user",
                        "created_at",
                        "closed_at",
                        "merged_at",
                        "draft",
                        "html_url",
                    )
                    if key in pr
                },
                "head_sha": pr["head"]["sha"],
                "base_sha": pr["base"]["sha"],
                **pull_request_discussion(api, number),
            }
        )
    return result
