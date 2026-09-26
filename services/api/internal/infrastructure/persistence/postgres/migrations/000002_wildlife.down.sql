-- Reverts 000002 only; Requirement 001 identity tables are untouched.
-- Drops are qualified with current_schema() so an unqualified name can never
-- fall through the search_path to another schema's table of the same name.
-- The PostGIS extension is database-wide shared infrastructure and is kept.
DO $$
BEGIN
  EXECUTE format('DROP TABLE IF EXISTS %I.encounter_locations', current_schema());
  EXECUTE format('DROP FUNCTION IF EXISTS %I.encounter_location_point(double precision, double precision)', current_schema());
  EXECUTE format('DROP TABLE IF EXISTS %I.encounters', current_schema());
  EXECUTE format('DROP TABLE IF EXISTS %I.hias', current_schema());
  EXECUTE format('DROP FUNCTION IF EXISTS %I.hias_guard_immutable_identity()', current_schema());
  EXECUTE format('DROP TABLE IF EXISTS %I.zones', current_schema());
  EXECUTE format('DROP TABLE IF EXISTS %I.parks', current_schema());
END
$$;
