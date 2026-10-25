# Handoff: team access cycle

Updated: 2026-10-25

## Branch / PR

- Worktree: `/tmp/saldo-team-access`
- Branch: `globalarray/feat/team_access_cycle`
- Source SHA at start: `dccf513e4145a8a32b87dbd321187726adb5ffc5`
- Draft PR: not published yet (first commit/push in progress)

## Closed in this block

1. Auth owns members/invitations:
   - Migration `services/auth/migrations/00004_team_invitations.sql`
   - Domain `invitation.go`; repo `internal/repository/invitation/`
   - Membership: insert/delete financier, list company members, company lock
   - Service `team.go`: list/remove members; create/list/accept/decline/revoke
   - Pending lifetime **7 days**; terminal history ≤30 days; role always financier
   - Accept idempotency does not restore removed membership (`MEMBERSHIP_REVOKED`)
   - Company name snapshot accepted from API (Core-read under owner), not trusted as authz
2. gRPC `team.go` + error mapping for invitation/membership reasons
3. API HTTP `team.go`: `/companies/{id}/members|invitations`, `/me/invitations` (+ accept/decline)
   - CreateInvitation: Core `ReadCompany` → Auth CreateInvitation
   - AcceptInvitation: Auth Accept → Core `ReadCompany` → Company DTO
4. Checks run:
   - `golangci-lint` **v2.15.0** on Auth touched packages + API `handler/http`: 0 issues
   - `go test -race` Auth account+grpc; API handler/http (502)
   - Integration on disposable Postgres `127.0.0.1:58432/auth_test`:
     `TestTeamAccessDatabaseCycle` (owner/financier/stranger, CSRF, idempotency,
     revoke/decline/expiry, remove + accept replay, app restart)

## Schema

- `invitations` table; unique pending `(company_id, email)`; create idempotency unique
- Membership PK unchanged; financier insert `ON CONFLICT DO NOTHING`

## Not done / limits

- Full monorepo CI / release not run (owner forbid)
- Latest `origin/main` merge still required before final push
- No production deploy; no email sender
- Frontend unchanged (already calls real REST)

## Next exact step

```bash
cd /tmp/saldo-team-access
# after commit+push+draft PR:
git fetch origin && git merge origin/main
# resolve conflicts keeping Scenarios/Comparisons and team routes
AUTH_TEST_DATABASE_URL='postgres://saldo:saldo@127.0.0.1:58432/auth_test?sslmode=disable' \
  go test -tags=integration -race -count=1 ./services/auth/integration/ -run TestTeamAccessDatabaseCycle
```
