// Mobile authentication mode resolution.
//
// LOCAL AUTH IS DEVELOPMENT ONLY. IT MUST NEVER BE ENABLED IN PRODUCTION.
//
// Local mode requires all three of:
//   EXPO_PUBLIC_AUTH_MODE=local
//   EXPO_PUBLIC_APP_ENV in {development, test}
//   a development bundle (__DEV__ === true; release/production builds are false)
// Anything else is either Supabase mode or a configuration error. There is no
// implicit default and no fallback between modes.

export type AuthMode = 'local' | 'supabase';

export type PublicEnv = {
  appEnv: string | undefined;
  authMode: string | undefined;
  apiUrl: string | undefined;
  supabaseUrl: string | undefined;
  supabasePublishableKey: string | undefined;
};

export type AuthConfig =
  | { mode: 'local'; apiUrl: string }
  | { mode: 'supabase'; apiUrl: string; supabaseUrl: string; supabasePublishableKey: string };

export class AuthConfigError extends Error {}

const LOCAL_AUTH_APP_ENVS = new Set(['development', 'test']);
const APP_ENVS = new Set(['development', 'test', 'production']);

// Expo only inlines EXPO_PUBLIC_* values accessed with static dot notation, so
// every variable is read explicitly here and nowhere else.
export function readPublicEnv(): PublicEnv {
  return {
    appEnv: process.env.EXPO_PUBLIC_APP_ENV,
    authMode: process.env.EXPO_PUBLIC_AUTH_MODE,
    apiUrl: process.env.EXPO_PUBLIC_API_URL,
    supabaseUrl: process.env.EXPO_PUBLIC_SUPABASE_URL,
    supabasePublishableKey: process.env.EXPO_PUBLIC_SUPABASE_PUBLISHABLE_KEY,
  };
}

function required(name: string, value: string | undefined): string {
  const trimmed = value?.trim() ?? '';
  if (!trimmed) throw new AuthConfigError(`${name} is required`);
  return trimmed;
}

export function resolveAuthConfig(env: PublicEnv, isDevelopmentBundle: boolean): AuthConfig {
  const appEnv = required('EXPO_PUBLIC_APP_ENV', env.appEnv);
  if (!APP_ENVS.has(appEnv)) throw new AuthConfigError('EXPO_PUBLIC_APP_ENV must be development, test, or production');
  const mode = required('EXPO_PUBLIC_AUTH_MODE', env.authMode);
  const apiUrl = required('EXPO_PUBLIC_API_URL', env.apiUrl);

  if (mode === 'local') {
    if (!LOCAL_AUTH_APP_ENVS.has(appEnv) || !isDevelopmentBundle) {
      throw new AuthConfigError('Local development authentication is forbidden in this build');
    }
    return { mode: 'local', apiUrl };
  }
  if (mode === 'supabase') {
    return {
      mode: 'supabase',
      apiUrl,
      supabaseUrl: required('EXPO_PUBLIC_SUPABASE_URL', env.supabaseUrl),
      supabasePublishableKey: required('EXPO_PUBLIC_SUPABASE_PUBLISHABLE_KEY', env.supabasePublishableKey),
    };
  }
  throw new AuthConfigError('EXPO_PUBLIC_AUTH_MODE must be local or supabase');
}
