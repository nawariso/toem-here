# ADR-007: Wildlife Domain Boundaries

Status: Proposed (Requirement 002 — pending independent review)

## Context

Requirement 002 introduces the first wildlife concepts. They must stay inside the modular monolith (ADR-001) without leaking persistence or identity-provider details into domain code, and without pre-building future features (Identification, Media, Re-ID).

## Decision

Four domain packages, each owning its tables:

| Module | Package | Tables |
| --- | --- | --- |
| Park (parks + zones) | `internal/domain/park` | `parks`, `zones` |
| Hia | `internal/domain/hia` | `hias` |
| Encounter | `internal/domain/encounter` | `encounters` |
| Location | `internal/domain/location` | `encounter_locations` (written only through the encounter repository) |

- Domain packages import neither pgx nor HTTP. Application ports: `ParkRepository`, `HiaRepository`, `EncounterRepository`, `CurrentUserResolver`.
- The encounter module never reads `auth_identities`. It resolves the current internal user through `CurrentUserResolver` (the Requirement 001 user boundary) and uses only the internal user ID and status.
- **Encounter has no `hiaId`.** Which Hia an encounter shows is an Identification/Verification decision with its own evidence, reviewers, and history; it will own that relationship.
- Protected writes require `User.CanWrite()` (`ACTIVE` only).
- Hias are read-only through the API in this requirement; there is no create/update/merge endpoint.

## Alternatives Considered

- **One `wildlife` package:** fewer files, but park reference data, individual identity, and user observations change for different reasons.
- **`hiaId` on Encounter now:** convenient but makes an unverified guess authoritative and would have to be migrated away later.

## Consequences

Cross-module reads happen in application services (for example `EncounterService` enriches responses with park/zone names), not through SQL joins in domain code.

## Exit Strategy

Each module can be extracted behind its repository port. Adding Identification later adds a new module and table referencing `encounters` and `hias`; neither existing table needs a new column.
