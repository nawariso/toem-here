# Requirement 001 — Project Bootstrap & Identity Foundation

Status: CORE FOUNDATION ACCEPTED — external auth integration DEFERRED

> **Amended by Requirement 001-B** (`docs/requirements/001B-local-development-mode.md`).
>
> | Area | Status |
> | --- | --- |
> | Code / architecture / CI | ACCEPTED |
> | Supabase Auth, Email OTP, real provider JWT end to end | DEFERRED TO INTEGRATION & PILOT HARDENING (not tested, not passed) |
>
> The rule "development/test mode must not silently bypass token validation" now reads: development/test environments may use an explicit Local Development Identity Provider (`AUTH_MODE=local`); authentication is never silently disabled, and `APP_ENV=production` + `AUTH_MODE=local` is a fatal startup error.

## Objective

Build the first reproducible technical foundation for TOEM HERE: monorepo structure, Expo mobile shell, Go modular-monolith API, PostgreSQL, internal user identity, Supabase email OTP authentication, security/observability baseline, tests, CI, ADRs, and developer documentation. Do not implement wildlife/Hia domain features.

Principles: small start, mandatory infrastructure cost $0/month, stable/GA/LTS dependencies only, replaceable external providers, prove before scale.

## Required repository shape

- `apps/mobile`: React Native + Expo + TypeScript + Expo Router.
- `services/api`: Go modular monolith.
- `packages/contracts`: shared API contract artifacts/types, without coupling Go domain to TypeScript.
- `infra/docker`: local PostgreSQL Docker Compose assets.
- `docs/architecture`, `docs/adr`, `scripts`, `.github/workflows`.
- Root `.env.example`, README and pinned lock/module files.
- Do not create admin, Re-ID, AI, camera, scan, map, leaderboard, HiaDex, or other future-domain infrastructure.

## Version baseline (verified at implementation time)

Use compatible stable releases and pin them. Current authoritative releases checked on 2026-09-15: Expo SDK/npm `expo` 57.0.22 (Expo docs identify SDK 57 as latest), Expo Router 57.0.21, Expo SDK 57's bundled compatible React Native 0.86.3 and the patched Expo Router-compatible React 19.2.8, `@supabase/supabase-js` 2.116.0, `expo-secure-store` 57.0.4, Go 1.27.1, PostgreSQL 18.6. React Native 0.87.1 is stable but is not the React Native version bundled by stable Expo SDK 57, so it is not suitable for this Expo application. Use Node 24 LTS in CI/developer requirements rather than the local non-LTS Node 25. Avoid beta/RC/canary/nightly.

## Mobile behavior

Represent auth state explicitly as `UNKNOWN | GUEST | AUTHENTICATED`; never assume login. Initial routes/screens only: Splash, Home, Create Your Hia Passport / Login, Passport Setup, Profile. First open transitions Splash → Home as guest. Home includes `Scan a Hia — Coming Soon`. Primary language is `Create Your Hia Passport`, not `Register Account`.

Email OTP flow: enter email → Supabase `signInWithOtp` → enter OTP → Supabase `verifyOtp` → access JWT → authenticated `POST /v1/auth/bootstrap`. Persist Supabase auth session through an adapter backed by Expo SecureStore; never use plain AsyncStorage for access/refresh tokens. External auth must sit behind a mobile adapter. Logout calls provider sign-out, clears local/sensitive auth state, and navigates to guest Home.

After authentication: bootstrap internal identity. If profile lacks username/displayName, route to Passport Setup; otherwise Profile/Home as appropriate. Setup collects username and display name; Profile displays internal API user. Tests must cover guest, authenticated, incomplete/complete profile, login navigation and logout navigation. Prefer testable state/reducer/navigation policy logic over brittle UI internals.

Only public Expo config may use `EXPO_PUBLIC_SUPABASE_URL`, `EXPO_PUBLIC_SUPABASE_PUBLISHABLE_KEY`, and API URL. No private Supabase service key belongs in the app.

## Backend boundaries and behavior

Use a modular monolith with domain/application/infrastructure/transport boundaries. Domain cannot import Supabase, pgx, SQL generators, or PostgreSQL adapters. Define `IdentityVerifier.Verify(token) -> ExternalIdentity{provider, subject, email?}`. Implement a Supabase-compatible JWT/JWKS verifier that validates cryptographic signature, allowed algorithm, issuer, audience, expiry and required subject; reject invalid/expired tokens. Do not trust client-provided user ID, role, permission, username, or status as authoritative identity.

Persistence must be behind interfaces; use mature lightweight `pgx/v5` rather than a large ORM and document the choice. Use parameterized SQL. First bootstrap creates User + AuthIdentity + USER role atomically in one PostgreSQL transaction; repeat/concurrent login returns the existing user without duplicates.

Endpoints:
- `GET /health`: liveness, no auth.
- `GET /ready`: readiness including DB check.
- `POST /v1/auth/bootstrap`: Bearer-authenticated identity resolution/upsert, returns current internal user.
- `GET /v1/users/me`: Bearer-authenticated current user and roles.
- `PATCH /v1/users/me`: accepts only username, displayName, locale. Never permits roles, userId/id, or status mutation.

Current-user JSON includes `id`, nullable `username`, nullable `displayName`, nullable `avatarUrl`, `locale`, `roles`, and `profileComplete`.

Every API error uses `{ "error": { "code": "...", "message": "...", "requestId": "..." } }`; never expose stack traces, SQL errors, JWT/OTP, secrets, or credentials. Add request ID middleware and structured logs with method, path, status, duration; avoid token/email/precise-location logs. Define a no-op/replaceable telemetry boundary but do not deploy a platform.

Environment config includes `APP_ENV`, `HTTP_PORT`, `DATABASE_URL`, `AUTH_ISSUER`, `AUTH_AUDIENCE`, `AUTH_JWKS_URL`, with fail-fast validation. Development/test mode must not silently bypass token validation. Unit tests may inject a fake `IdentityVerifier` through the abstraction.

## Database

Versioned up/down SQL migrations create:
- `users`: UUID PK, nullable username/display_name/avatar_url/deleted_at, locale, status, created_at, updated_at, TIMESTAMPTZ.
- `auth_identities`: UUID PK, user FK, provider, provider_subject, created_at, last_login_at; unique(provider, provider_subject).
- `user_roles`: user FK + role; initial USER; schema can hold future role strings/enumeration values without implementing permissions.

Enforce case-insensitive username uniqueness in PostgreSQL (for example a unique index on `lower(username)` for non-null values), foreign keys and data integrity. Preserve future deletion/anonymization via status/deleted_at. Migrations and persistence integration tests must run against real PostgreSQL, never SQLite.

## Tests and CI

Strict TDD for behavior: create failing tests first, observe expected failures, implement minimally, refactor green. Backend test coverage at minimum: user creation, auth identity creation, first/repeat login, duplicate prevention/concurrency or constraint behavior, case-insensitive username uniqueness, unauthorized/authorized `/users/me`, permitted profile update and rejection/ignoring of forbidden fields, JWT rejection cases, and migrations on PostgreSQL. Mobile covers all state/navigation cases listed above.

GitHub Actions must run Go formatting/static checks (`gofmt`, `go vet`), Go tests, TypeScript checking, ESLint, mobile tests, PostgreSQL migration/integration tests, and appropriate free vulnerability checks (`govulncheck`, `npm audit` at a documented severity). CI uses a PostgreSQL service and must be capable of green execution.

## Documentation

Create ADRs with Context, Decision, Alternatives Considered, Consequences, Exit Strategy:
1. Modular Monolith.
2. PostgreSQL as System of Record.
3. External Authentication + Internal User Identity.
4. Zero-Cost-First Development.

README must let a fresh developer clone, configure env, start PostgreSQL, run migrations, start API, start mobile, configure and execute Supabase email OTP login, and run every test without guessing. Include exact commands and version prerequisites.

## Secrets and cost

Commit `.env.example` with placeholders only. Ignore all real env files and credentials. Mandatory monthly infrastructure cost is $0; Supabase Free Tier is the only hosted dependency. No AWS, paid Cloudflare, Railway, Kubernetes, Redis, Kafka, managed DB, or paid monitoring.

## Non-goals

Do not implement Hia domain, Encounter, Camera, Scan, uploads, Re-ID/CV/AI, HiaDex, Explore/Map/Park/Zones, leaderboard/points/achievements, notifications, admin console, community, moderation, or full account deletion. Only preserve schema ability for future deletion/anonymization.

## Acceptance and final evidence

A fresh clone can start PostgreSQL, apply/rollback/reapply migrations, run API and Expo app; guest/login/OTP/bootstrap/passport/profile/logout code paths are present and tested; JWT validation is strict; first/repeat login semantics and case-insensitive username uniqueness are proven with PostgreSQL; tests/checks pass; CI is green; no secrets are committed; all ADRs and README are complete; cost is $0. Final report must include commit hash, repository tree, exact versions, architecture decisions, commands, migration/test/CI/security results, limitations, deviations, debt, and next step.
