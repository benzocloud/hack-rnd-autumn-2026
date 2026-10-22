import copy
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent / "runner"))
from github_api import REPOSITORY, GitHub
from github_dialogue import dialogue_root, issue_references, related_pull_requests


def cross_reference(number, repository=REPOSITORY):
    return {
        "event": "cross-referenced",
        "source": {
            "issue": {
                "number": number,
                "url": f"https://api.github.com/repos/{repository}/issues/{number}",
                "pull_request": {},
            }
        },
    }


def related_pr(number=3, body="Refs #7", state="closed"):
    return {
        "number": number,
        "title": "Fix history",
        "body": body,
        "state": state,
        "head": {"sha": "c" * 40, "repo": {"full_name": REPOSITORY}},
        "base": {"sha": "a" * 40},
        "user": {"login": "dev"},
        "created_at": "2026-10-22T10:00:00Z",
    }


class DialogueTests(unittest.TestCase):
    def test_only_formal_same_repository_body_references_define_a_root(self):
        for body in (
            "Refs #7",
            "Fixes #7",
            "Resolved #7",
            "CLOSES #7",
            f"Fixes {REPOSITORY}#7",
            "Closes: #7",
            "CLOSES:#7",
            "Refs : #7",
            f"Fixes: https://github.com/{REPOSITORY}/issues/7",
            f"Ref https://github.com/{REPOSITORY}/issues/7",
            "<!-- agent-plan:issue=7 -->\nRefs #7",
        ):
            with self.subTest(body=body):
                self.assertEqual(dialogue_root({"body": body}, {}), 7)
        for body in (
            "Discussed in #7",
            "PrefixRefs #7",
            "Refs #7abc",
            "Refs#7",
            "Closes:: #7",
            "Refs https://github.com/other/repo/issues/7",
            "Fixes other/repo#7",
            f"https://github.com/{REPOSITORY}/issues/7",
        ):
            with self.subTest(body=body):
                self.assertIsNone(dialogue_root({"body": body}, {}))
        self.assertEqual(dialogue_root({"number": 2, "body": "Refs #7"}, None), 2)

    def test_multiple_distinct_roots_are_rejected_even_with_one_marker(self):
        for body in (
            "Refs #7\nFixes #8",
            "Refs #7\nCloses: #8",
            f"Refs #7\nCloses {REPOSITORY}#8",
            "Refs: #7, #8",
            "<!-- agent-plan:issue=7 -->\nFixes: #8",
            "Refs #7, #8",
            "Refs #7 and #8",
            "<!-- agent-plan:issue=7 -->\nRefs #8",
        ):
            with self.subTest(body=body), self.assertRaisesRegex(ValueError, "Ambiguous"):
                dialogue_root({"body": body}, {})
        self.assertEqual(issue_references("Refs #7, #7"), {7})

    def test_timeline_candidates_require_current_body_and_same_repository(self):
        from unittest.mock import Mock

        api = Mock()
        api.pages.side_effect = lambda path: copy.deepcopy(
            {
                "/issues/7/timeline": [
                    cross_reference(2),
                    cross_reference(3),
                    cross_reference(3),
                    cross_reference(4),
                    cross_reference(5),
                    cross_reference(6, "other/repo"),
                    {"event": "commented"},
                ],
                "/issues/3/comments": [
                    {"id": 31, "body": "Prior question", "user": {"login": "dev"}}
                ],
                "/pulls/3/reviews": [{"id": 32, "state": "CHANGES_REQUESTED"}],
                "/pulls/3/comments": [
                    {"id": 33, "in_reply_to_id": 30, "path": "api/a.py", "line": 4}
                ],
            }[path]
        )
        foreign = related_pr(5)
        foreign["head"]["repo"]["full_name"] = "other/repo"
        api.request.side_effect = lambda path: copy.deepcopy(
            {
                "/pulls/3": related_pr(body=f"Fixes {REPOSITORY}#7"),
                "/pulls/4": related_pr(4, "Mention #7"),
                "/pulls/5": foreign,
            }[path]
        )
        threads = related_pull_requests(api, 7, 2)
        self.assertEqual([p["task"]["number"] for p in threads], [3])
        self.assertEqual(threads[0]["task"]["state"], "closed")
        self.assertEqual(threads[0]["task"]["user"]["login"], "dev")
        self.assertEqual(threads[0]["head_sha"], "c" * 40)
        self.assertEqual(threads[0]["pr_review_comments"][0]["in_reply_to_id"], 30)
        self.assertEqual(api.request.call_count, 3)
        foreign["head"]["repo"] = None
        self.assertEqual(len(related_pull_requests(api, 7, 2)), 1)

    def test_incomplete_unavailable_ambiguous_or_oversized_history_is_not_silenced(self):
        from unittest.mock import Mock

        api = Mock()
        api.pages.side_effect = ValueError("Pagination failed")
        with self.assertRaisesRegex(ValueError, "Pagination"):
            related_pull_requests(api, 7, 2)
        api.pages.side_effect = None
        api.pages.return_value = [cross_reference(n) for n in range(10, 21)]
        with self.assertRaisesRegex(ValueError, "Too many"):
            related_pull_requests(api, 7, 2)
        api.pages.return_value = [cross_reference(3)]
        api.request.side_effect = RuntimeError("Unavailable")
        with self.assertRaisesRegex(RuntimeError, "Unavailable"):
            related_pull_requests(api, 7, 2)
        api.request.side_effect = None
        api.request.return_value = related_pr(body="Refs #7, #8")
        with self.assertRaisesRegex(ValueError, "Ambiguous"):
            related_pull_requests(api, 7, 2)

    def test_timeline_pagination_uses_all_pages_and_never_returns_partial_history(self):
        from unittest.mock import Mock

        api = object.__new__(GitHub)
        api.request = Mock(side_effect=[[{"event": "commented"}] * 100, [cross_reference(3)]])
        self.assertEqual(len(api.pages("/issues/7/timeline")), 101)
        self.assertIn("page=2", api.request.call_args.args[0])
        api.request.side_effect = None
        api.request.return_value = [{"event": "commented"}] * 100
        with self.assertRaisesRegex(ValueError, "pagination limit"):
            api.pages("/issues/7/timeline")
