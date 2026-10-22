import io
import json
import os
import sys
import tarfile
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent.parent / "review"))
sys.path.insert(0, str(Path(__file__).parent.parent / "runner"))
import github_review
import review_dispatch
import ssh_review
from review_policy import REPOSITORY, is_request, skills_for, validate_report


def context():
    return {
        "repository": REPOSITORY,
        "pr_number": 2,
        "comment_id": 3,
        "head_sha": "a" * 40,
        "base_sha": "b" * 40,
        "title": "ML: fix: classifier",
        "scope": "ML",
        "required_skills": ["caveman-review", "ponytail-review"],
        "files": ["ml/main.py"],
        "issues": [],
        "dialogue_root": None,
        "related_prs": [],
        "issue_comments": [],
        "pr_comments": [],
        "pr_reviews": [],
        "pr_review_comments": [],
        "body": None,
        "ci": {},
    }


def report():
    return {
        "head_sha": "a" * 40,
        "base_sha": "b" * 40,
        "scope": "ML",
        "status": "complete",
        "memory": "unavailable",
        "skills": ["caveman-review", "ponytail-review"],
        "checks": ["Статический анализ"],
        "limitations": ["Тесты не запускались"],
        "findings": [],
    }


def event():
    return {
        "action": "created",
        "issue": {"number": 2, "pull_request": {"url": "example"}},
        "comment": {
            "id": 3,
            "body": "/agent-review",
            "user": {"login": "reviewer", "type": "User"},
        },
    }


class ReviewTests(unittest.TestCase):
    def setUp(self):
        environment = patch.dict(
            os.environ, {"GITHUB_EVENT_NAME": "issue_comment", "GITHUB_RUN_ATTEMPT": "1"}
        )
        environment.start()
        self.addCleanup(environment.stop)

    def test_privileged_dispatch_rejects_arbitrary_original_command(self):
        with (
            patch.object(review_dispatch.os, "geteuid", return_value=0),
            patch.object(review_dispatch.sys, "argv", ["review_dispatch.py"]),
            patch.dict(os.environ, {"SSH_ORIGINAL_COMMAND": "agent-review; id"}),
            patch.object(review_dispatch.subprocess, "call") as invoke,
        ):
            with self.assertRaises(SystemExit):
                review_dispatch.main()
            invoke.assert_not_called()

    def test_only_new_exact_human_pr_comment_triggers(self):
        self.assertTrue(is_request(event()))
        for body in ("please /agent-review", "/agent-review now", " /agent-review", "hello"):
            e = event()
            e["comment"]["body"] = body
            self.assertFalse(is_request(e))
        for action in ("edited", "deleted"):
            e = event()
            e["action"] = action
            self.assertFalse(is_request(e))
        e = event()
        e["issue"].pop("pull_request")
        self.assertFalse(is_request(e))
        self.assertFalse(is_request(event(), "push"))
        self.assertFalse(is_request(event(), attempt="2"))
        e = event()
        e["comment"]["user"]["type"] = "Bot"
        self.assertFalse(is_request(e))

    def test_literal_combined_prefix_and_go_changes(self):
        self.assertNotIn("golang-how-to", skills_for("ML: feat: train", ["ml/train.py"])[1])
        self.assertIn("golang-how-to", skills_for("INFRA/ML: feat: connect", ["ml/train.py"])[1])
        self.assertNotIn(
            "golang-how-to", skills_for("INFRA: fix: CI", [".github/workflows/ci.yml"])[1]
        )
        self.assertIn("golang-how-to", skills_for("INFRA: fix: tool", ["tools/main.go"])[1])
        self.assertIn("golang-how-to", skills_for("fix: modules", ["api/go.mod"])[1])
        self.assertNotIn(
            "golang-how-to", skills_for("ML: feat: mixed", ["ml/train.py", "api/main.go"])[1]
        )

    def test_read_only_requester_never_reaches_pr_or_runner(self):
        class API:
            def request(self, path):
                if not path.endswith("/permission"):
                    raise AssertionError("PR should not be retrieved")
                return {"permission": "read"}

        self.assertIsNone(github_review.select_request(API(), event()))

    def test_other_events_and_reruns_cannot_authorize_review(self):
        for name, attempt in (("push", "1"), ("pull_request", "1"), ("issue_comment", "2")):
            with (
                self.subTest(name=name, attempt=attempt),
                patch.dict(os.environ, {"GITHUB_EVENT_NAME": name, "GITHUB_RUN_ATTEMPT": attempt}),
                patch.object(github_review.GitHub, "request") as request,
            ):
                self.assertIsNone(
                    github_review.select_request(object.__new__(github_review.GitHub), event())
                )
                request.assert_not_called()

    def test_sha_and_scope_are_validated_before_publication(self):
        validate_report(report(), context())
        r = report()
        r["head_sha"] = "c" * 40
        with self.assertRaisesRegex(ValueError, "head_sha"):
            validate_report(r, context())
        r = report()
        r["skills"].append("golang-how-to")
        with self.assertRaisesRegex(ValueError, "forbidden"):
            validate_report(r, context())

    def test_ml_go_conflict_produces_incomplete_without_runner(self):
        c = context()
        c["files"].append("api/main.go")
        with tempfile.TemporaryDirectory() as folder:
            previous = Path.cwd()
            try:
                os.chdir(folder)
                Path("context.json").write_text(json.dumps(c))
                with patch.object(
                    github_review,
                    "invoke_json",
                    side_effect=AssertionError("Runner must not execute"),
                ):
                    github_review.invoke()
                r = json.loads(Path("report.json").read_text())["report"]
                validate_report(r, c)
                self.assertEqual(r["status"], "incomplete")
                self.assertEqual(r["skills"], [])
            finally:
                os.chdir(previous)

    def test_tar_traversal_and_links_never_escape_workspace(self):
        for name, kind in (("root/../../escape", tarfile.REGTYPE), ("root/link", tarfile.SYMTYPE)):
            data = io.BytesIO()
            with tarfile.open(fileobj=data, mode="w:gz") as archive:
                member = tarfile.TarInfo(name)
                member.type = kind
                member.linkname = "/etc/passwd"
                archive.addfile(member)
            with tempfile.TemporaryDirectory() as folder, self.assertRaises(ValueError):
                ssh_review.extract_snapshot(data.getvalue(), Path(folder))

    def test_push_during_review_discards_without_posting(self):
        class API:
            writes = 0

            def request(self, path, body=None):
                if body is not None:
                    self.writes += 1
                    raise AssertionError("Stale review must not post")
                if path.endswith("/permission"):
                    return {"permission": "write"}
                return {
                    "state": "open",
                    "number": 2,
                    "title": context()["title"],
                    "head": {"sha": "c" * 40, "repo": {"full_name": REPOSITORY}},
                    "base": {"sha": "b" * 40},
                }

        with tempfile.TemporaryDirectory() as folder:
            previous = Path.cwd()
            try:
                os.chdir(folder)
                Path("report.json").write_text(
                    json.dumps({"context": context(), "report": report()})
                )
                api = API()
                with (
                    patch.object(github_review, "GitHub", return_value=api),
                    patch.object(github_review, "event", return_value=event()),
                    patch.object(github_review, "update_status") as status,
                ):
                    github_review.publish()
                self.assertEqual(api.writes, 0)
                self.assertIn("Результат ревью устарел", status.call_args.args[2])
            finally:
                os.chdir(previous)


if __name__ == "__main__":
    unittest.main()
