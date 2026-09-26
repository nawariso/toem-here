# ADR-008: Private vs Public Location

Status: Proposed (Requirement 002 — pending independent review)

## Context

A precise encounter location reveals where a person was at a given time and where a wild animal can be found. Both are sensitive. Public experiences only need park- or zone-level place.

## Decision

- Precise location lives only in `encounter_locations` (1:1 with an encounter), never on the `encounters` row.
- Location is **write-only** through the API. Encounter responses contain `park {id, name}` and `zone {id, name}` only; the response structs have no coordinate, point, or accuracy field, so precise location cannot be serialised by accident.
- Logs never contain coordinates, notes, tokens, or email. Validation messages may name a field (`longitude must be between -180 and 180`) but never echo a value.
- Coordinates are validated in the domain (finite, lat [-90, 90], lon [-180, 180], accuracy ≥ 0) and again in the database: writes go through `encounter_location_point(lon, lat)`, which raises on out-of-range or NaN input because PostGIS silently coerces out-of-range `::geography` casts into range.

## Alternatives Considered

- **Coordinates on the encounter row with response filtering:** one careless `SELECT *`/DTO change leaks them.
- **Rounded public coordinates now:** no current consumer; a later requirement can add a deliberate, reviewed public granularity.

## Consequences

A mandatory E2E test stores a GPS point, confirms it is in the database, and asserts that no response (create, patch, get, list, submit, parks, zones, hias, validation error) and no log line contains it.

## Exit Strategy

Access to the private table is isolated in `EncounterRepository`. Adding a public, coarsened representation later is a new field computed server-side, not a change to the private table.
