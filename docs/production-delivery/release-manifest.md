# Staging release manifest · 2026-11-11

**NOT READY for production promotion.** This manifest pins the staging build; later verification and documentation commits do not change the deployed application.

| Item | Recorded value |
|---|---|
| Candidate | `1292f8f5dc30b16e458723ba05a50029dbecb814` on `codex/controlled-beta-release-20261111` |
| Staging API | `b9e005dd-d6b1-4afc-b8ae-4627cd25a326`, `sha256:be227f9226f747b7dfb3b9ed737fa3c7c963969019460cd54b655b055c4e5df7` |
| Staging Admin | `1bb9dd0a-34d5-4421-870b-7bfee9f0eb38`, `sha256:032883d508690f971c533fae8f3dccc1620c9b6dd2c7aeff324cbadde3c4be96` |
| Database | All 32 migrations present in staging. Two simultaneous runners applied all 32 on a disposable clean PostgreSQL 17 instance without pgvector. |
| Staging policy | Controlled beta, designated tester only, signed WhatsApp requests, Paystack Test Mode, simulated pickup. Existing phone-specific staging override verified; CRM app callback unchanged. |
| Rollback API | `146a1589-ef41-4be9-a23a-77ac0819cbc0`, `sha256:ae88b7da8ff2bd5e43b0754a13c1d1168f922a5325167c5ce4aeb4f6e997390c` |
| Rollback Admin | `0cbe74ff-1743-4582-ac64-0de84f816c04`, `sha256:677686e56c0cf79032d026b07c999519daab9429f169e4c19bbd7aea9071b181` |

To roll back the application, select the two recorded prior deployments in the **staging** Railway environment and roll back API and Admin as a pair. Verify API and DB health, Admin login and a read-only order lookup; then verify the tester filter and the existing webhook route. The two releases share the same applied schema. Restore the database only for a separate data recovery incident, first to a separate database for validation. On November 11 the exact Railway Rollback operation activated both pinned digests (API `22100a21-0f52-4fc4-905d-1a8ecf978ad2` and Admin `20abacfd-9a56-4ec5-b3d9-e1ba8df22a75`). API and database health, Admin login and authenticated order lookup passed. Staging then returned to the prior candidate. The notification fix in this table was deployed after that rehearsal and passed API and database health. Use Rollback for exact artifact recovery. Railway Redeploy rebuilt the API and produced a different digest.

The staging backup restored successfully to a disposable database. The private backup and its checksum were verified; it was not uploaded to Git. Backup retention and failure alerting still need verification. See [remaining gates](remaining-gates.md) and [test evidence](test-evidence.md). The owner authorized promotion after every gate passes. The current verdict keeps promotion on hold.


The staging notification acceptance order is `ZC-20261111-897BB040`. All four updates were delivered. The subsequent closed-window probe and timestamp restoration required no application build. Public staging preflight passed API, database and Admin health. See [evidence](test-evidence.md).
