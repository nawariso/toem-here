# Requirement 002 — Wildlife Domain Foundation

Status: **ACCEPTED** — independent review PASS WITH MINOR ISSUES at `9e8822b750169b075f88776680ebb5bd1402cc81`; GitHub Actions `quality` run 36239981201 (pull_request) green; merged to `main` via PR #1 (merge commit `9c788aa8723e5f23c54cc8a58e55fcf1e6ebcf02`, reviewed commit preserved in history).

| Gate | Status |
| --- | --- |
| REQ-002 Wildlife Domain Foundation | ACCEPTED |
| Native local-auth smoke (device/simulator) | NOT RUN — mandatory gate before Requirement 003 |
| Supabase Auth / Email OTP / real provider JWT | DEFERRED TO INTEGRATION & PILOT HARDENING |
| Requirement 003 | NOT STARTED — blocked until native local-auth smoke = PASS |

Baseline: accepted Requirement 001 + 001-B at `ea3673bcd848db1de791453f102fe2a5fcb23034`.

## Scope

The first wildlife domain: parks, zones, individual monitor lizards (Hias) with stable public identity, user-submitted encounters, and private precise encounter locations. Persistence is PostgreSQL + PostGIS. Development and E2E tests use Controlled Local Development Authentication (Requirement 001-B); no external SaaS is required.

Out of scope (not built): Camera, MediaStore, Re-ID, pgvector, cloud object storage, maps, gamification, HiaDex, notifications, hia create/update/merge APIs, Identification/Verification, Requirement 003.

## Review amendments applied

| # | Amendment | Where |
| --- | --- | --- |
| 1 | Local auth for development and E2E | `internal/e2e/wildlife_e2e_test.go` uses `AUTH_MODE=local` via `identity.FromConfig` |
| 2 | No Supabase / email / OTP / cloud / maps / SaaS | No new dependency; `go.mod` unchanged |
| 3 | PostgreSQL + PostGIS mandatory | `postgis/postgis:18-3.6-alpine@sha256:ffcf0c4b…` in Compose and CI (ADR-006) |
| 4 | Server-derived internal user | `EncounterService` resolves the user through `CurrentUserResolver`; request DTOs have no observer field |
| 5 | `User.status` enforcement | `domain.User.CanWrite()` — only `ACTIVE`; `SUSPENDED`/`DELETED` get `403 USER_NOT_ACTIVE` on create/update/submit; reads stay allowed |
| 6 | Private/public location separation | `encounter_locations` table; response DTOs have no coordinate field (ADR-008) |
| 7 | No authoritative `hiaId` on Encounter | No column, no field; tested at the schema level |
| 8 | No Camera/Media/Re-ID/pgvector/storage/maps/gamification/HiaDex/notifications | none added |
| 9 | $0/month mandatory runtime cost | PostGIS runs in the existing local Docker container |
| 10 | Do not begin Requirement 003 | not started |

## Versions

- PostgreSQL **18.6**, PostGIS **3.6.4** — image `postgis/postgis:18-3.6-alpine` pinned by index digest `sha256:ffcf0c4b904e41b9779f8098007fb5a9484025319c18c70cf8e1bcebb742b9b7`, identical in `infra/docker/compose.yaml` and `.github/workflows/quality.yml`.
- `CREATE EXTENSION IF NOT EXISTS postgis WITH SCHEMA public` in migration `000002`.

## Domain model

| Entity | Module | Key rules |
| --- | --- | --- |
| Park | `domain/park` | slug unique case-insensitively; `countryCode` ISO-3166 alpha-2; `timezone` must be a real IANA name (no Bangkok default; `""`/`Local` rejected); status `ACTIVE`/`INACTIVE`/`ARCHIVED` |
| Zone | `domain/park` | belongs to one park; slug unique per park (same slug allowed in different parks); optional `MultiPolygon` SRID 4326 boundary |
| Hia | `domain/hia` | internal UUID never exposed; `publicCode` `HIA-000001` server-generated, unique, immutable; status `PROVISIONAL`/`CONFIRMED`/`INACTIVE`/`ARCHIVED`/`MERGED`; `MERGED` ⇔ `mergedIntoHiaId` set, never itself |
| Encounter | `domain/encounter` | observer = authenticated internal user; `capturedAt` (when seen) is distinct from `createdAt` (when recorded); zone requires park and must belong to it; behavior enum; notes ≤ 2000 chars; no hia reference |
| Private location | `domain/location` | WGS 84 lat [-90, 90], lon [-180, 180], finite; accuracy ≥ 0; source `GPS`/`MANUAL`/`IMPORT`/`UNKNOWN` |

Encounter status set: `DRAFT`, `SUBMITTED`, `PROCESSING`, `NEEDS_REVIEW`, `CONFIRMED`, `REJECTED`. Only `DRAFT → SUBMITTED` is reachable in this requirement. Submitting an already `SUBMITTED` encounter is a deterministic no-op that returns the same `submittedAt`/`updatedAt`. There is no API path back to `DRAFT`. Only `DRAFT` is editable (`409 ENCOUNTER_NOT_EDITABLE`).

## API

Public (no authentication):

- `GET /v1/parks`, `GET /v1/parks/{id}`, `GET /v1/parks/{id}/zones` — ACTIVE parks/zones only; inactive → `404 PARK_NOT_FOUND`
- `GET /v1/hias[?parkId=]`, `GET /v1/hias/{publicCode}` — read-only; no create/update/merge endpoint exists (`405`)

Authenticated (owner-only; another user's encounter is indistinguishable from a missing one: `404 ENCOUNTER_NOT_FOUND`):

- `POST /v1/encounters` → `201`
- `GET /v1/encounters/{id}`
- `PATCH /v1/encounters/{id}` — absent = unchanged, `null` clears; `capturedAt` cannot be null
- `POST /v1/encounters/{id}/submit` — no request body
- `GET /v1/users/me/encounters` — newest `capturedAt` first

Request bodies are decoded with unknown fields rejected; `observerUserId`, `userId`, `status`, `hiaId`, `id`, `submittedAt`, `createdAt`, `updatedAt` all return `400`. Contract: `packages/contracts/openapi.yaml` (v0.2.0) and `packages/contracts/src/index.ts`.

## Data design

Migration `000002_wildlife` (Requirement 001's `000001` is untouched). Every `*_at` column is `TIMESTAMPTZ`; the API emits RFC 3339 UTC.

- `parks` — `lower(slug)` unique index; format/length/status checks.
- `zones` — FK to park; `(park_id, lower(slug))` unique; `UNIQUE (id, park_id)` as the target of the encounter composite FK; GiST index on `boundary`; `ST_IsValid` check.
- `hias` — `public_number BIGINT GENERATED ALWAYS AS IDENTITY`; `public_code` a stored generated column; both unique; a `BEFORE UPDATE` trigger rejects any change to `id`/`public_number`; self-FK for merges with not-self and status/target consistency checks.
- `encounters` — FK to `users`; composite FK `(zone_id, park_id) → zones(id, park_id)` so a zone from another park is impossible at the database level; status/behavior/notes checks; `(status = 'DRAFT') = (submitted_at IS NULL)`.
- `encounter_locations` — PK/FK `encounter_id` (`ON DELETE CASCADE`); `point geography(Point, 4326)`; accuracy/source checks. Writes go through `encounter_location_point(lon, lat)`, which raises on out-of-range or NaN input because a plain `::geography` cast silently **coerces** out-of-range points into range (verified: latitude 95 is stored as 85).

Down migration `000002` drops only wildlife objects (schema-qualified) and leaves the PostGIS extension and identity tables in place. The migrator supports `up`, `down`, and `down-to <version>`. It keeps no version table: every up script is idempotent (`IF NOT EXISTS` / guarded `DO` blocks), so `up` re-applies safely, and each down script reverts only its own objects.

## Development seed

`go run ./services/api/cmd/seed` inserts Lumpini Park (`lumpini-park`, Bangkok, `TH`, `Asia/Bangkok`) and zones Lake Zone, North Path, South Pond. It is idempotent (insert-if-absent by slug; never modifies an existing row), refuses any `APP_ENV` other than `development`/`test`, and creates **no Hias**. Synthetic Hias exist only in tests.

## Logging

Encounter events (`encounter_created`, `encounter_updated`, `encounter_submitted`) log only `request_id`, `encounter_id`, `user_id`, `park_id`, `status`. Never coordinates, notes, tokens, or email. Unknown errors log `error_type=internal` without the driver message.

## Test map

| Requirement | Test |
| --- | --- |
| §45 Park / Zone | `domain/park/park_test.go`; `TestParkAndZoneConstraints` |
| §45 Hia | `domain/hia/hia_test.go`; `TestHiaPublicCodesAreSequentialUniqueAndImmutable` |
| §45 Encounter + transitions | `domain/encounter/encounter_test.go`; `application/wildlife_test.go` |
| §20 / amendment 5 ACTIVE-only | `domain/user_status_test.go`; `TestNonActiveUsersCannotWrite`; `TestNonActiveUsersCannotWriteEncounters` (E2E, SUSPENDED + DELETED) |
| §37 001 → 002 up → down → up, identity intact | `TestWildlifeMigrationUpgradesAndRollsBackWithoutTouchingIdentity` |
| §46 spatial insert/read, SRID, `ST_DWithin` | `TestEncounterWithPrivateLocationRoundTrips` |
| §38 DB constraints | `TestEncounterDatabaseConstraints`, `TestParkAndZoneConstraints` |
| §46 concurrent publicCode | `TestConcurrentHiaCreationNeverDuplicatesPublicCodes` (40 goroutines) |
| Concurrent submit | `TestConcurrentSubmitTransitionsExactlyOnce` (8 goroutines, exactly one transition) |
| §34 seed | `TestDevelopmentSeedIsIdempotentAndRefusesProduction` |
| §47 lifecycle E2E (local auth) | `TestWildlifeEncounterLifecycleWithLocalAuth` |
| §47 ownership E2E | `TestEncounterOwnershipAcrossUsers` (two real internal users via the offline JWKS stack) |
| §25/§26 MANDATORY no coordinates in responses; §48/§49 logs | `TestPublicResponsesAndLogsNeverContainPreciseLocation` |
| §48 mass assignment | `TestEncounterRequestsRejectServerControlledFields` |
| Validation / auth errors | `TestEncounterValidationErrors` |
| §32/§33 public reads, no hia writes | `TestPublicParkAndHiaReads` |

## Accepted decisions, future requirements, and hardening debt

Recorded at acceptance. None of these required a change to Requirement 002.

1. **Pagination (future requirement).** List endpoints are unpaginated. This is accepted for Requirement 002; pagination must be designed before public pilot or scaled HiaDex usage.
2. **Hia public code gaps (accepted).** `HIA-xxxxxx` codes come from a database sequence, and a rolled-back insert consumes a value, so gaps are expected.
   - uniqueness is mandatory
   - immutability is mandatory
   - reuse is forbidden
   - gap-free allocation is not required and must not be attempted
3. **Migration version tracking (mandatory hardening).** The current idempotent migration runner (no version table) is accepted for local development. Trustworthy migration history/version tracking becomes mandatory before the first shared or deployed persistent database.
4. **Precise-location write boundary (mandatory hardening).** `encounter_location_point()` is the controlled application/database write boundary for precise coordinates. Privileged arbitrary raw SQL is outside that invariant boundary; production database permissions/access controls must prevent uncontrolled wildlife-location writes.
5. **Public Hia visibility (accepted for the foundation).** Public Hia reads include every lifecycle status (`PROVISIONAL`, `CONFIRMED`, `INACTIVE`, `ARCHIVED`, `MERGED`) with `status` exposed. HiaDex product visibility rules will be defined later; historical and merged identities must remain resolvable.

## Known limitations

- Mobile has contract types only; no wildlife screens (Requirement 002 §44).
- Native local-auth smoke remains **NOT RUN** — deferred mandatory gate before Requirement 003 (see 001-B).
- Supabase Auth / Email OTP / real provider JWT remain **DEFERRED TO INTEGRATION & PILOT HARDENING**.
