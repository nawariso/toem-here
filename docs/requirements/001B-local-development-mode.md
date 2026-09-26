# Requirement 001-B — Controlled Local Development Mode & External Integration Deferral

Status: ACCEPTED — independent review PASS WITH MINOR ISSUES at `ea3673bcd848db1de791453f102fe2a5fcb23034` (GitHub Actions run 36236034318 green)

| Gate | Status |
| --- | --- |
| REQ-001 Core Foundation | ACCEPTED |
| REQ-001-B Local Development Mode | ACCEPTED |
| Native local-auth smoke (device/emulator) | NOT RUN — DEFERRED MANDATORY GATE BEFORE REQUIREMENT 003 |
| Supabase Auth / Email OTP / real provider JWT E2E | DEFERRED TO INTEGRATION & PILOT HARDENING (not tested, not passed) |
| REQ-002 | UNBLOCKED |

## Purpose

Remove runtime dependency on external providers during core product development while preserving production-grade architectural boundaries. External authentication, email OTP, production storage, maps, notifications, and similar integrations are deferred to the **Integration & Pilot Hardening** milestone. They are not removed.

## Requirement 001 reclassification

| Area | Status |
| --- | --- |
| REQ-001 core foundation (code, architecture, CI) | ACCEPTED |
| Supabase Auth | DEFERRED TO INTEGRATION & PILOT HARDENING |
| Email OTP | DEFERRED TO INTEGRATION & PILOT HARDENING |
| Real provider JWT end to end | DEFERRED TO INTEGRATION & PILOT HARDENING |

Supabase Email OTP has **not** been tested end to end against a real project. It remains mandatory before public beta or pilot.

The Requirement 001 rule "development/test mode must not silently bypass token validation" is clarified as: development/test environments may use an explicit Local Development Identity Provider; authentication is never silently disabled.

## Runtime modes

| Variable | Values | Default |
| --- | --- | --- |
| `APP_ENV` | `development`, `test`, `production` | none (required) |
| `AUTH_MODE` | `local`, `supabase` | none (required) |
| `EXPO_PUBLIC_APP_ENV` | `development`, `test`, `production` | none (required) |
| `EXPO_PUBLIC_AUTH_MODE` | `local`, `supabase` | none (required) |

- Unknown values fail startup. There is no fallback.
- `AUTH_MODE=local` is allowed only for `APP_ENV` in `{development, test}`.
- `APP_ENV=production` + `AUTH_MODE=local` is a **fatal startup error**.
- The mobile app also refuses local mode in any release (non-`__DEV__`) bundle.

Required configuration:

- `AUTH_MODE=local`: `APP_ENV`, `DATABASE_URL`, `EXPO_PUBLIC_API_URL`. No Supabase value is read.
- `HTTP_PORT` is optional in every mode and keeps the existing Requirement 001 default of `8080` (accepted deviation from the 001-B draft, which listed it as required).
- `AUTH_MODE=supabase`: additionally `AUTH_ISSUER`, `AUTH_AUDIENCE`, `AUTH_JWKS_URL`, `EXPO_PUBLIC_SUPABASE_URL`, `EXPO_PUBLIC_SUPABASE_PUBLISHABLE_KEY`. A missing value fails fast.

## Implementation map

| Concern | Location |
| --- | --- |
| Mode config and production guard | `services/api/internal/infrastructure/config/config.go` |
| `LocalDevVerifier`, verifier selection (`FromConfig`) | `services/api/internal/infrastructure/identity/local.go` |
| API wiring and startup warning | `services/api/cmd/api/main.go` |
| Mobile mode resolution | `apps/mobile/src/config/auth-mode.ts` |
| `LocalDevAuthProvider` | `apps/mobile/src/auth/local-dev-adapter.ts` |
| Adapter selection | `apps/mobile/src/auth/AuthContext.tsx` |
| "Continue as Dev User" UI | `apps/mobile/app/passport.tsx` |
| Decision record | `docs/adr/ADR-005-controlled-local-development-adapters.md` |

Local identity: provider `LOCAL_DEV`, subject `developer-001`, no email. The credential is the deterministic, non-sensitive string `toem-local-dev.developer-001`. The Supabase verifier rejects it.

## Test map

| Requirement | Test |
| --- | --- |
| valid local credential → deterministic identity | `identity/local_test.go` `TestLocalDevVerifierResolvesDeterministicIdentity` |
| invalid credential → unauthorized | `identity/local_test.go` `TestLocalDevVerifierRejectsAnythingElse`; `e2e/local_auth_e2e_test.go` `TestLocalAuthDoesNotTrustClientSuppliedIdentity` |
| local auth + production → startup rejected | `config_test.go` `TestProductionLocalIsRejected`; `identity/local_test.go` `TestLocalDevVerifierCannotBeConstructedForProduction`; `cmd/api/main_test.go` `TestProductionWithLocalAuthTerminatesStartup` (real process exit) |
| supabase mode → local credential rejected | `identity/local_test.go` `TestFromConfigSelectsExactlyTheConfiguredVerifier`; `e2e/local_auth_e2e_test.go` `TestSupabaseStackRejectsLocalDevelopmentCredential` |
| repeated local bootstrap → same internal user | `e2e/local_auth_e2e_test.go` `TestLocalAuthBootstrapsOneStableInternalUser` (PostgreSQL) |
| config matrix (dev+local, dev+supabase missing, prod+local, prod+supabase) | `config_test.go` |
| mobile local → dev login available; supabase → unavailable | `__tests__/screens.test.tsx` |
| dev login → authenticated; logout → guest | `__tests__/screens.test.tsx`, `__tests__/controller.test.ts`, `__tests__/local-dev-adapter.test.ts` |
| dev login never in production configuration | `__tests__/screens.test.tsx` "production UX isolation", `__tests__/auth-mode.test.ts` |

## Development runtime acceptance (native smoke)

```text
Launch → Guest Home → Create Your Hia Passport → Continue as Dev User
  → Bootstrap → Passport Setup → Profile → Logout → Guest Home
```

This does not depend on Supabase, email, or OTP.

Status: **NOT RUN.** It has not passed and must not be described as passed. The independent review reclassified it as a **deferred mandatory gate before Requirement 003**: it must be completed before Camera / Media / device-dependent functionality begins and before any public pilot. It does not block Requirement 002, which is domain/API/PostgreSQL/PostGIS work with no native-device dependency.

## Accepted review debt

- `HTTP_PORT` keeps its `8080` default.
- The local credential string exists in both Go (`identity/local.go`) and TypeScript (`src/auth/local-dev-adapter.ts`); contract tests keep them aligned. No cross-language generator is to be built.
- One deterministic development user is sufficient. Role/user switching is not added until a real requirement needs it.

## Deferred-integration policy (Requirement 001-B §21–§29)

Until Integration & Pilot Hardening, feature requirements prefer local processes, Docker, in-memory/filesystem adapters, and deterministic fixtures over external SaaS. Each deferred provider capability gets a stable application-facing port **when a real requirement needs it** (Replaceable Edge, Not Abstract Everything). Domain code never imports provider SDKs. No speculative ports (`MediaStore`, `NotificationPort`, `MapProvider`) are created by this requirement; later requirements introduce them with their first real use.

## Non-goals

Real Email OTP, SMTP, Google/Apple/LINE auth, production Supabase configuration, cloud deployment/storage, maps, push notifications, and all wildlife domain work (delivered separately from Requirement 002 onward).
