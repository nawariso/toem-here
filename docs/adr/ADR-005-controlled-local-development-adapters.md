# ADR-005: Controlled Local Development Adapters

Status: Accepted (Requirement 001-B)

## Context

Requirement 001 delivered the identity architecture: `IdentityVerifier` on the API, `AuthProvider` on mobile, a Supabase Email OTP adapter, a hardened JWKS/JWT verifier, and an internal `User` resolved from `(provider, provider_subject)`. The architecture is correct, but external runtime providers (Supabase project setup, email delivery, OTP delivery, provider availability) were blocking core product work. None of them help answer the current product question: is meeting and rediscovering individual real Hias fun?

## Decision

Use explicit, locally controlled adapters behind the existing application ports, selected by an explicit mode:

```text
IdentityVerifier (Go)                  AuthProvider (mobile)
├── LocalDevVerifier      AUTH_MODE=local     ├── LocalDevAuthProvider
└── JWT Verifier (Supabase) AUTH_MODE=supabase └── SupabaseAuthProvider
```

- `APP_ENV` and `AUTH_MODE` are required; neither has a default and there is no fallback between modes. Unknown values fail startup.
- `AUTH_MODE=local` is permitted only for `APP_ENV` in the allowlist `{development, test}`. `APP_ENV=production` with `AUTH_MODE=local` is a fatal startup error. The check runs in `config.Load`, again in `config.Validate` via `identity.FromConfig`, and again in the `NewLocalDevVerifier` constructor.
- Mobile resolves the mode from `EXPO_PUBLIC_APP_ENV` / `EXPO_PUBLIC_AUTH_MODE` and additionally requires a development bundle (`__DEV__`). A release build cannot select local mode.
- The local credential is a deterministic, non-sensitive bearer string. The API still runs `credential → IdentityVerifier → ExternalIdentity(LOCAL_DEV, developer-001) → AuthIdentity → User`. The client never sends a user ID, role, status, or permission. The Supabase JWT verifier rejects the local credential.
- Supabase adapters, SecureStore, JWKS, and documentation remain in place. They are deferred, not removed.

## Alternatives Considered

- **Configure external providers immediately:** rejected for now. It blocks product iteration on infrastructure that does not validate the product hypothesis.
- **Remove authentication completely:** rejected. Domain rules (ownership, server-derived identity, status) could not be developed or tested honestly.
- **Mock only at test level:** insufficient. Automated tests already inject fakes; the problem is running the real app on a device without external services.
- **Local development adapter behind existing ports:** chosen. It keeps the production path structurally identical and keeps the switch a configuration change.

## Consequences

Positive: faster iteration, reproducible development, fewer external blockers, $0 development runtime, preserved architecture, and a real PostgreSQL bootstrap path in every local session.

Negative: provider-specific issues (email delivery, OTP UX, real JWT issuance, session refresh) will be discovered later. This requires an explicit Integration & Pilot Hardening milestone. Local mode must stay strongly isolated from production, which is why it is guarded at config load, verifier construction, and the mobile bundle level, with tests for each guard.

## Exit Strategy

No migration is required. Switch `AUTH_MODE=local` to `AUTH_MODE=supabase` (and `EXPO_PUBLIC_AUTH_MODE` likewise), supply the Supabase values, and the production adapters are used. Local development users are stored under provider `LOCAL_DEV` and cannot collide with Supabase subjects; they are not carried into any deployed database.
