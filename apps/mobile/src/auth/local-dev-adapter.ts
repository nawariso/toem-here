import type { AuthProvider } from './controller';

// LOCAL DEVELOPMENT ONLY. MUST NEVER BE ENABLED IN PRODUCTION.
//
// This adapter never tells the API who the user is. It holds a deterministic,
// non-sensitive development credential and sends it as a Bearer token; the Go
// API's LocalDevVerifier (services/api/internal/infrastructure/identity/local.go)
// resolves it to an external identity, and the normal bootstrap flow maps that
// to the internal User. The credential has no authority on a Supabase-mode API.
export const LOCAL_DEV_CREDENTIAL = 'toem-local-dev.developer-001';

const SESSION_KEY = 'toem-here.local-dev-session';
const OTP_UNAVAILABLE = 'Email OTP is unavailable in local development mode';

export type SessionStorage = {
  getItem(key: string): Promise<string | null>;
  setItem(key: string, value: string): Promise<void>;
  removeItem(key: string): Promise<void>;
};

export function createLocalDevAuthProvider(storage: SessionStorage): AuthProvider {
  return {
    kind: 'local',
    async getAccessToken() {
      return (await storage.getItem(SESSION_KEY)) === LOCAL_DEV_CREDENTIAL ? LOCAL_DEV_CREDENTIAL : null;
    },
    async signInDevelopmentUser() {
      await storage.setItem(SESSION_KEY, LOCAL_DEV_CREDENTIAL);
      return LOCAL_DEV_CREDENTIAL;
    },
    async requestEmailOtp() {
      throw new Error(OTP_UNAVAILABLE);
    },
    async verifyEmailOtp(): Promise<string> {
      throw new Error(OTP_UNAVAILABLE);
    },
    async signOut() {
      await storage.removeItem(SESSION_KEY);
    },
  };
}
