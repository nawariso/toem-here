# Foundation Architecture

The Go API is a modular monolith with inward dependencies:

```text
transport/http -> application -> domain
                       ^
infrastructure/identity + infrastructure/persistence
```

- Domain: internal user and validation rules; no framework/database/provider imports except UUID generation.
- Application: use cases and ports (`IdentityVerifier`, `UserRepository`, readiness).
- Infrastructure: pgx PostgreSQL repository, migrations, Supabase-compatible JWKS verifier, config and telemetry boundary.
- Transport: routes, Bearer authentication, stable errors, request IDs and structured request logs.

Authentication resolves `JWT -> ExternalIdentity -> AuthIdentity -> User -> Roles`. A PostgreSQL transaction and a transaction-scoped advisory lock guarantee one internal user per external identity even during concurrent first login.

The mobile app keeps provider details behind `AuthProvider`, API access behind `ApiClient`, and navigation policy in a pure state reducer. Session tokens are delegated to Supabase with an Expo SecureStore storage adapter.

The contracts package is the transport source for OpenAPI and language-neutral JSON shapes. It does not make the Go domain depend on TypeScript.

The telemetry interface is intentionally no-op in Requirement 001; an OpenTelemetry adapter can replace it without changing application/domain code.
