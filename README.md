# TOEM HERE

Requirement 001 foundation for a community monitor-lizard application. This repository currently contains only the mobile shell, identity/authentication boundary, internal-user API, and PostgreSQL persistence. Wildlife features are intentionally absent.

## Architecture and cost

- Expo SDK 57.0.23 / React Native 0.86.3 mobile app with Expo Router and secure Supabase email-OTP sessions.
- Go 1.27.1 modular-monolith API. Domain and application layers do not depend on Supabase or pgx.
- PostgreSQL 18.6 as the system of record, run locally with Docker Compose.
- Supabase Auth Free Tier as the initial external identity provider.
- Mandatory infrastructure cost: **$0/month**.

See `docs/architecture/foundation.md` and `docs/adr/`.

## Prerequisites

Install stable versions:

- Git
- Docker Desktop / Docker Engine with Compose v2+
- Go 1.27.1
- Node.js 24.3+ LTS (Node 25 is intentionally unsupported)
- npm 11+
- Expo Go or an Android/iOS simulator
- A free Supabase project with asymmetric JWT signing keys

Pinned product versions are in `services/api/go.mod`, `apps/mobile/package.json`, and `apps/mobile/package-lock.json`.

## 1. Clone and configure

```bash
git clone https://github.com/nawariso/toem-here.git
cd toem-here
cp .env.example .env.local
```

Edit `.env.local`:

1. Replace the local database password placeholder.
2. In Supabase Dashboard, copy Project URL and the **publishable** key (never a secret/service-role key) into the `EXPO_PUBLIC_` fields.
3. Set `AUTH_ISSUER` to `https://PROJECT_REF.supabase.co/auth/v1`.
4. Set `AUTH_JWKS_URL` to `https://PROJECT_REF.supabase.co/auth/v1/.well-known/jwks.json`.
5. Keep `AUTH_AUDIENCE=authenticated` unless the Supabase JWT configuration explicitly uses a different audience.
6. Set `EXPO_PUBLIC_API_URL` to this computer's LAN URL (for example `http://192.168.1.20:8080`) when testing on a physical phone. A phone cannot reach the computer through its own `localhost`.

`.env.local` is ignored by Git. `.env.example` contains placeholders only.

## 2. Configure Supabase email OTP

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

Both return HTTP 200 when the process and database are healthy. Startup fails immediately if critical database/auth configuration is missing.

## 6. Start the mobile app

Expo embeds variables prefixed `EXPO_PUBLIC_` from the process environment. From the same shell where `.env.local` is loaded:

```bash
cd apps/mobile
npm ci
npm start
```

Scan the QR code with Expo Go, or press `a`/`i` for a configured simulator. Expected first launch: Splash → guest Home. Select **Create Your Hia Passport**, enter email, then the OTP. First login bootstraps one internal user and routes to Passport Setup; enter username and display name. Profile then shows the internal user and USER role. Logout calls Supabase local sign-out, clears its SecureStore-backed session, and returns to guest Home.

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
- `POST /v1/auth/bootstrap` (Bearer JWT)
- `GET /v1/users/me` (Bearer JWT)
- `PATCH /v1/users/me` (Bearer JWT; only `username`, `displayName`, `locale`)

See `packages/contracts/openapi.yaml`. Errors always use:

```json
{"error":{"code":"UNAUTHENTICATED","message":"Authentication is required","requestId":"..."}}
```

## Security notes

- Supabase session data uses Expo SecureStore, never AsyncStorage.
- The API verifies RS256 signature, key ID, issuer, audience, expiry, issued-at validity, and subject.
- JWKS refreshes are throttled and the key cache is TTL-bounded, so unauthenticated callers cannot turn unknown-key-id tokens into unbounded outbound fetches against the identity provider. Signing keys below a 2048-bit RSA modulus are ignored even if the JWKS endpoint offers them. Key rotation is still picked up once the cache expires.
- Client claims do not authorize internal user IDs or roles.
- SQL is parameterized and identity creation is one transaction.
- Logs contain request ID, method, path, status, and duration, but not JWT, OTP, email, credentials, or precise location.
- React 19.2.8 is deliberately pinned above Expo's older 19.2.3 recommendation because it satisfies Expo Router's `react-server-dom-webpack@~19.2.4` peer range while staying compatible with React Native 0.86.3. `expo.install.exclude` records this deliberate validation exception for React/React DOM only; all other Expo dependency checks remain active and `npx expo-doctor` passes 21/21.
- `govulncheck` reports **no vulnerabilities**. `pgx` is pinned to v5.9.2 and `golang.org/x/text` to v0.39.0 specifically to clear GO-2026-5004 (SQL injection via dollar-quoted placeholder confusion) and GO-2026-5970.
- `npm audit --audit-level=high` passes. Thirteen **moderate** advisories remain inside Expo's own build toolchain (`@expo/cli` → `xcode` → `uuid`, and `expo-router` → `query-string` → `decode-uri-component`). `npm audit fix --force` "resolves" them by downgrading to Expo 46 / expo-router 5, which would abandon the SDK 57 baseline, so they are accepted and gated at `high` instead. They affect developer tooling, not the shipped app runtime.

## Current limits

There is no deployed API/database, production SMTP, Apple/Google/LINE login, account-deletion workflow, or wildlife functionality. A real email-OTP end-to-end run requires the developer's free Supabase project values. Account deletion must be designed with future wildlife contribution-retention semantics before public beta or store release.
