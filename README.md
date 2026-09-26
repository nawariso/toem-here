# TOEM HERE

Requirement 001 foundation (plus Requirement 001-B local development mode) for a community monitor-lizard application. This repository currently contains only the mobile shell, identity/authentication boundary, internal-user API, and PostgreSQL persistence. Wildlife features are intentionally absent.

A fresh clone runs the mobile app, Go API, PostgreSQL, and authentication **without a Supabase account, email provider, OTP, or any cloud account**, using Controlled Local Development Mode (`AUTH_MODE=local`).

> **LOCAL AUTH IS DEVELOPMENT ONLY. IT MUST NEVER BE ENABLED IN PRODUCTION.**
> `APP_ENV=production` + `AUTH_MODE=local` is a fatal startup error, and release mobile builds refuse local mode.

Status: Requirement 001 core foundation accepted. Supabase Auth / Email OTP / real provider JWT are **DEFERRED TO INTEGRATION & PILOT HARDENING** — not tested end to end. See `docs/requirements/001B-local-development-mode.md`.

## Architecture and cost

- Expo SDK 57 / React Native 0.86.3 / React 19.2.3 mobile app with Expo Router. Auth goes through an `AuthProvider` adapter: `LocalDevAuthProvider` (development) or `SupabaseAuthProvider` (email OTP, deferred).
- Go 1.27.1 modular-monolith API. Domain and application layers do not depend on Supabase or pgx.
- PostgreSQL 18.6 as the system of record, run locally with Docker Compose.
- API identity goes through `IdentityVerifier`: `LocalDevVerifier` (`AUTH_MODE=local`) or the Supabase JWKS/JWT verifier (`AUTH_MODE=supabase`). Supabase Auth Free Tier remains the selected provider for Integration & Pilot Hardening.
- Mandatory infrastructure cost: **$0/month**. Local development has no external runtime dependency.

See `docs/architecture/foundation.md` and `docs/adr/`.

## Prerequisites

Install stable versions:

- Git
- Docker Desktop / Docker Engine with Compose v2+
- Go 1.27.1
- Node.js 24.3.0 exactly (`.nvmrc` and `.node-version`)
- npm 11.4.2 exactly (`packageManager`, `devEngines`, and `apps/mobile/.npmrc`)
- Expo Go or an Android/iOS simulator
- (Supabase mode only, deferred) a free Supabase project with asymmetric JWT signing keys

Pinned product versions are in `services/api/go.mod`, `apps/mobile/package.json`, and `apps/mobile/package-lock.json`. Use `nvm use` (or an equivalent version manager) from the repository root before running npm. npm rejects a different Node/npm toolchain so a fresh developer cannot silently regenerate a materially different lockfile. The lockfile is generated and checked in CI's exact Node 24.3.0 / npm 11.4.2 environment.

## 1. Clone and configure

```bash
git clone https://github.com/nawariso/toem-here.git
cd toem-here
cp .env.example .env.local
```

Edit `.env.local`:

1. Replace the local database password placeholder in both `POSTGRES_PASSWORD` and `DATABASE_URL`.
2. Keep `APP_ENV=development`, `AUTH_MODE=local`, `EXPO_PUBLIC_APP_ENV=development`, `EXPO_PUBLIC_AUTH_MODE=local`.
3. Set `EXPO_PUBLIC_API_URL` to this computer's LAN URL (for example `http://192.168.1.20:8080`) when testing on a physical phone. A phone cannot reach the computer through its own `localhost`.

No Supabase value is needed in local mode. `.env.local` is ignored by Git. `.env.example` contains placeholders only.

### Authentication modes

| `APP_ENV` | `AUTH_MODE` | Result |
| --- | --- | --- |
| `development` / `test` | `local` | Starts. Deterministic dev user `LOCAL_DEV/developer-001`. No Supabase config read. |
| any | `supabase` | Starts only when `AUTH_ISSUER`, `AUTH_AUDIENCE`, `AUTH_JWKS_URL` are set (mobile also needs `EXPO_PUBLIC_SUPABASE_URL`, `EXPO_PUBLIC_SUPABASE_PUBLISHABLE_KEY`). |
| `production` | `local` | **Fatal startup error.** |
| missing / unknown | missing / unknown | Fatal startup error. No defaults, no fallback. |

The local credential is a fixed, non-sensitive development string. The API still verifies it through `IdentityVerifier` and resolves the internal user through the normal bootstrap; the client never sends a user ID, role, or status. A Supabase-mode API rejects it.

## 2. (Deferred) Configure Supabase email OTP

Skip this section during core development. It is required only for `AUTH_MODE=supabase`, which is deferred to the Integration & Pilot Hardening milestone. Set `AUTH_MODE=supabase` / `EXPO_PUBLIC_AUTH_MODE=supabase`, uncomment the Supabase block in `.env.local`, set `AUTH_ISSUER` to `https://PROJECT_REF.supabase.co/auth/v1`, `AUTH_JWKS_URL` to `https://PROJECT_REF.supabase.co/auth/v1/.well-known/jwks.json`, keep `AUTH_AUDIENCE=authenticated`, and copy the Project URL and **publishable** key (never a secret/service-role key) into the `EXPO_PUBLIC_SUPABASE_*` fields.

In the free Supabase project:

1. Authentication → Providers → Email: enable Email.
2. Authentication → Email Templates → Magic Link: make the message show `{{ .Token }}` so the user receives a numeric OTP instead of relying only on a magic link.
3. Authentication → Signing Keys: use an asymmetric signing key (RS256 for this baseline). The API deliberately accepts RS256 only.
4. Use the dashboard's built-in email sender for development. Its rate limits are acceptable for this requirement; configure custom SMTP only when justified later.

The mobile app calls `signInWithOtp` and then `verifyOtp(type: "email")`. It sends the resulting access JWT to the API; TOEM HERE never receives or stores a password.

## 3. Start PostgreSQL

Load the local environment in Bash/Git Bash and start the pinned image:

```bash
set -a
source .env.local
set +a
docker compose --env-file .env.local -f infra/docker/compose.yaml up -d

docker compose --env-file .env.local -f infra/docker/compose.yaml ps
```

Wait until `postgres` is `healthy`.

## 4. Run and roll back migrations

From the repository root with `.env.local` loaded:

```bash
go run ./services/api/cmd/migrate up
go run ./services/api/cmd/migrate down
go run ./services/api/cmd/migrate up
```

The migration creates `users`, `auth_identities`, and `user_roles`, including foreign keys, `(provider, provider_subject)` uniqueness, and a partial unique index on `lower(username)`.

## 5. Start the API

```bash
go run ./services/api/cmd/api
```

In another terminal:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

Both return HTTP 200 when the process and database are healthy. Startup fails immediately if critical database/auth configuration is missing. In local mode the API logs a `local_development_auth_enabled` warning at startup.

Exercise local auth from the terminal (development only):

```bash
curl -X POST -H "Authorization: Bearer toem-local-dev.developer-001" http://localhost:8080/v1/auth/bootstrap
```

## 6. Start the mobile app

Expo embeds variables prefixed `EXPO_PUBLIC_` from the process environment. From the same shell where `.env.local` is loaded:

```bash
cd apps/mobile
npm ci
npm start
```

Scan the QR code with Expo Go, or press `a`/`i` for a configured simulator.

Local-auth smoke test (the development runtime acceptance test):

1. Launch → Splash → guest Home.
2. **Create Your Hia Passport** → the screen shows a `LOCAL DEVELOPMENT MODE` badge and **Continue as Dev User** (no email form).
3. **Continue as Dev User** → `POST /v1/auth/bootstrap` creates (first time) or reuses the internal user → Passport Setup.
4. Enter username and display name → Profile shows the internal user and USER role.
5. **Log out** → local session cleared from SecureStore → guest Home.
6. Repeat step 3: the same internal user is returned; no duplicate is created.

In Supabase mode (deferred) the same screen shows the email/OTP form instead and never shows **Continue as Dev User**. Supabase token auto-refresh starts only while React Native reports the app as active; backgrounding stops refresh, and provider unmount removes the AppState listener.

## 7. Run checks

Backend unit tests (integration tests skip only when `TEST_DATABASE_URL` is absent):

```bash
cd services/api
gofmt -w .
go vet ./...
go test ./...
```

PostgreSQL integration/migration tests from the repository root:

```bash
set -a; source .env.local; set +a
export TEST_DATABASE_URL="$DATABASE_URL"
cd services/api
go test -race -count=1 ./...
```

Mobile and contracts:

```bash
cd apps/mobile
npm ci
npm test
npm run typecheck
npm run lint
npx expo install --check
npx --yes expo-doctor@1.20.4
npm audit --audit-level=high
./node_modules/.bin/tsc -p ../../packages/contracts/tsconfig.json
```

GitHub Actions runs the same gates with a real PostgreSQL 18.6 service and pinned Node/Go versions.

## API

- `GET /health`
- `GET /ready`
- `POST /v1/auth/bootstrap` (Bearer credential: Supabase JWT, or the local dev credential in `AUTH_MODE=local`)
- `GET /v1/users/me` (Bearer credential)
- `PATCH /v1/users/me` (Bearer credential; only `username`, `displayName`, `locale`)

See `packages/contracts/openapi.yaml`. Errors always use:

```json
{"error":{"code":"UNAUTHENTICATED","message":"Authentication is required","requestId":"..."}}
```

## Security notes

- Local development auth is guarded three times on the API (config load, config re-validation in `identity.FromConfig`, `NewLocalDevVerifier` constructor) and twice on mobile (`EXPO_PUBLIC_APP_ENV` allowlist and `__DEV__` bundle check). Each guard has a test, including a real-process test that `APP_ENV=production AUTH_MODE=local` exits non-zero before touching the database.
- Supabase session data and the local development credential use Expo SecureStore, never AsyncStorage.
- The API verifies RS256 signature, key ID, issuer, audience, expiry, issued-at validity, and subject.
- JWKS refreshes are throttled and the key cache is TTL-bounded, so unauthenticated callers cannot turn unknown-key-id tokens into unbounded outbound fetches against the identity provider. Signing keys below a 2048-bit RSA modulus are ignored even if the JWKS endpoint offers them. Key rotation is still picked up once the cache expires.
- Client claims do not authorize internal user IDs or roles.
- SQL is parameterized and identity creation is one transaction.
- Logs contain request ID, method, path, status, and duration, but not JWT, OTP, email, credentials, or precise location.
- React and React DOM are pinned to Expo SDK 57's supported 19.2.3 baseline. Requirement 001 does not use React Server Components, so `react-server-dom-webpack` is not a direct dependency. React and React DOM have no `expo.install.exclude` exception; `npx expo install --check` validates them normally.
- `govulncheck` reports **no vulnerabilities**. `pgx` is pinned to v5.9.2 and `golang.org/x/text` to v0.39.0 specifically to clear GO-2026-5004 (SQL injection via dollar-quoted placeholder confusion) and GO-2026-5970.
- `npm audit --audit-level=high` passes. Thirteen **moderate** advisories remain inside Expo's own build toolchain (`@expo/cli` → `xcode` → `uuid`, and `expo-router` → `query-string` → `decode-uri-component`). `npm audit fix --force` "resolves" them by downgrading to Expo 46 / expo-router 5, which would abandon the SDK 57 baseline, so they are accepted and gated at `high` instead. They affect developer tooling, not the shipped app runtime.

## Current limits

There is no deployed API/database, production SMTP, Apple/Google/LINE login, account-deletion workflow, or wildlife functionality. Supabase Auth, Email OTP, and real provider JWT verification end to end are **DEFERRED TO INTEGRATION & PILOT HARDENING** and have not been tested against a real project; they are mandatory before any public beta or Lumpini pilot. Account deletion must be designed with future wildlife contribution-retention semantics before public beta or store release.
