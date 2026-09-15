# ADR-002: PostgreSQL as System of Record

Status: Accepted

## Context
Identity creation needs transactions, foreign keys, UUIDs, case-insensitive uniqueness, and production-equivalent integration tests at zero local cost.

## Decision
Use PostgreSQL 18.6 as the system of record and local Docker Compose database. Use versioned SQL migrations and lightweight pgx/v5 behind repository interfaces. The first-login write uses one transaction.

## Alternatives Considered
- SQLite: rejected because its concurrency, types, and uniqueness behavior are not a PostgreSQL substitute.
- A large ORM: rejected because three tables do not justify its abstraction and migration cost.
- Managed PostgreSQL now: rejected because it creates mandatory cost before product proof.

## Consequences
Developers need Docker or PostgreSQL. SQL remains explicit and PostgreSQL integration tests prove actual constraints. Domain code does not import pgx.

## Exit Strategy
Repository ports permit another persistence adapter. A future managed PostgreSQL service can use the same schema and migrations without changing the domain.
