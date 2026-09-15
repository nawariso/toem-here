import { createAuthController, type AuthProvider, type ApiClient } from '../src/auth/controller';

const user = { id: 'u1', username: null, displayName: null, avatarUrl: null, locale: 'th', roles: ['USER'], profileComplete: false };

function providers() {
  const auth: jest.Mocked<AuthProvider> = {
    getAccessToken: jest.fn().mockResolvedValue(null),
    requestEmailOtp: jest.fn().mockResolvedValue(undefined),
    verifyEmailOtp: jest.fn().mockResolvedValue('jwt'),
    signOut: jest.fn().mockResolvedValue(undefined),
  };
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
});
