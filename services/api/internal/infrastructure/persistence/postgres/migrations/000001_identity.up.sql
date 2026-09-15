CREATE TABLE IF NOT EXISTS users (
  id UUID PRIMARY KEY,
  username TEXT NULL,
  display_name TEXT NULL,
  avatar_url TEXT NULL,
  locale TEXT NOT NULL DEFAULT 'th',
  status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'SUSPENDED', 'DELETED')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at TIMESTAMPTZ NULL,
  CONSTRAINT users_username_format CHECK (username IS NULL OR username ~ '^[A-Za-z0-9_]{3,30}$')
);
CREATE UNIQUE INDEX IF NOT EXISTS users_username_ci_unique ON users (lower(username)) WHERE username IS NOT NULL;
CREATE TABLE IF NOT EXISTS auth_identities (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,
  provider_subject TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_login_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT auth_identities_provider_subject_unique UNIQUE(provider, provider_subject)
);
CREATE INDEX IF NOT EXISTS auth_identities_user_id_idx ON auth_identities(user_id);
CREATE TABLE IF NOT EXISTS user_roles (
  user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('USER','CONTRIBUTOR','VERIFIER','MODERATOR','RESEARCHER','ADMIN')),
  PRIMARY KEY(user_id, role)
);
