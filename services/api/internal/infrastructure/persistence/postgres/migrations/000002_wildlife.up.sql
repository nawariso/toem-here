-- Requirement 002: wildlife domain foundation (parks, zones, hias, encounters,
-- private encounter locations). Idempotent so `migrate up` can be re-run.
--
-- The whole file runs as one implicit transaction. The advisory lock
-- serialises concurrent runs (for example parallel integration-test schemas)
-- so CREATE EXTENSION cannot race with itself.
SELECT pg_advisory_xact_lock(2002002002);

-- PostGIS is database-wide shared infrastructure. It is created once in the
-- public schema and deliberately NOT dropped by the down migration.
CREATE EXTENSION IF NOT EXISTS postgis WITH SCHEMA public;

-- Park module ---------------------------------------------------------------
CREATE TABLE IF NOT EXISTS parks (
  id UUID PRIMARY KEY,
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  city TEXT NOT NULL,
  country_code TEXT NOT NULL,
  timezone TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT parks_slug_format CHECK (char_length(slug) <= 64 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  CONSTRAINT parks_name_length CHECK (char_length(name) BETWEEN 1 AND 120),
  CONSTRAINT parks_city_length CHECK (char_length(city) BETWEEN 1 AND 120),
  CONSTRAINT parks_country_code_format CHECK (country_code ~ '^[A-Z]{2}$'),
  -- IANA validity is checked by the application (pg_timezone_names is not
  -- immutable, so it cannot back a CHECK constraint).
  CONSTRAINT parks_timezone_present CHECK (char_length(timezone) BETWEEN 1 AND 64),
  CONSTRAINT parks_status_valid CHECK (status IN ('ACTIVE', 'INACTIVE', 'ARCHIVED'))
);
CREATE UNIQUE INDEX IF NOT EXISTS parks_slug_ci_unique ON parks (lower(slug));

CREATE TABLE IF NOT EXISTS zones (
  id UUID PRIMARY KEY,
  park_id UUID NOT NULL REFERENCES parks(id) ON DELETE RESTRICT,
  slug TEXT NOT NULL,
  name TEXT NOT NULL,
  status TEXT NOT NULL,
  boundary public.geometry(MultiPolygon, 4326) NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT zones_slug_format CHECK (char_length(slug) <= 64 AND slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
  CONSTRAINT zones_name_length CHECK (char_length(name) BETWEEN 1 AND 120),
  CONSTRAINT zones_status_valid CHECK (status IN ('ACTIVE', 'INACTIVE', 'ARCHIVED')),
  CONSTRAINT zones_boundary_valid CHECK (boundary IS NULL OR public.ST_IsValid(boundary)),
  -- Target of the encounters (zone_id, park_id) composite foreign key.
  CONSTRAINT zones_id_park_unique UNIQUE (id, park_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS zones_park_slug_ci_unique ON zones (park_id, lower(slug));
CREATE INDEX IF NOT EXISTS zones_boundary_gix ON zones USING GIST (boundary);

-- Hia module ----------------------------------------------------------------
-- public_number comes from an identity sequence (never COUNT(*)); public_code
-- is derived from it. Neither can change after insert (see trigger below).
CREATE TABLE IF NOT EXISTS hias (
  id UUID PRIMARY KEY,
  public_number BIGINT GENERATED ALWAYS AS IDENTITY (START WITH 1 MINVALUE 1 NO CYCLE),
  public_code TEXT GENERATED ALWAYS AS (
    'HIA-' || CASE WHEN public_number < 1000000 THEN lpad(public_number::text, 6, '0') ELSE public_number::text END
  ) STORED,
  nickname TEXT NULL,
  status TEXT NOT NULL,
  home_park_id UUID NULL REFERENCES parks(id) ON DELETE RESTRICT,
  first_seen_at TIMESTAMPTZ NULL,
  confirmed_at TIMESTAMPTZ NULL,
  merged_into_hia_id UUID NULL REFERENCES hias(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT hias_public_number_unique UNIQUE (public_number),
  CONSTRAINT hias_public_code_unique UNIQUE (public_code),
  CONSTRAINT hias_nickname_length CHECK (nickname IS NULL OR char_length(nickname) BETWEEN 1 AND 60),
  CONSTRAINT hias_status_valid CHECK (status IN ('PROVISIONAL', 'CONFIRMED', 'INACTIVE', 'ARCHIVED', 'MERGED')),
  CONSTRAINT hias_not_merged_into_self CHECK (merged_into_hia_id IS NULL OR merged_into_hia_id <> id),
  CONSTRAINT hias_merge_target_matches_status CHECK ((status = 'MERGED') = (merged_into_hia_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS hias_home_park_idx ON hias (home_park_id);
CREATE INDEX IF NOT EXISTS hias_merged_into_idx ON hias (merged_into_hia_id);

CREATE OR REPLACE FUNCTION hias_guard_immutable_identity() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.id <> OLD.id OR NEW.public_number <> OLD.public_number THEN
    RAISE EXCEPTION 'hia id and public code are immutable' USING ERRCODE = 'check_violation';
  END IF;
  RETURN NEW;
END;
$$;
CREATE OR REPLACE TRIGGER hias_immutable_identity
  BEFORE UPDATE ON hias FOR EACH ROW EXECUTE FUNCTION hias_guard_immutable_identity();

-- Encounter module ----------------------------------------------------------
-- No hia_id column: linking an encounter to a Hia belongs to the future
-- Identification/Verification boundary.
CREATE TABLE IF NOT EXISTS encounters (
  id UUID PRIMARY KEY,
  observer_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  captured_at TIMESTAMPTZ NOT NULL,
  submitted_at TIMESTAMPTZ NULL,
  park_id UUID NULL REFERENCES parks(id) ON DELETE RESTRICT,
  zone_id UUID NULL,
  status TEXT NOT NULL,
  behavior TEXT NULL,
  notes TEXT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT encounters_status_valid CHECK (status IN ('DRAFT', 'SUBMITTED', 'PROCESSING', 'NEEDS_REVIEW', 'CONFIRMED', 'REJECTED')),
  CONSTRAINT encounters_behavior_valid CHECK (behavior IS NULL OR behavior IN ('BASKING', 'SWIMMING', 'WALKING', 'RESTING', 'EATING', 'CLIMBING', 'OTHER')),
  CONSTRAINT encounters_notes_length CHECK (notes IS NULL OR char_length(notes) <= 2000),
  CONSTRAINT encounters_zone_requires_park CHECK (zone_id IS NULL OR park_id IS NOT NULL),
  CONSTRAINT encounters_submitted_at_matches_status CHECK ((status = 'DRAFT') = (submitted_at IS NULL)),
  CONSTRAINT encounters_zone_in_park FOREIGN KEY (zone_id, park_id) REFERENCES zones (id, park_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS encounters_observer_captured_idx ON encounters (observer_user_id, captured_at DESC, id);
CREATE INDEX IF NOT EXISTS encounters_park_idx ON encounters (park_id);

-- Precise location is private personal data, stored apart from the encounter.
--
-- Casting an out-of-range point to geography does NOT fail: PostGIS coerces
-- it into range with only a NOTICE (latitude 95 is stored as 85). A CHECK on
-- the geography column therefore cannot see the original value, so every
-- write goes through encounter_location_point(), which rejects out-of-range
-- or non-finite input before the cast.
CREATE OR REPLACE FUNCTION encounter_location_point(longitude DOUBLE PRECISION, latitude DOUBLE PRECISION)
RETURNS public.geography
LANGUAGE plpgsql IMMUTABLE STRICT AS $$
BEGIN
  -- NaN compares greater than every number, so BETWEEN rejects it too.
  IF NOT (latitude BETWEEN -90 AND 90) OR NOT (longitude BETWEEN -180 AND 180) THEN
    RAISE EXCEPTION 'encounter location is out of range'
      USING ERRCODE = 'check_violation', CONSTRAINT = 'encounter_locations_point_range';
  END IF;
  RETURN public.ST_SetSRID(public.ST_MakePoint(longitude, latitude), 4326)::public.geography;
END;
$$;

CREATE TABLE IF NOT EXISTS encounter_locations (
  encounter_id UUID PRIMARY KEY REFERENCES encounters(id) ON DELETE CASCADE,
  point public.geography(Point, 4326) NOT NULL,
  accuracy_meters DOUBLE PRECISION NULL,
  source TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  -- Backstop only: coerced casts always pass this (see the function above).
  CONSTRAINT encounter_locations_point_range CHECK (
    public.ST_Y(point::public.geometry) BETWEEN -90 AND 90
    AND public.ST_X(point::public.geometry) BETWEEN -180 AND 180
  ),
  CONSTRAINT encounter_locations_accuracy_valid CHECK (
    accuracy_meters IS NULL OR (accuracy_meters >= 0 AND accuracy_meters < 'Infinity'::double precision)
  ),
  CONSTRAINT encounter_locations_source_valid CHECK (source IN ('GPS', 'MANUAL', 'IMPORT', 'UNKNOWN'))
);
CREATE INDEX IF NOT EXISTS encounter_locations_point_gix ON encounter_locations USING GIST (point);
