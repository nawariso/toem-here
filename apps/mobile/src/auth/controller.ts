import type { User } from './state';

export type ProfileInput = { username?: string; displayName?: string; locale?: string };
export interface AuthProvider {
  getAccessToken(): Promise<string | null>;
  requestEmailOtp(email: string): Promise<void>;
  verifyEmailOtp(email: string, otp: string): Promise<string>;
  signOut(): Promise<void>;
}
export interface ApiClient {
  bootstrap(token: string): Promise<User>;
  getMe(token: string): Promise<User>;
  updateMe(token: string, patch: ProfileInput): Promise<User>;
}
export function createAuthController(auth: AuthProvider, api: ApiClient) {
  return {
    requestOtp: (email: string) => auth.requestEmailOtp(email.trim().toLowerCase()),
    async verifyOtp(email: string, otp: string) { const token = await auth.verifyEmailOtp(email.trim().toLowerCase(), otp.trim()); return api.bootstrap(token); },
    async restore() { const token = await auth.getAccessToken(); return token ? api.bootstrap(token) : null; },
    async updateProfile(patch: ProfileInput) { const token = await auth.getAccessToken(); if (!token) throw new Error('Authentication is required'); return api.updateMe(token, patch); },
    signOut: () => auth.signOut(),
  };
}
export type AuthController = ReturnType<typeof createAuthController>;
