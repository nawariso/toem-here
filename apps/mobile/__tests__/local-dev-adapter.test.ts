import { createLocalDevAuthProvider, LOCAL_DEV_CREDENTIAL, type SessionStorage } from '../src/auth/local-dev-adapter';

function memoryStorage(): SessionStorage & { values: Map<string, string> } {
  const values = new Map<string, string>();
  return {
    values,
    getItem: async (key) => values.get(key) ?? null,
    setItem: async (key, value) => {
      values.set(key, value);
    },
    removeItem: async (key) => {
      values.delete(key);
    },
  };
}

describe('LocalDevAuthProvider', () => {
  it('matches the backend development credential and is identified as local', () => {
    // Must equal identity.LocalDevCredential in services/api/internal/infrastructure/identity/local.go.
    expect(LOCAL_DEV_CREDENTIAL).toBe('toem-local-dev.developer-001');
    expect(createLocalDevAuthProvider(memoryStorage()).kind).toBe('local');
  });

  it('has no session until the developer explicitly signs in, then restores it', async () => {
    const storage = memoryStorage();
    const provider = createLocalDevAuthProvider(storage);
    await expect(provider.getAccessToken()).resolves.toBeNull();

    await expect(provider.signInDevelopmentUser?.()).resolves.toBe(LOCAL_DEV_CREDENTIAL);
    await expect(createLocalDevAuthProvider(storage).getAccessToken()).resolves.toBe(LOCAL_DEV_CREDENTIAL);
  });

  it('never sends a client-chosen user id, role, or status', async () => {
    const provider = createLocalDevAuthProvider(memoryStorage());
    const credential = await provider.signInDevelopmentUser?.();
    expect(credential).not.toMatch(/userId|role|status|permission|\{/i);
  });

  it('ignores tampered stored values instead of forwarding them', async () => {
    const storage = memoryStorage();
    await storage.setItem('toem-here.local-dev-session', 'user-id-00000000');
    await expect(createLocalDevAuthProvider(storage).getAccessToken()).resolves.toBeNull();
  });

  it('sign-out clears the local session', async () => {
    const storage = memoryStorage();
    const provider = createLocalDevAuthProvider(storage);
    await provider.signInDevelopmentUser?.();
    await provider.signOut();
    await expect(provider.getAccessToken()).resolves.toBeNull();
    expect(storage.values.size).toBe(0);
  });

  it('does not pretend to deliver email OTP', async () => {
    const provider = createLocalDevAuthProvider(memoryStorage());
    await expect(provider.requestEmailOtp('person@example.com')).rejects.toThrow('unavailable');
    await expect(provider.verifyEmailOtp('person@example.com', '123456')).rejects.toThrow('unavailable');
  });
});
