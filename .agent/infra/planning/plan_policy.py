"""Issue-to-plan protocol: no source implementation or arbitrary write paths."""

import base64
import hashlib
import json
import re
import sys
from pathlib import Path
from urllib.parse import quote

sys.path.insert(0, str(Path(__file__).resolve().parent.parent / "review"))
from review_policy import REPOSITORY, SHA, safe_path, validate_history

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

REVIEW_MODES = ("per-stage", "after-all")
STAGE_STATUSES = ("Не начат", "В работе", "На ревью", "Принят")
STAGE_LISTS = ("affected_files", "external_dependencies", "actions", "checks", "acceptance")
STAGE_SCHEMA = {
    "type": "object",
    "additionalProperties": False,
    "properties": {
        "number": {"type": "integer", "minimum": 1},
        "title": {"type": "string", "minLength": 1, "maxLength": 3000, "pattern": r"^[^\r\n]+$"},
        "result": {"type": "string", "minLength": 1, "maxLength": 3000, "pattern": r"^[^\r\n]+$"},
        "status": {"type": "string", "enum": list(STAGE_STATUSES)},
        "depends_on": {"type": "array", "items": {"type": "integer", "minimum": 1}, "maxItems": 40},
        **{
            name: {
                "type": "array",
                "items": {"type": "string", "minLength": 1, "maxLength": 3000},
                "minItems": 0 if name == "external_dependencies" else 1,
                "maxItems": 40,
            }
            for name in STAGE_LISTS
        },
    },
    "required": ["number", "title", "result", "status", "depends_on", *STAGE_LISTS],
}
STAGE_SCHEMA["properties"]["affected_files"]["items"]["pattern"] = r"^[^\s]+$"

# Scope assessments retain the compact schema; generated plans use stages.
STAGED_SCHEMA = json.loads(json.dumps(SCHEMA))
del STAGED_SCHEMA["properties"]["steps"]
STAGED_SCHEMA["properties"].update(
    {
        "review_mode": {"type": "string", "enum": list(REVIEW_MODES)},
        "stages": {"type": "array", "items": STAGE_SCHEMA, "minItems": 1, "maxItems": 40},
    }
)
STAGED_SCHEMA["required"] = [name for name in SCHEMA["required"] if name != "steps"] + [
    "review_mode",
    "stages",
]

PLAN_STATE_PREFIX = "<!-- agent-plan-state:v1:"
PLAN_FORMAT_MARKER = "<!-- agent-plan-format:v1 -->"
PLAN_FILE_LIMIT = 128 * 1024

SOURCE_REFERENCE = re.compile(
    r"(?P<link>\[[^\]\n]*\]\([^\n)]*\))"
    r"|(?<![\w/`])(?P<tick>`?)"
    r"(?P<path>(?:[\w.-]+/)+[\w.-]+|[\w.-]*\.[\w.-]+)"
    r":(?P<ranges>[1-9][0-9]*(?:[-–][1-9][0-9]*)?"
    r"(?:,\s*[1-9][0-9]*(?:[-–][1-9][0-9]*)?)*)"
    r"(?P=tick)(?![\w–-])"
)


def discussion_command(payload, event_name, attempt="1"):
    comment = payload.get("comment", {})
    if (
        attempt != "1"
        or event_name != "issue_comment"
        or payload.get("action") != "created"
        or comment.get("user", {}).get("type") != "User"
    ):
        return None
    body = comment.get("body") or ""
    for name in ("question", "discuss"):
        prefix = "/" + name
        rest = body[len(prefix) :] if body.startswith(prefix) else ""
        if rest and rest[0].isspace() and rest.strip():
            return name, rest.strip()
    return None


def plan_edit_requested(text):
    # Bind the leading action to the plan itself; a later mention is not consent.
    plan = r"(?:(?:the|this)[ \t]+)?plan\b"
    return bool(
        "?" not in text
        and re.match(
            r"^(?:(?:поправь|поправить|исправь|исправить|измени|изменить|обнови|обновить|дополни|перепиши|уточни)[ \t]+(?:этот[ \t]+)?план\b"
            r"|добавь[ \t]+в[ \t]+(?:этот[ \t]+)?план\b"
            r"|(?:убери|удали)[ \t]+из[ \t]+(?:этого[ \t]+)?плана\b"
            rf"|(?:update|revise|edit|change|amend|rewrite|clarify)[ \t]+{plan}"
            rf"|add[ \t]+to[ \t]+{plan}"
            rf"|remove[ \t]+from[ \t]+{plan})",
            text,
            re.I,
        )
    )


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
    discussion = discussion_command(payload, event_name, attempt)
    if "pull_request" in issue and discussion:
        name, text = discussion
        if (
            name == "discuss"
            and plan_edit_requested(text)
            and re.search(r"<!-- agent-plan:issue=[1-9][0-9]* -->", issue.get("body") or "")
        ):
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
    validate_history(context, context.get("pr_number") or context["issue_number"])
    if context["dialogue_root"] != context["issue_number"]:
        raise ValueError("Planning Issue dialogue root mismatch")
    if context["issue"]["state"] != "open" and not (
        context["issue"]["state"] == "closed"
        and context["mode"] == "discuss-plan"
        and type(context.get("pr_number")) is int
        and context["pr_number"] > 0
        and context["pr_state"] == "open"
    ):
        raise ValueError("Issue must remain open outside an open plan PR")
    if context["mode"] == "discuss-plan":
        _, path = identity(context["issue_number"])
        if (
            not is_plan_branch(context["branch"], context["issue_number"])
            or context["plan_path"] != path
            or not context.get("existing_plan")
        ):
            raise ValueError("Not a managed plan")


def _validate_report_fields(report, context, schema, lists):
    validate_context(context)
    if not isinstance(report, dict):
        raise ValueError("Planning report must be an object")
    if set(report) != set(schema["required"]):
        missing = sorted(set(schema["required"]) - set(report))
        unexpected = sorted(set(report) - set(schema["required"]))
        raise ValueError(
            f"Planning report schema mismatch (missing={missing}, unexpected={unexpected}); "
            "deploy matching GitHub and runner planning adapters"
        )
    if (
        report["source_sha"] != context["source_sha"]
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
    for name in lists:
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
    if not report["in_scope"] or not report["acceptance"]:
        raise ValueError("Scope and acceptance criteria are required")
    if len(json.dumps(report)) > 45000:
        raise ValueError("Plan is too large")


def validate_report(report, context):
    _validate_report_fields(report, context, SCHEMA, LISTS)
    if not report["steps"]:
        raise ValueError("Proposed steps are required")


def _validate_staged_shape(report, context):
    _validate_report_fields(
        report, context, STAGED_SCHEMA, [name for name in LISTS if name != "steps"]
    )
    if report["review_mode"] not in REVIEW_MODES:
        raise ValueError("Invalid plan review mode")
    stages = report["stages"]
    if not isinstance(stages, list) or not 1 <= len(stages) <= 40:
        raise ValueError("A plan requires 1-40 structured stages")
    for number, stage in enumerate(stages, 1):
        if not isinstance(stage, dict) or set(stage) != set(STAGE_SCHEMA["required"]):
            raise ValueError("Invalid stage fields")
        if type(stage["number"]) is not int or stage["number"] != number:
            raise ValueError("Stage numbers must be consecutive and ordered from 1")
        if stage["status"] not in STAGE_STATUSES:
            raise ValueError("Invalid stage status")
        for key in ("title", "result"):
            if not isinstance(stage[key], str) or not stage[key].strip() or len(stage[key]) > 3000:
                raise ValueError("Stage title and result must be nonempty text")
            if stage[key].splitlines() != [stage[key]]:
                raise ValueError("Stage title and result must be single lines")
        dependencies = stage["depends_on"]
        if (
            not isinstance(dependencies, list)
            or any(type(n) is not int or not 1 <= n < number for n in dependencies)
            or len(set(dependencies)) != len(dependencies)
        ):
            raise ValueError("Stage dependencies must name distinct earlier stages")
        for key in STAGE_LISTS:
            values = stage[key]
            if (
                not isinstance(values, list)
                or len(values) > 40
                or (not values and key != "external_dependencies")
                or any(not isinstance(v, str) or not v.strip() or len(v) > 3000 for v in values)
            ):
                raise ValueError("Invalid stage details")
        if not set(stage["affected_files"]) <= set(report["affected_files"]):
            raise ValueError("Stage paths must belong to the declared affected files")


def validate_staged_report(
    report, context, *, previous_plan=None, approved_review_mode="per-stage"
):
    """Validate model proposals against caller-attested state, never Markdown claims.

    previous_plan is the trusted prior structured report (or a legacy report),
    bound by the caller to this Issue/source history. approved_review_mode may
    differ from per-stage only after the caller verifies developer authorization
    by meaning and trusted context, without requiring an exact phrase or language.
    Neither value may be taken from model output or unverified Markdown claims.
    """
    _validate_staged_shape(report, context)
    if approved_review_mode not in REVIEW_MODES or report["review_mode"] != approved_review_mode:
        raise ValueError("Plan review mode lacks developer authorization")
    previous = []
    if previous_plan is not None:
        if context["mode"] != "discuss-plan" or not isinstance(previous_plan, dict):
            raise ValueError("Prior plan state requires a managed plan revision")
        prior_context = {**context, "source_sha": previous_plan.get("source_sha")}
        if not isinstance(prior_context["source_sha"], str):
            raise ValueError("Invalid prior plan source")
        if "stages" in previous_plan:
            _validate_staged_shape(previous_plan, prior_context)
            previous = previous_plan["stages"]
        else:
            validate_report(previous_plan, prior_context)
    old_stages = {stage["number"]: stage for stage in previous}
    new_stages = {stage["number"]: stage for stage in report["stages"]}
    for stage in report["stages"]:
        old = old_stages.get(stage["number"])
        if stage["status"] != (old["status"] if old else "Не начат"):
            raise ValueError("Stage progress requires trusted developer/execution state")
    for old in previous:
        new = new_stages.get(old["number"])
        if old["status"] == "Принят" and new != old:
            raise ValueError("Accepted stage history must be preserved")
        if old["status"] != "Не начат" and new is None:
            raise ValueError("Cannot discard an implementation stage already in progress")


def source_links(text, sha):
    def replace(match):
        if match.group("link"):
            return match[0]
        path = match["path"]
        try:
            safe_path(path)
        except ValueError:
            return match[0]
        links = []
        for span in match["ranges"].split(","):
            span = span.strip()
            bounds = re.split("[-–]", span)
            start, end = int(bounds[0]), int(bounds[-1])
            if end < start:
                return match[0]
            anchor = f"#L{start}" + (f"-L{end}" if len(bounds) == 2 else "")
            url = f"https://github.com/{REPOSITORY}/blob/{sha}/{quote(path, safe='/')}{anchor}"
            links.append(f"[`{path}:{span}`]({url})")
        return ", ".join(links)

    return SOURCE_REFERENCE.sub(replace, text)


def previous_plan(context):
    """Read structured state from the fixed Git file, never from discussion text.

    The GitHub publisher must re-fetch the same file at the authorized PR head
    before using this state. The worker cannot authorize a new mode or progress.
    Legacy Markdown has no trusted progress and migrates to unstarted stages.
    """
    if context["mode"] != "discuss-plan":
        return None
    markdown = context["existing_plan"]
    match = re.search(r"\n<!-- agent-plan-state:v1:([A-Za-z0-9+/=]+) -->\n?\Z", markdown)
    if match is None:
        if (
            PLAN_STATE_PREFIX in markdown
            or PLAN_FORMAT_MARKER in markdown
            or re.search(r"(?m)^## Этап [1-9][0-9]* — ", markdown)
        ):
            raise ValueError("Missing or invalid stored plan state")
        return None
    try:
        report = json.loads(base64.b64decode(match[1], validate=True))
    except (ValueError, UnicodeDecodeError) as error:
        raise ValueError("Invalid stored plan state") from error
    if not isinstance(report, dict) or not isinstance(report.get("source_sha"), str):
        raise ValueError("Invalid stored plan state")
    prior_context = {**context, "source_sha": report["source_sha"]}
    validate_staged_report(
        report,
        prior_context,
        previous_plan=report,
        approved_review_mode=report.get("review_mode"),
    )
    if render_staged_plan(report) != markdown:
        raise ValueError("Stored plan text and structured state disagree")
    return report


def report_schema(context):
    if context["mode"] == "scope":
        return json.loads(json.dumps(SCHEMA))
    previous = previous_plan(context)
    schema = json.loads(json.dumps(STAGED_SCHEMA))
    schema["properties"]["review_mode"]["enum"] = [
        previous["review_mode"] if previous else "per-stage"
    ]
    return schema


def validate_response(report, context):
    if context["mode"] == "scope":
        validate_report(report, context)
        return
    previous = previous_plan(context)
    validate_staged_report(
        report,
        context,
        previous_plan=previous,
        approved_review_mode=previous["review_mode"] if previous else "per-stage",
    )


def render(report, context):
    validate_response(report, context)
    if context["mode"] != "scope":
        return render_staged_plan(report)
    lines = [
        f"# План исправления Issue #{context['issue_number']}",
        source_links(report["summary"], report["source_sha"]),
        f"Исходники: [`{report['source_sha']}`](https://github.com/{REPOSITORY}/commit/{report['source_sha']}). Статус: "
        + ("готов к обсуждению." if report["status"] == "ready" else "нужны уточнения."),
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
        lines += [
            "## " + label,
            "\n".join("- " + source_links(value, report["source_sha"]) for value in report[name])
            or "Нет.",
        ]
    return "\n\n".join(lines) + "\n"


def plan_text(text, sha):
    """Render model text literally, keeping generated source references as links."""

    def escape(value):
        return re.sub(r"([\\`*_{}\[\]()#+.!<>|~=-])", r"\\\1", value)

    text = "\n".join(text.splitlines())
    result, cursor = [], 0
    for match in SOURCE_REFERENCE.finditer(text):
        result.append(escape(text[cursor : match.start()]))
        if match.group("link"):
            link = re.fullmatch(r"\[([^\]\n]*)\]\((https?://[^\s<>]+)\)", match[0])
            result.append(
                f"[{escape(link[1])}]({quote(link[2], safe='/:#?&=%+-._~')})"
                if link
                else escape(match[0])
            )
        else:
            linked = source_links(match[0], sha)
            result.append(linked if linked != match[0] else escape(match[0]))
        cursor = match.end()
    result.append(escape(text[cursor:]))
    return "".join(result)


def render_staged_plan(report):
    sha = report["source_sha"]

    def items(values, checklist=False):
        prefix = "- [ ] " if checklist else "- "
        return (
            "\n".join(prefix + plan_text(value, sha).replace("\n", "\n    ") for value in values)
            or "Нет."
        )

    after_all = report["review_mode"] == "after-all"
    review = (
        "После всех этапов — остановка для итогового ревью и апрува разработчика."
        if after_all
        else "После каждого этапа — остановка до ревью и явного апрува разработчика."
    )
    lines = [
        f"# План исправления Issue #{report['issue_number']}",
        PLAN_FORMAT_MARKER,
        "## Цель",
        plan_text(report["summary"], sha),
        "Статус плана: "
        + ("готов к обсуждению." if report["status"] == "ready" else "нужны уточнения."),
        "Режим ревью: " + review,
        "План не запускает реализацию. Перечисленные действия и проверки предложены, "
        "а не выполнены. GBrain недоступен; анализ статический, тесты не запускались.",
        "## Текущее состояние и исходники",
        f"Исходники: [`{sha}`](https://github.com/{REPOSITORY}/commit/{sha}).",
        items(report["evidence"]),
        "## Граница исправления",
        items(report["in_scope"]),
        "## Вне скоупа",
        items(report["out_of_scope"]),
        "## Затрагиваемые файлы",
        items(report["affected_files"]),
    ]
    for stage in report["stages"]:
        dependencies = ", ".join(f"этап {n}" for n in stage["depends_on"]) or "отсутствуют"
        lines += [
            f"## Этап {stage['number']} — " + plan_text(stage["title"], sha),
            f"Статус: {stage['status']}.",
            "Результат: " + plan_text(stage["result"], sha),
            "Зависимости: " + dependencies + ".",
            "### Внешние зависимости",
            items(stage["external_dependencies"]),
            "### Затрагиваемые файлы",
            items(stage["affected_files"]),
            "### Предлагаемые действия",
            items(stage["actions"], checklist=True),
            "### Предлагаемые проверки",
            items(stage["checks"], checklist=True),
            "### Критерии завершения",
            items(stage["acceptance"]),
            "Точка ревью: "
            + (
                "после всех этапов; до итогового апрува этап остаётся на ревью."
                if after_all
                else "остановиться после этапа до ревью и явного апрува разработчика."
            ),
        ]
    lines += [
        "## Общие критерии приёмки",
        items(report["acceptance"]),
        "## Риски",
        items(report["risks"]),
        "## Открытые вопросы",
        items(report["questions"]),
        "Апрув этапа не разрешает коммит, отправку изменений, слияние или развёртывание.",
    ]
    state = base64.b64encode(json.dumps(report).encode()).decode()
    markdown = "\n\n".join(lines) + f"\n\n{PLAN_STATE_PREFIX}{state} -->\n"
    if len(markdown.encode()) > PLAN_FILE_LIMIT:
        raise ValueError("Rendered plan exceeds file limit")
    return markdown
