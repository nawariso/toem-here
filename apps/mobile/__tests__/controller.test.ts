import { createAuthController, type AuthProvider, type ApiClient } from '../src/auth/controller';

const user = { id: 'u1', username: null, displayName: null, avatarUrl: null, locale: 'th', roles: ['USER'], profileComplete: false };

function providers(kind: 'local' | 'supabase' = 'supabase') {
  const auth: jest.Mocked<AuthProvider> = {
    kind,
    getAccessToken: jest.fn().mockResolvedValue(null),
    requestEmailOtp: jest.fn().mockResolvedValue(undefined),
    verifyEmailOtp: jest.fn().mockResolvedValue('jwt'),
    signOut: jest.fn().mockResolvedValue(undefined),
  };
  if (kind === 'local') auth.signInDevelopmentUser = jest.fn().mockResolvedValue('dev-credential');
  const api: jest.Mocked<ApiClient> = {
    bootstrap: jest.fn().mockResolvedValue(user),
    getMe: jest.fn().mockResolvedValue(user),
    updateMe: jest.fn().mockResolvedValue({ ...user, username: 'mickey', displayName: 'Mickey', profileComplete: true }),
  };
  return { auth, api };
}

describe('auth controller', () => {
  it('requests and verifies email OTP then bootstraps the internal user', async () => {
    const { auth, api } = providers(); const controller = createAuthController(auth, api);
    await controller.requestOtp('person@example.com');
    const result = await controller.verifyOtp('person@example.com', '123456');
    expect(auth.requestEmailOtp).toHaveBeenCalledWith('person@example.com');
    expect(auth.verifyEmailOtp).toHaveBeenCalledWith('person@example.com', '123456');
    expect(api.bootstrap).toHaveBeenCalledWith('jwt');
    expect(result.profileComplete).toBe(false);
  });

  it('restores an authenticated provider session through bootstrap', async () => {
    const { auth, api } = providers(); auth.getAccessToken.mockResolvedValue('restored-jwt');
    const controller = createAuthController(auth, api);
    await expect(controller.restore()).resolves.toEqual(user);
    expect(api.bootstrap).toHaveBeenCalledWith('restored-jwt');
  });

  it('signs out through provider and leaves no session token', async () => {
    const { auth, api } = providers(); const controller = createAuthController(auth, api);
    await controller.signOut();
    expect(auth.signOut).toHaveBeenCalledTimes(1);
  });

  it('local dev sign-in sends only the credential through the normal bootstrap', async () => {
    const { auth, api } = providers('local'); const controller = createAuthController(auth, api);
    expect(controller.mode).toBe('local');
    await expect(controller.signInDevelopmentUser()).resolves.toEqual(user);
    expect(api.bootstrap).toHaveBeenCalledWith('dev-credential');
    expect(auth.requestEmailOtp).not.toHaveBeenCalled();
  });

  it('local dev sign-in clears the local session when bootstrap fails', async () => {
    const { auth, api } = providers('local'); api.bootstrap.mockRejectedValue(new Error('API unreachable'));
    await expect(createAuthController(auth, api).signInDevelopmentUser()).rejects.toThrow('API unreachable');
    expect(auth.signOut).toHaveBeenCalledTimes(1);
  });

  it('refuses development sign-in through the Supabase adapter', async () => {
    const { auth, api } = providers('supabase'); const controller = createAuthController(auth, api);
    expect(controller.mode).toBe('supabase');
    await expect(controller.signInDevelopmentUser()).rejects.toThrow('not available');
    expect(api.bootstrap).not.toHaveBeenCalled();
  });
});
