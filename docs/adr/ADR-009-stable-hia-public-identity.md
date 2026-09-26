# ADR-009: Stable Hia Public Identity

Status: Accepted (Requirement 002, reviewed at `9e8822b`)

## Context

People will refer to individual monitor lizards by a short, stable public code (`HIA-000001`). Codes must be unique under concurrency, never reused, never editable, and must not expose internal IDs.

## Decision

- Internal identity: UUID `id`, never returned by the API.
- Public identity: `public_number BIGINT GENERATED ALWAYS AS IDENTITY` and `public_code` as a stored generated column (`HIA-` + six-digit zero padding, growing beyond six digits after 999999). Both carry unique constraints.
- The server always assigns the number; a caller-supplied code is ignored. The allocator is a database sequence, **never** `COUNT(*)`/`MAX()+1`, which races.
- A `BEFORE UPDATE` trigger rejects any change to `id` or `public_number`, so the code is immutable even to direct SQL.
- A merge keeps both rows: the merged Hia gets status `MERGED` and `merged_into_hia_id` (FK, never itself); the API exposes `mergedIntoPublicCode`. No merge operation is built in this requirement.

## Alternatives Considered

- **`COUNT(*) + 1` or `MAX() + 1`:** duplicates under concurrent inserts.
- **Random short codes:** unique but not orderable or memorable.
- **Application-side counter:** needs its own locking; the sequence already provides it.

## Consequences

Codes are unique and never reused, but not gap-free: a rolled-back insert consumes a sequence value. Gaps are accepted; gap-free allocation must not be attempted. Uniqueness and immutability are mandatory and reuse is forbidden. A 40-goroutine concurrency test asserts no duplicates.

## Exit Strategy

The format is produced both by the database and by `hia.FormatPublicCode` (tested to agree). A different public scheme later would be a new column; existing codes remain valid aliases.
