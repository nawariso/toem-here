# ADR-001: Modular Monolith

Status: Accepted

## Context
TOEM HERE needs clear domain boundaries but has one small team, one API, and no demonstrated need for distributed deployment.

## Decision
Build one Go process arranged as domain, application ports/use cases, infrastructure adapters, and HTTP transport. Dependencies point inward. Identity and persistence are interfaces at the application boundary.

## Alternatives Considered
- Microservices: rejected because operational and consistency costs are unearned.
- Unlayered handlers and SQL: rejected because provider/database replacement would leak across the codebase.
- Serverless functions per endpoint: rejected because it fragments transactions and local reproduction.

## Consequences
Deployment and local operation stay simple. Module boundaries require discipline and tests, but cross-module transactions remain straightforward.

## Exit Strategy
Split a module only after measured independent scaling, ownership, or reliability needs. Preserve contracts and move the existing adapter behind a network boundary.
