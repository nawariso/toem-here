# ADR-003: External Authentication and Internal User Identity

Status: Accepted

## Context
Email OTP should launch quickly through Supabase Free Tier, while TOEM HERE users, passports, roles, and future contributions must survive provider changes and added Google, Apple, or LINE identities.

## Decision
Supabase authenticates email OTP and issues JWTs. Mobile access goes through an AuthProvider adapter and stores sessions in Expo SecureStore. The API verifies asymmetric JWT signatures through JWKS plus issuer, audience, expiry, algorithm, and subject checks behind IdentityVerifier. It maps `(provider, provider_subject)` to a stable internal UUID. User, identity, and USER role creation is atomic.

## Alternatives Considered
- Custom passwords/sessions: rejected due security and operational burden.
- Supabase user ID as domain user ID: rejected because it locks domain history to one provider.
- Trusting mobile claims: rejected because clients are untrusted.

## Consequences
The API depends on public signing-key availability/cache refresh, not Supabase SDK types. The JWKS cache is TTL-bounded with a refresh cooldown and a minimum RSA modulus size, so unknown key ids cannot amplify outbound traffic and undersized keys are never trusted; rotation is still adopted once the cache expires. Logout revokes the provider session locally; already-issued stateless access tokens remain valid until expiry unless future session introspection is added.

## Exit Strategy
Add IdentityVerifier/AuthProvider adapters and auth_identity rows for new providers. Migrate providers without changing internal user IDs.
