# Growth insight operations

## Rollout configuration

Personal analysis is opt-in. Deploying code does not enroll existing users. The
worker also requires an explicit service-level enable switch; read, correction,
consent and withdrawal APIs remain initialized when model processing is disabled.

| Environment variable | Default | Meaning |
| --- | --- | --- |
| `GROWTH_INSIGHTS_WORKER_ENABLED` | `false` | Start background growth analysis processing |
| `GROWTH_INSIGHTS_MAX_OUTPUT_TOKENS` | `4096` | Maximum output tokens per growth model request |
| `GROWTH_INSIGHTS_DAILY_ATTEMPT_LIMIT` | `100` | Total reserved model attempts per UTC calendar day |
| `GROWTH_INSIGHTS_USER_DAILY_ATTEMPT_LIMIT` | `3` | Reserved attempts per user per UTC calendar day |

Limits must be positive integers. Zero is not an unlimited mode. Invalid explicit
configuration fails validation. All worker replicas must use the same limits.
Configuration changes require restarting all relevant server instances.

The model provider, endpoint, credentials and model continue to come from the
existing administrator model configuration. Only growth requests receive the new
output limit and a response-header timeout equal to the configured model timeout
(30 seconds when unset). Connection/TLS guards remain and the worker has an outer
four-minute processing context. Other administrator/chat callers retain their
previous request parameters and timeouts.

## Attempt accounting

Reservations are stored in `app_growth_insight_attempts` under a PostgreSQL
transaction lock shared across worker instances. An attempt is reserved only
after checking current consent, ownership, job lease and sufficient evidence,
before the provider call. Failed, timed-out and subsequently invalidated calls
still count. Admin retry, new evidence and consent withdrawal do not refund them.
Hard deletion and anonymized account deletion remove the owner association while
preserving the current global count. Ordinary disable/re-enable retains the
per-user count.

Budgets use UTC calendar days, not a rolling 24-hour window. Exhausted jobs remain
pending until the next UTC day, keep their published report visible, and do not
spend a provider retry. The existing once-per-24-hour successful generation rule,
30-minute inactivity interval and seven-day publication rule still apply.
The ledger retains the current day and six prior days while the worker runs;
future-dated rows are not deleted by this cleanup. It contains no message text.
If the worker is disabled, scheduled cleanup pauses until it resumes.

These are request/output bounds, not a currency-denominated spending ceiling.
Provider pricing and token accounting determine actual costs. Keep provider-side
billing controls and account alerts enabled. A richer operations dashboard and
incremental evidence scheduler remain separately scoped.

## Monitoring and pause

Inspect counts and pending jobs in the existing database. Restrict access to
administrators; do not export evidence or model credentials into application logs.

```sql
SELECT budget_day, count(*) AS reserved_attempts
FROM app_growth_insight_attempts
WHERE budget_day >= (now() AT TIME ZONE 'UTC')::date - 6
GROUP BY budget_day ORDER BY budget_day DESC;

SELECT status, error_code, count(*)
FROM app_growth_insight_jobs
GROUP BY status, error_code ORDER BY status, error_code;
```

Set `GROWTH_INSIGHTS_WORKER_ENABLED=false` and gracefully restart server instances
to pause model consumption. Pending jobs remain persisted. Restore `true` to
resume with the same budget ledger. Consent withdrawal is the user-facing erase
operation, not an operational pause switch.

## Release and rollback

Before release, back up business data, full schema, deployed images and Compose
configuration. Validate the additive schema twice against a database copy. Use
the existing single-server graceful drain procedure in `DEPLOY.md`; do not mix old
and new server workers. Keep original App/H5 release pipelines separate.

A binary rollback alone leaves growth tables, menus and source invalidation
triggers in place. Keeping those triggers preserves erasure semantics for reports
created while the feature was active. If a trigger itself causes a regression,
first disable the worker, then use a reviewed transaction to disable only the
affected growth triggers and mark existing reports invalid before restoring
access. Never restore an old whole-database dump over later user activity, and
never drop tables or remove privacy triggers as an unreviewed rollback shortcut.

The opt-in `TestGrowthLoopRealProviderSmoke` uses synthetic evidence in an isolated
loopback test database. It is skipped unless `GROWTH_PROVIDER_SMOKE_CONFIG` is
explicitly set. Keep that JSON out of shell output/logs; pass the latest SQL via
`GROWTH_PROVIDER_SMOKE_SCHEMA_PATH`. A smoke pass checks integration and structure,
not the quality of every future personal report.
