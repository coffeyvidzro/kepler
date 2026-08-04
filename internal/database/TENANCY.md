# Tenant isolation

Dugble applies tenant isolation in multiple layers:

1. Transport middleware authenticates an actor and builds a validated tenant access context.
2. Services authorize the requested operation and obtain the tenant ID only from that context.
3. Tenant-owned SQL queries include `team_id`; `tenant_scope_test.go` fails when a new query omits it without an explicit trusted-system exemption.
4. Database constraints protect cross-record and membership invariants independently of application checks.

## Trusted-system query exemptions

The SQL audit permits narrowly listed queries that cannot accept a caller-selected tenant. These include credential lookup by a hashed secret, invitation consumption by a hashed secret, and worker updates guarded by an acquired worker lock. New exemptions should document which immutable relationship or ownership check supplies the tenant boundary.

## Row-level security

PostgreSQL row-level security is intentionally not enabled yet. The API, worker, and backoffice currently share the same database role and connection-pool setup. Enabling policies without separating those roles would either be ineffective (through owner/bypass behavior) or break trusted cross-tenant worker jobs.

Before enabling RLS:

- provision separate API, worker, backoffice, and migration roles;
- make API repository operations transaction-bound and set a transaction-local tenant ID;
- define explicit worker/backoffice policies rather than relying on the API policy;
- verify pooled connections cannot retain tenant state between transactions;
- exercise every tenant-owned table with policy integration tests.

Until those prerequisites are met, explicit query scoping plus CI auditing and database invariants are the enforced defense-in-depth boundary.
