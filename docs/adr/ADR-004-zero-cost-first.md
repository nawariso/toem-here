# ADR-004: Zero-Cost-First Development

Status: Accepted

## Context
The product has not proved traffic or revenue. Premature hosted infrastructure creates fixed cost and operational surface.

## Decision
Mandatory infrastructure cost is $0/month: local Docker PostgreSQL, local Go API, local Expo tooling, GitHub Actions free allowance, and Supabase Auth Free Tier. No Redis, Kafka, Kubernetes, paid monitoring, managed database, storage, or AI infrastructure is introduced.

## Alternatives Considered
- Managed database/API hosting now: rejected as unproven recurring cost.
- Full local Supabase stack: deferred because hosted Free Tier is the selected initial identity provider and local PostgreSQL remains the domain system of record.
- Kubernetes: rejected as unjustified complexity.

## Consequences
Developers configure one free Supabase project and run data services locally. CI provides deterministic checks without a deployed environment. Hosted uptime and centralized telemetry are intentionally absent.

## Exit Strategy
Adopt paid services only when measured reliability, capacity, or team needs justify them. Put provider additions behind existing identity, persistence, and telemetry ports and record a new ADR.

## Amendment — Requirement 001-B
Local development no longer requires any hosted service. With `AUTH_MODE=local` the mandatory development runtime is local Docker PostgreSQL, the local Go API, and local Expo tooling ($0/month, no accounts). Supabase Auth Free Tier remains the selected identity provider for the Integration & Pilot Hardening milestone. See ADR-005.
