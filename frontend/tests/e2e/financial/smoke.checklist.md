# Browser smoke checklist

| Step | Action | Evidence | Status |
| --- | --- | --- | --- |
| 1 | Open frontend origin (isolated) | screenshot `01-login.png` | blocked:no_playwright_manifest |
| 2 | Register allowlisted `.test` user | no secrets in shot | blocked:no_playwright_manifest |
| 3 | Verify email (demo code) | no auto-login | blocked:no_playwright_manifest |
| 4 | Login | CSRF cookie path | blocked:no_playwright_manifest |
| 5 | Create/select company | company id in URL/UI | blocked:no_playwright_manifest |
| 6 | Upload `ordinary_90d_c001.csv` | preview issues visible | blocked:no_playwright_manifest |
| 7 | Ack + commit | ledger rows | blocked:no_playwright_manifest |
| 8 | Create forecast, wait 30 points | chart + table | blocked:no_playwright_manifest |
| 9 | Reload | same result | blocked:no_playwright_manifest |
| 10 | Timestamp + source SHAs in report | verification doc | HTTP SHA table updated; browser pending B |

Bind each completed row to API/frontend/candidate SHAs in
`docs/verification/financial-checkpoint-2026-10-25.md`.
