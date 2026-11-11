# Staging release manifest · 2026-11-11

**NOT READY for production promotion.** This manifest pins the staging build; later documentation commits do not change the deployed application.

| Item | Recorded value |
|---|---|
| Candidate | `eda0375e08a293487a8f253294bae53fb1db03b6` on `codex/controlled-beta-release-20261111` |
| Staging API | `bfa7c90b-d8eb-4221-89ff-7b79c257060a`, `sha256:8cc6bfd23148fab9c815439c3273e7cdd84d06ae78d6bda9634706254b10deb9` |
| Staging Admin | `2b627848-f713-4f82-87b5-0c536b548fd6`, `sha256:032883d508690f971c533fae8f3dccc1620c9b6dd2c7aeff324cbadde3c4be96` |
| Database | All 32 migrations present in staging. Two simultaneous runners applied all 32 on a disposable clean PostgreSQL 17 instance without pgvector. |
| Staging policy | Controlled beta, designated tester only, signed WhatsApp requests, Paystack Test Mode, simulated pickup. Existing phone-specific staging override verified; CRM app callback unchanged. |
| Rollback API | `146a1589-ef41-4be9-a23a-77ac0819cbc0`, `sha256:ae88b7da8ff2bd5e43b0754a13c1d1168f922a5325167c5ce4aeb4f6e997390c` |
| Rollback Admin | `0cbe74ff-1743-4582-ac64-0de84f816c04`, `sha256:677686e56c0cf79032d026b07c999519daab9429f169e4c19bbd7aea9071b181` |

To roll back the application, select the two recorded prior deployments in the **staging** Railway environment and roll back API and Admin as a pair. Verify API and DB health, Admin login and a read-only order lookup; then verify the tester filter and the existing webhook route. The two releases share the same applied schema. Restore the database only for a separate data recovery incident, first to a separate database for validation. No live rollback was rehearsed.

The staging backup restored successfully to a disposable database. The private backup and its checksum were verified; it was not uploaded to Git. Backup retention and failure alerting still need verification. See [remaining gates](remaining-gates.md) and [test evidence](test-evidence.md). No production authorization or staging-to-production promotion was performed.
