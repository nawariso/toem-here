import 'react-native-url-polyfill/auto';
import { createClient, type SupabaseClient } from '@supabase/supabase-js';
import { secureSessionStorage } from './secure-storage';
import type { AuthProvider } from './controller';

export function createSupabaseClient(url: string, publishableKey: string): SupabaseClient {
  if (!url || !publishableKey) throw new Error('EXPO_PUBLIC_SUPABASE_URL and EXPO_PUBLIC_SUPABASE_PUBLISHABLE_KEY are required');
  return createClient(url, publishableKey, {
    auth: { storage: secureSessionStorage, autoRefreshToken: true, persistSession: true, detectSessionInUrl: false },
  });
}

export function createSupabaseAuthProvider(client: SupabaseClient): AuthProvider {
  return {
    async getAccessToken() { const { data, error } = await client.auth.getSession(); if (error) throw error; return data.session?.access_token ?? null; },
    async requestEmailOtp(email) { const { error } = await client.auth.signInWithOtp({ email, options: { shouldCreateUser: true } }); if (error) throw error; },
    async verifyEmailOtp(email, otp) { const { data, error } = await client.auth.verifyOtp({ email, token: otp, type: 'email' }); if (error) throw error; if (!data.session?.access_token) throw new Error('Authentication did not return a session'); return data.session.access_token; },
    async signOut() { const { error } = await client.auth.signOut({ scope: 'local' }); if (error) throw error; },
  };
}
