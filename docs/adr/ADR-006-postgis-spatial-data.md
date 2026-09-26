# ADR-006: PostGIS for Spatial Data

Status: Accepted (Requirement 002, reviewed at `9e8822b`)

## Context

Encounters carry a precise location, zones may carry a boundary polygon, and later requirements will ask spatial questions (which zone contains a point, which encounters are near each other). PostgreSQL is already the system of record (ADR-002). Mandatory runtime cost must stay at $0/month (ADR-004).

## Decision

Use the PostGIS extension inside the existing PostgreSQL instance.

- Image `postgis/postgis:18-3.6-alpine`, pinned by index digest `sha256:ffcf0c4b904e41b9779f8098007fb5a9484025319c18c70cf8e1bcebb742b9b7` (PostgreSQL 18.6, PostGIS 3.6.4), identical in `infra/docker/compose.yaml` and the CI service container.
- `CREATE EXTENSION IF NOT EXISTS postgis WITH SCHEMA public`; spatial types are schema-qualified (`public.geography`) so isolated test schemas resolve them.
- Precise points: `geography(Point, 4326)` — metres-based distance functions without projection choices.
- Zone boundaries: `geometry(MultiPolygon, 4326)` with a GiST index and an `ST_IsValid` check.
- The extension is shared database infrastructure; the down migration does not drop it.

## Alternatives Considered

- **Plain `latitude`/`longitude` numeric columns:** simple, but every spatial query would be hand-written and unindexed.
- **Separate spatial service / SaaS geocoding:** adds cost and an external dependency for no current need.
- **PostGIS (chosen):** same database, same transactions and constraints, $0.

## Consequences

The local and CI images are PostGIS images rather than stock PostgreSQL. A `::geography` cast coerces out-of-range coordinates into range instead of failing, so writes go through `encounter_location_point()`, which validates first (see ADR-008).

## Exit Strategy

The point is stored in its own table behind `EncounterRepository`. Moving to numeric columns or another store changes one repository and one migration; the domain and API do not change.
