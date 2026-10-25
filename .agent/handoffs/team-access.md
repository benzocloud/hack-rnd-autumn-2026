# Handoff: team access cycle

Updated: 2026-10-25 (post-merge review)

## Branch / PR

- Worktree: `/tmp/saldo-team-access`
- Branch: `globalarray/feat/team_access_cycle`
- Draft PR: https://github.com/benzocloud/hack-rnd-autumn-2026/pull/58
- Started from: `dccf513e4145a8a32b87dbd321187726adb5ffc5`
- Merged `origin/main` through `6d2cb37` (scenarios #56/#57 kept; `router.go` auto-merged with team + scenario routes)

## Closed

1. Auth members/invitations (migration `00004`, repos, service, gRPC, errors)
2. API HTTP team routes; Core name snapshot on create; Company DTO on accept
3. Invitation lifetime **7 days**; history ≤30 days after terminal
4. Accept idempotency never restores removed membership
5. Lock order fix: Accept/Decline peek invitation → lock memberships → lock invitation
6. Checks (after main merge):
   - Auth `go test -race` account+grpc: pass
   - `TestTeamAccessDatabaseCycle` on disposable PG `:58432/auth_test`: pass
   - API `handler/http` 504 tests + golangci-lint v2.15.0: pass
7. No merge/deploy from this agent

## Schema

- `invitations` + financier membership SQL only (Auth). Core scenarios migration `00016` from main untouched.

## Remaining / limits

- Full monorepo CI not run
- Production update is for the main agent after review
- Disposable PG container `saldo-team-access-pg` can be removed when done

## Next command for pickup

```bash
cd /tmp/saldo-team-access
gh pr view 58 --json url,isDraft,mergeStateStatus
# main agent: review PR #58, then integrate/deploy — do not merge from this worktree unless owner says so
```
