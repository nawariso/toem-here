# Foundation Architecture

The Go API is a modular monolith with inward dependencies:

```text
transport/http -> application -> domain
                       ^
infrastructure/identity + infrastructure/persistence
```

- Domain: internal user and validation rules; wildlife packages `domain/park`, `domain/hia`, `domain/encounter`, `domain/location` (Requirement 002, ADR-007). No framework/database/provider imports except UUID generation.
- Application: use cases and ports (`IdentityVerifier`, `UserRepository`, readiness; `ParkRepository`, `HiaRepository`, `EncounterRepository`, `CurrentUserResolver`).
- Infrastructure: pgx PostgreSQL repository, migrations, Supabase-compatible JWKS verifier, config and telemetry boundary.
- Transport: routes, Bearer authentication, stable errors, request IDs and structured request logs.

Authentication resolves `JWT -> ExternalIdentity -> AuthIdentity -> User -> Roles`. A PostgreSQL transaction and a transaction-scoped advisory lock guarantee one internal user per external identity even during concurrent first login.

## Authentication modes (Requirement 001-B, ADR-005)

`AUTH_MODE` selects exactly one `IdentityVerifier` through `identity.FromConfig`; there is no fallback.

```text
credential ──> IdentityVerifier ──> ExternalIdentity ──> AuthIdentity ──> User
               ├── LocalDevVerifier   (AUTH_MODE=local;    LOCAL_DEV / developer-001)
               └── JWT Verifier       (AUTH_MODE=supabase; SUPABASE / JWT sub)
```

Both modes share the same transport middleware, bootstrap use case, repository, and database constraints. Local mode only replaces how a bearer credential becomes an `ExternalIdentity`; it never lets the client name a user, role, or status.

**LOCAL AUTH IS DEVELOPMENT ONLY. IT MUST NEVER BE ENABLED IN PRODUCTION.** Guards:

1. `config.Load` requires `APP_ENV` and `AUTH_MODE` (no defaults) and rejects `AUTH_MODE=local` unless `APP_ENV` is `development` or `test`.
2. `identity.FromConfig` re-runs `Config.Validate`, and `NewLocalDevVerifier` refuses construction outside the allowlist.
3. The API logs a `local_development_auth_enabled` warning on every local-mode start.
4. Mobile `resolveAuthConfig` applies the same allowlist to `EXPO_PUBLIC_APP_ENV` and also requires a development bundle (`__DEV__`), so a release build cannot render "Continue as Dev User".

Supabase configuration is required only in `AUTH_MODE=supabase`.

## Deferred integrations

External runtime providers (Supabase Auth/OTP, email, object storage, maps, push, analytics) are deferred to the Integration & Pilot Hardening milestone. Each gets an application-facing port only when a real requirement needs it; domain code never imports provider SDKs.

The mobile app keeps provider details behind `AuthProvider` (`LocalDevAuthProvider` or `SupabaseAuthProvider`, chosen once from `EXPO_PUBLIC_AUTH_MODE`), API access behind `ApiClient`, and navigation policy in a pure state reducer. Supabase session tokens are delegated to Supabase with an Expo SecureStore storage adapter; the local development credential is also kept in SecureStore.

The contracts package is the transport source for OpenAPI and language-neutral JSON shapes. It does not make the Go domain depend on TypeScript.

## Wildlife domain (Requirement 002)

PostgreSQL 18.6 + PostGIS 3.6.4 (ADR-006). Each wildlife module owns its tables. The encounter module resolves the current internal user through `CurrentUserResolver` and never reads `auth_identities`; only `ACTIVE` users may create, edit, or submit.

```text
parks 1──* zones
  │          │  (zone_id, park_id) composite FK
  └──* encounters ──1 encounter_locations   (private; geography(Point,4326); write-only)
         │
         └── observer_user_id ──> users
hias (public_code from an identity sequence; immutable; optional merged_into self-FK)
```

Encounters have no Hia reference: that relationship belongs to a future Identification/Verification module. Precise location never appears in a response or log (ADR-008). Hia public codes are sequence-backed and immutable (ADR-009).

The telemetry interface is intentionally no-op in Requirement 001; an OpenTelemetry adapter can replace it without changing application/domain code.
