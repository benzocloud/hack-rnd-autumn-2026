# Team access cycle — plan

Branch: `globalarray/feat/team_access_cycle`
Base: `origin/main` @ `dccf513e4145a8a32b87dbd321187726adb5ffc5`
Draft PR: pending first push

## Scope

Implement Auth members/invitations RPCs + API HTTP mapping so frontend «Команда»
works against real backend. Invitation lifetime: **7 days** (canonical
`docs/auth/contract.md` / privacy). History retention: 30 calendar days after
first terminal transition. No email delivery; accept via «Мои компании».

## Closed

- Auth migration `00004_team_invitations.sql`
- Invitation + membership SQL repos (insert/delete financier, list members, locks)
- Service: ListMembers, RemoveMember, Create/List/Accept/Decline/Revoke
- gRPC handlers + error reasons; API team routes; mTLS API allowlist already covers new RPCs
- Unit tests + disposable Postgres integration `TestTeamAccessDatabaseCycle`

## Remaining

- Self-review (caveman/ponytail/Go); merge latest main before final push
- Update draft PR body; handoff after each saved block
- No merge/deploy

## Next command

```bash
cd /tmp/saldo-team-access
git fetch origin && git merge origin/main
# then re-run focused Auth/API tests and push
```
