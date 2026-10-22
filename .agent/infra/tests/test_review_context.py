import json
import os
import sys
import tempfile
import unittest
from contextlib import contextmanager
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent.parent / "review"))
sys.path.insert(0, str(Path(__file__).parent.parent / "runner"))
import github_review
from github_api import GitHub, pull_request_discussion
from review_policy import validate_report
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
            return {"number": 7, "title": "Task", "body": "Criteria"}
        if path.startswith("/tarball/"):
            return b"fixture archive"
        if path.endswith("/status"):
            return {"state": "success"}
        raise AssertionError("Unexpected API read " + path)

    def pages(self, path, key=None):
        if path == "/pulls/2/files":
            return [{"filename": "ml/main.py"}]
        if path == "/issues/2/comments":
            return self.comments
        if path == "/issues/7/comments":
            return [{"id": 70, "body": "Issue clarification"}]
        if path == "/pulls/2/reviews":
            return [{"id": 80, "body": "Earlier review decision", "state": "COMMENTED"}]
        if path == "/pulls/2/comments":
            return [
                {"id": 90, "body": "Inline observation", "path": "ml/main.py"},
                {"id": 91, "in_reply_to_id": 90, "body": "Clarifying reply"},
            ]
        if path.endswith("/check-runs"):
            return []
        raise AssertionError("Unexpected pages " + path)


class ReviewContextTests(unittest.TestCase):
    def setUp(self):
        environment = patch.dict(
            os.environ, {"GITHUB_EVENT_NAME": "issue_comment", "GITHUB_RUN_ATTEMPT": "1"}
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
            Path("report.json").write_text(json.dumps({"context": c, "report": report()}))
            with (
                patch.object(github_review, "GitHub", return_value=api),
                patch.object(github_review, "event", return_value=event()),
            ):
                github_review.start()
                github_review.start()
                github_review.publish()
            self.assertEqual(
                [p for p, _, _ in api.writes], ["/issues/2/comments", "/issues/comments/50"]
            )
            self.assertEqual(api.writes[-1][2], "PATCH")
            self.assertIn("No linked Issue", api.comments[-1]["body"])
            self.assertIn("PR description is empty", api.comments[-1]["body"])
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
                    "cancelled" if result == "cancelled" else "failed", api.comments[-1]["body"]
                )
                self.assertIn("actions/runs/123", api.comments[-1]["body"])
