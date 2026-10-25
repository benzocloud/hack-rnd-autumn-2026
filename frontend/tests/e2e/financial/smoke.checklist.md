# Browser smoke checklist

| Step | Action | Evidence | Status |
| --- | --- | --- | --- |
| 1 | Open frontend origin (isolated) | screenshot `01-login.png` | todo |
| 2 | Register allowlisted `.test` user | no secrets in shot | todo |
| 3 | Verify email (demo code) | no auto-login | todo |
| 4 | Login | CSRF cookie path | todo |
| 5 | Create/select company | company id in URL/UI | todo |
| 6 | Upload `ordinary_90d_c001.csv` | preview issues visible | todo |
| 7 | Ack + commit | ledger rows | todo |
| 8 | Create forecast, wait 30 points | chart + table | todo |
| 9 | Reload | same result | todo |
| 10 | Timestamp + source SHAs in report | verification doc | todo |

Bind each completed row to API/frontend/candidate SHAs in
`docs/verification/financial-checkpoint-2026-10-25.md`.
