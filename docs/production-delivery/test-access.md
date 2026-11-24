# Test access

No passwords or tokens are stored in this repository. Each login below points to a local, owner-only (chmod 600) file that holds the password.

## Staging (Railway project `zidicommerce-staging`, environment `staging`)

| Item | Value |
|---|---|
| Admin | https://zidicommerce-staging-admin-staging.up.railway.app/login |
| API health | https://zidicommerce-staging-api-staging.up.railway.app/health |
| Owner login | `staging.owner@zidicommerce.test` |
| Password file | `/Users/oluwateniolasobande/.zidi-staging-login.YZZyMV/login.txt` |
| Organization | Bing Chun Nigeria (Staging) |
| Second organization (not Bing Chun) | slug `zidi-commerce-staging-qa`; confirm owner login before use |
| WhatsApp test number | ZidiHQ business number **+234 808 510 5382** (phone number ID `683059678235387`, WABA `773193152076286`) |
| Designated tester | number ending **2209** (full list in Railway variable `WHATSAPP_TESTER_NUMBERS`) |
| Payments | Paystack **test** key (`sk_test_*`); use Paystack's published test card only |
| Email | Resend SMTP; invitation/reset tests may go to the owner-authorised address only |

Rollback references (Railway): API `1748bc4a`, Admin `350c8d14` (pre-redesign, 18 Nov 2026).

## Local acceptance stack

```bash
cd /Users/oluwateniolasobande/Zidi/zidi-worktrees/commerce-production-v2
bash scripts/local-acceptance/start.sh
```

| Item | Value |
|---|---|
| Admin | http://127.0.0.1:4326/login |
| Field | http://127.0.0.1:4327/login |
| API | http://127.0.0.1:4325/health |
| Login file | `.local-acceptance/login.txt` (gitignored; created against the current local database) |
| Organization | Zidi Local Foods |

The local login exists only while `.local-acceptance/pg` exists. Stop with `bash scripts/local-acceptance/stop.sh`. Run the automated suite with `npm run acceptance:local`. Each run registers its own disposable organization.
