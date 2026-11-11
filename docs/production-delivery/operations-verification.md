# Bing Chun operating checks

Use the staging environment for drills. Use synthetic data and the designated WhatsApp tester. A test payment must come from Paystack Test Mode. Save timestamps, deployment IDs and outcome codes. Exclude tokens, signatures, raw callback bodies, checkout URLs and pickup codes from evidence.

| Signal | What to check | What proves alerting |
|---|---|---|
| Availability | External API `/health` including database result, and Admin login page | An isolated unreachable target generates an alert at the designated inbox and a recovery notice when restored |
| Runtime | API crashes and sustained server errors | A disposable staging fault produces an alert identifying the environment and failing service |
| Queue | Age of oldest ready job, retry rate and permanent failures grouped by job type | A disposable failing job triggers the queue alert and appears in the same environment's worker logs |
| Payment | Pending paid-provider transactions and reconciliation failures | A controlled Test Mode discrepancy raises an alert and clears after successful reconciliation |
| Webhooks | Signed callback processing failures and absence of expected provider delivery | A controlled failing staging probe raises an alert. Confirm normal signed events still process |
| AI provider | Provider failures and fallback rate | Isolated provider failure raises an alert and the customer receives the safe fallback |
| Backup | Scheduled backup age, failure and restore validation | A disposable failing backup generates an alert, followed by a successful backup and separate restore |

Railway build-failure email delivery has been verified. API and Admin deployment health checks are configured. Track ongoing incident alert checks separately from build notifications and deployment health.

For an incident, preserve the relevant order, job and provider event references. Use the [pinned release manifest](release-manifest.md) for application rollback. Investigate payment status using the staging payment and event records before retrying any action. Leave the Paystack Test Mode and WhatsApp tester controls in place during recovery.

On November 11, Railway returned `BAD_USER_INPUT` when enabling the native staging backup schedule because scheduled backups require a Pro workspace. The logical backup restore passed. Before launch, verify recurring backups and failure alert delivery using a supported Railway plan or a separately operated backup and alert service. [Railway backup reference](https://docs.railway.com/volumes/backups)


The staging Railway project inventory showed no bucket and the API service exposed no backup or operator alert variables. An external option is an approved private S3 bucket for staging PostgreSQL backups with a scoped service credential delivered through the secret store. Configure retention and verify a scheduled execution, disposable restore and actual failure notification before closing that gate. Railway build-failure email remains the verified delivery path.
