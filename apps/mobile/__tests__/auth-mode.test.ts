import { AuthConfigError, resolveAuthConfig, type PublicEnv } from '../src/config/auth-mode';

const localEnv: PublicEnv = {
  appEnv: 'development',
  authMode: 'local',
  apiUrl: 'http://192.168.1.20:8080',
  supabaseUrl: undefined,
  supabasePublishableKey: undefined,
};
const supabaseEnv: PublicEnv = {
  appEnv: 'production',
  authMode: 'supabase',
  apiUrl: 'https://api.example',
  supabaseUrl: 'https://project.supabase.co',
  supabasePublishableKey: 'sb_publishable_x',
};

describe('auth mode resolution', () => {
  it('development + local resolves without any Supabase configuration', () => {
    expect(resolveAuthConfig(localEnv, true)).toEqual({ mode: 'local', apiUrl: 'http://192.168.1.20:8080' });
  });

  it('production + supabase with valid configuration resolves', () => {
    expect(resolveAuthConfig(supabaseEnv, false)).toMatchObject({ mode: 'supabase', supabaseUrl: 'https://project.supabase.co' });
  });

  it.each(['EXPO_PUBLIC_SUPABASE_URL', 'EXPO_PUBLIC_SUPABASE_PUBLISHABLE_KEY'] as const)(
    'supabase mode fails fast when %s is missing',
    (name) => {
      const env = { ...supabaseEnv, appEnv: 'development' };
      if (name === 'EXPO_PUBLIC_SUPABASE_URL') env.supabaseUrl = ' ';
      else env.supabasePublishableKey = undefined;
      expect(() => resolveAuthConfig(env, true)).toThrow(name);
    },
  );

  it('production + local is refused', () => {
    expect(() => resolveAuthConfig({ ...localEnv, appEnv: 'production' }, true)).toThrow(AuthConfigError);
  });

  it('local mode is refused in a release bundle even when APP_ENV says development', () => {
    expect(() => resolveAuthConfig(localEnv, false)).toThrow(AuthConfigError);
  });

  it.each([undefined, '', 'staging', 'dev', 'Development'])('local mode is refused for APP_ENV=%p', (appEnv) => {
    expect(() => resolveAuthConfig({ ...localEnv, appEnv }, true)).toThrow(AuthConfigError);
  });

  it.each([undefined, '', 'none', 'LOCAL', 'disabled'])('has no implicit auth mode (%p fails)', (authMode) => {
    expect(() => resolveAuthConfig({ ...localEnv, authMode }, true)).toThrow(AuthConfigError);
  });

  it('requires the API URL in every mode', () => {
    expect(() => resolveAuthConfig({ ...localEnv, apiUrl: '' }, true)).toThrow('EXPO_PUBLIC_API_URL');
  });
});
