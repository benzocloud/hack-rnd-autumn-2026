import copy
import io
import json
import os
import sys
import tempfile
import unittest
from contextlib import contextmanager, nullcontext, redirect_stdout
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent.parent / "review"))
sys.path.insert(0, str(Path(__file__).parent.parent / "runner"))
import github_review
import ssh_review
from github_api import GitHub, pull_request_discussion
from review_policy import validate_report
from test_dialogue import cross_reference, related_pr
from test_review import context, event, report


@contextmanager
def temporary_workdir():
    previous = Path.cwd()
    with tempfile.TemporaryDirectory() as folder:
        try:
            os.chdir(folder)
            yield Path(folder)
        finally:
            os.chdir(previous)


class API:
    def __init__(self, body=None):
        self.pr = {
            "number": 2,
            "state": "open",
            "changed_files": 1,
            "title": context()["title"],
            "body": body,
            "head": {"sha": "a" * 40, "repo": {"full_name": github_review.REPOSITORY}},
            "base": {"sha": "b" * 40},
        }
        self.comments = [event()["comment"]]
        self.issue = {"number": 7, "state": "closed", "title": "Task", "body": "Criteria"}
        self.issue_comments = [{"id": 70, "body": "Issue clarification"}]
        self.reviews = [{"id": 80, "body": "Earlier review decision", "state": "COMMENTED"}]
        self.inline = [
            {"id": 90, "body": "Inline observation", "path": "ml/main.py"},
            {"id": 91, "in_reply_to_id": 90, "body": "Clarifying reply"},
        ]
        self.sibling = None
        self.sibling_unavailable = False
        self.sibling_comments = [
            {
                "id": 301,
                "body": "Historical decision",
                "user": {"login": "dev"},
                "created_at": "2026-10-22T10:00:00Z",
            }
        ]
        self.sibling_reviews = [{"id": 302, "state": "COMMENTED", "body": "Keep scope"}]
        self.sibling_inline = [{"id": 303, "in_reply_to_id": 300, "path": "ml/main.py", "line": 5}]
        self.writes = []
        self.reads = []

    def request(self, path, body=None, raw=False, accept=None, method=None):
        if body is not None:
            self.writes.append((path, body, method))
            if path == "/issues/2/comments":
                self.comments.append(
                    {
                        "id": 50,
                        "body": body["body"],
                        "user": {"login": "github-actions[bot]", "type": "Bot"},
                    }
                )
                return self.comments[-1]
            if path == "/issues/comments/50" and method == "PATCH":
                self.comments[-1]["body"] = body["body"]
                return self.comments[-1]
            raise AssertionError("Unexpected publication " + path)
        self.reads.append(path)
        if path.endswith("/permission"):
            return {"permission": "write"}
        if path == "/pulls/2":
            return b"diff" if raw else self.pr
        if path == "/issues/7":
            return copy.deepcopy(self.issue)
        if path == "/pulls/3":
            if self.sibling_unavailable:
                raise RuntimeError("Related PR API unavailable")
            return copy.deepcopy(self.sibling)
        if path.startswith("/tarball/"):
            return b"fixture archive"
        if path.endswith("/status"):
            return {"state": "success"}
        raise AssertionError("Unexpected API read " + path)

    def pages(self, path, key=None):
        if path == "/pulls/2/files":
            return [{"filename": "ml/main.py"}]
        if path == "/issues/2/comments":
            return copy.deepcopy(self.comments)
        if path == "/issues/7/comments":
            return copy.deepcopy(self.issue_comments)
        if path == "/issues/7/timeline":
            return [cross_reference(3)] if self.sibling else []
        if path == "/pulls/2/reviews":
            return copy.deepcopy(self.reviews)
        if path == "/pulls/2/comments":
            return copy.deepcopy(self.inline)
        if path in ("/issues/3/comments", "/pulls/3/reviews", "/pulls/3/comments"):
            return copy.deepcopy(
                {
                    "/issues/3/comments": self.sibling_comments,
                    "/pulls/3/reviews": self.sibling_reviews,
                    "/pulls/3/comments": self.sibling_inline,
                }[path]
            )
        if path.endswith("/check-runs"):
            return []
        raise AssertionError("Unexpected pages " + path)


class ReviewContextTests(unittest.TestCase):
    def setUp(self):
        environment = patch.dict(
            os.environ,
            {
                "GITHUB_EVENT_NAME": "issue_comment",
                "GITHUB_RUN_ATTEMPT": "1",
                "GITHUB_RUN_ID": "123",
            },
        )
        environment.start()
        self.addCleanup(environment.stop)

    def test_no_issue_context_is_valid_but_ci_and_issue_list_stay_required(self):
        c = context()
        c["issues"] = []
        validate_report(report(), c)
        for key in ("ci", "issues"):
            incomplete = dict(c)
            incomplete.pop(key)
            with self.assertRaises(ValueError):
                validate_report(report(), incomplete)

    def test_prepare_collects_full_pr_discussion_with_optional_linked_issue(self):
        for body in (None, "Refs #7"):
            with self.subTest(body=body), temporary_workdir() as folder:
                api = API(body)
                with (
                    patch.dict(os.environ, {"GITHUB_OUTPUT": str(folder / "outputs")}),
                    patch.object(github_review, "GitHub", return_value=api),
                    patch.object(github_review, "event", return_value=event()),
                    patch.object(Path, "read_text", return_value="Trusted policy"),
                ):
                    github_review.prepare()
                c = json.loads(Path("context.json").read_text())
                self.assertEqual(len(c["issues"]), 1 if body else 0)
                self.assertEqual(c["pr_reviews"][0]["body"], "Earlier review decision")
                self.assertEqual(c["pr_review_comments"][1]["in_reply_to_id"], 90)
                self.assertEqual(c["pr_comments"][0]["body"], "/agent-review")
                if body:
                    self.assertEqual(c["issues"][0]["comments"][0]["body"], "Issue clarification")
                else:
                    self.assertNotIn("/issues/7", api.reads)

    def test_pagination_keeps_later_comments_and_refuses_silent_partial_context(self):
        api = object.__new__(GitHub)
        with patch.object(
            api,
            "request",
            side_effect=[[{"id": n} for n in range(100)], [{"id": 100, "in_reply_to_id": 99}]],
        ) as read:
            comments = api.pages("/pulls/2/comments")
        self.assertEqual(len(comments), 101)
        self.assertEqual(comments[-1]["in_reply_to_id"], 99)
        self.assertIn("page=2", read.call_args.args[0])
        with (
            patch.object(api, "request", return_value=[{}] * 100),
            self.assertRaisesRegex(ValueError, "partial"),
        ):
            api.pages("/issues/2/comments")

    def test_discussion_fetch_failure_is_not_treated_as_empty_context(self):
        with (
            patch.object(API, "pages", side_effect=RuntimeError("API unavailable")),
            self.assertRaisesRegex(RuntimeError, "unavailable"),
        ):
            pull_request_discussion(API(), 2)

    def test_one_start_comment_is_edited_with_report_without_an_issue(self):
        with (
            temporary_workdir(),
            patch.dict(
                os.environ,
                {
                    "GITHUB_RUN_ID": "123",
                    "REVIEW_JOB_RESULT": "success",
                    "REVIEW_REQUESTED": "true",
                },
            ),
        ):
            api = API()
            c = context()
            c["issues"] = []
            c["body"] = None
            with (
                patch.object(github_review, "GitHub", return_value=api),
                patch.object(github_review, "event", return_value=event()),
            ):
                github_review.start()
                self.assertIn("Начал ревью", api.comments[-1]["body"])
                github_review.start()
                c.update(github_review.discussion_context(api, api.pr))
                Path("report.json").write_text(json.dumps({"context": c, "report": report()}))
                github_review.publish()
            self.assertEqual(
                [p for p, _, _ in api.writes], ["/issues/2/comments", "/issues/comments/50"]
            )
            self.assertEqual(api.writes[-1][2], "PATCH")
            self.assertIn("Связанный Issue не указан", api.comments[-1]["body"])
            self.assertIn("Описание PR пустое", api.comments[-1]["body"])
            self.assertIn("Ревью агента: **завершено**", api.comments[-1]["body"])
            self.assertIn("a" * 40, api.comments[-1]["body"])

    def test_failed_or_cancelled_analysis_updates_start_comment_without_report(self):
        for result in ("failure", "cancelled"):
            with (
                self.subTest(result=result),
                temporary_workdir(),
                patch.dict(os.environ, {"GITHUB_RUN_ID": "123", "REVIEW_JOB_RESULT": result}),
            ):
                api = API()
                with (
                    patch.object(github_review, "GitHub", return_value=api),
                    patch.object(github_review, "event", return_value=event()),
                ):
                    github_review.start()
                    github_review.publish()
                self.assertEqual(api.writes[-1][0], "/issues/comments/50")
                self.assertIn(
                    "отменено" if result == "cancelled" else "завершилось ошибкой",
                    api.comments[-1]["body"],
                )
                self.assertIn("actions/runs/123", api.comments[-1]["body"])

    def prepare_shared(self, api):
        with (
            patch.dict(os.environ, {"GITHUB_OUTPUT": str(Path.cwd() / "outputs")}),
            patch.object(github_review, "GitHub", return_value=api),
            patch.object(github_review, "event", return_value=event()),
            patch.object(Path, "read_text", return_value="Trusted policy"),
        ):
            github_review.start()
            github_review.prepare()
        return json.loads(Path("context.json").read_text())

    def test_shared_review_context_reaches_worker_with_closed_issue_and_pr_history(self):
        with temporary_workdir() as folder:
            api = API("Closes: #7")
            api.sibling = related_pr()
            c = self.prepare_shared(api)
            self.assertEqual(c["dialogue_root"], 7)
            self.assertEqual(c["issues"][0]["state"], "closed")
            self.assertEqual(c["related_prs"][0]["pr_comments"][0]["user"]["login"], "dev")
            self.assertEqual(c["related_prs"][0]["pr_review_comments"][0]["in_reply_to_id"], 300)
            package = json.loads(Path("request.json").read_text())
            self.assertNotIn("GH_TOKEN", package)
            Path("required-skills.md").write_text("Trusted fixture skills")
            receipts = [{"name": name, "sha256": "0" * 64} for name in c["required_skills"]]
            with (
                patch.object(ssh_review.os, "geteuid", return_value=1000),
                patch.dict(os.environ, {"SSH_ORIGINAL_COMMAND": "agent-review"}),
                patch.object(ssh_review.sys, "stdin") as stdin,
                patch.object(ssh_review, "workspace", return_value=nullcontext(folder)),
                patch.object(ssh_review, "load_skills", return_value=receipts),
                patch.object(ssh_review, "infer", return_value=report()) as infer,
                redirect_stdout(io.StringIO()),
            ):
                stdin.buffer = io.BytesIO(json.dumps(package).encode())
                ssh_review.main()
                self.assertEqual(json.loads(Path("context.json").read_text()), c)
                self.assertIn("related_prs", infer.call_args.args[2])
            Path("report.json").write_text(json.dumps({"context": c, "report": report()}))
            with (
                patch.object(github_review, "GitHub", return_value=api),
                patch.object(github_review, "event", return_value=event()),
            ):
                github_review.publish()
            self.assertIn("Ревью агента: **завершено**", api.comments[-1]["body"])
            self.assertIn("pull/3", api.comments[-1]["body"])
            self.assertEqual(
                [path for path, _, _ in api.writes], ["/issues/2/comments", "/issues/comments/50"]
            )

    def test_review_discards_changed_shared_history_and_keeps_one_status_comment(self):
        for change in (
            "body",
            "issue",
            "issue-comment",
            "origin-comment",
            "origin-inline",
            "related-comment",
            "review",
            "inline",
            "head",
            "added",
            "removed",
        ):
            with self.subTest(change=change), temporary_workdir():
                api = API("Refs #7")
                if change != "added":
                    api.sibling = related_pr()
                c = self.prepare_shared(api)
                Path("report.json").write_text(json.dumps({"context": c, "report": report()}))
                if change == "body":
                    api.pr["body"] += "\nChanged requirement"
                elif change == "issue":
                    api.issue["body"] += " Changed"
                elif change == "issue-comment":
                    api.issue_comments[0]["body"] += " Changed"
                elif change == "origin-comment":
                    api.comments[0]["body"] = "/question Changed"
                elif change == "origin-inline":
                    api.inline[0]["path"] = "ml/another.py"
                elif change == "related-comment":
                    api.sibling_comments[0]["body"] += " Changed"
                elif change == "review":
                    api.sibling_reviews[0]["state"] = "DISMISSED"
                elif change == "inline":
                    api.sibling_inline[0]["line"] += 1
                elif change == "head":
                    api.sibling["head"]["sha"] = "d" * 40
                elif change == "added":
                    api.sibling = related_pr()
                else:
                    api.sibling = None
                with (
                    patch.object(github_review, "GitHub", return_value=api),
                    patch.object(github_review, "event", return_value=event()),
                ):
                    github_review.publish()
                self.assertIn("устарел", api.comments[-1]["body"])
                self.assertNotIn("Ревью агента: **завершено**", api.comments[-1]["body"])
                self.assertEqual(len(api.writes), 2)

    def test_review_rejects_ambiguous_or_unavailable_shared_history(self):
        for change in ("ambiguous", "unavailable", "oversized"):
            with self.subTest(change=change), temporary_workdir():
                api = API("Refs #7")
                api.sibling = related_pr()
                c = self.prepare_shared(api)
                Path("report.json").write_text(json.dumps({"context": c, "report": report()}))
                if change == "ambiguous":
                    api.pr["body"] += "\nCloses: #8"
                elif change == "unavailable":
                    api.sibling_unavailable = True
                else:
                    api.sibling_comments[0]["body"] = "x" * 160000
                with (
                    patch.object(github_review, "GitHub", return_value=api),
                    patch.object(github_review, "event", return_value=event()),
                    self.assertRaises((ValueError, RuntimeError)),
                ):
                    github_review.publish()
                self.assertIn("не опубликован", api.comments[-1]["body"])
                self.assertEqual(len(api.writes), 2)

    def test_review_ignores_reactions_and_display_metadata_in_shared_history(self):
        with temporary_workdir():
            api = API("Refs #7")
            api.sibling = related_pr()
            c = self.prepare_shared(api)
            Path("report.json").write_text(json.dumps({"context": c, "report": report()}))
            api.sibling_comments[0]["reactions"] = {"+1": 1}
            api.sibling_comments[0]["user"]["avatar_url"] = "https://example.test/avatar"
            api.issue["updated_at"] = "2026-10-22T15:00:00Z"
            with (
                patch.object(github_review, "GitHub", return_value=api),
                patch.object(github_review, "event", return_value=event()),
            ):
                github_review.publish()
            self.assertIn("Ревью агента: **завершено**", api.comments[-1]["body"])
