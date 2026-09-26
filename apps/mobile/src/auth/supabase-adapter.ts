import 'react-native-url-polyfill/auto';
import { createClient, type SupabaseClient } from '@supabase/supabase-js';
import { AppState, type AppStateStatus } from 'react-native';
import { secureSessionStorage } from './secure-storage';
import type { AuthProvider } from './controller';

type AppStateSource = {
  currentState: AppStateStatus;
  addEventListener(
    event: 'change',
    listener: (state: AppStateStatus) => void,
  ): { remove(): void };
};

const refreshLifecycleGenerations = new WeakMap<SupabaseClient, number>();

export function createSupabaseClient(url: string, publishableKey: string): SupabaseClient {
  if (!url || !publishableKey) throw new Error('EXPO_PUBLIC_SUPABASE_URL and EXPO_PUBLIC_SUPABASE_PUBLISHABLE_KEY are required');
  return createClient(url, publishableKey, {
    auth: { storage: secureSessionStorage, autoRefreshToken: true, persistSession: true, detectSessionInUrl: false },
  });
}

export function manageSupabaseAutoRefresh(
  client: SupabaseClient,
  appState: AppStateSource = AppState,
): () => void {
  const generation = (refreshLifecycleGenerations.get(client) ?? 0) + 1;
  refreshLifecycleGenerations.set(client, generation);
  let currentState = appState.currentState;
  let disposed = false;
  let initialized = false;
  let pendingRefreshChange = Promise.resolve();

  const scheduleRefresh = (state: AppStateStatus) => {
    pendingRefreshChange = pendingRefreshChange
      .then(async () => {
        if (refreshLifecycleGenerations.get(client) !== generation) return;
        if (!disposed && state === 'active') await client.auth.startAutoRefresh();
        else await client.auth.stopAutoRefresh();
      })
      .catch(() => undefined);
  };

  const subscription = appState.addEventListener('change', (state) => {
    currentState = state;
    if (initialized) scheduleRefresh(state);
  });

  void client.auth.initialize().then(
    () => {
      initialized = true;
      scheduleRefresh(currentState);
    },
    () => {
      initialized = true;
      scheduleRefresh('background');
    },
  );

  return () => {
    disposed = true;
    subscription.remove();
    scheduleRefresh('background');
  };
}

export function createSupabaseAuthProvider(client: SupabaseClient): AuthProvider {
  return {
    kind: 'supabase',
    async getAccessToken() { const { data, error } = await client.auth.getSession(); if (error) throw error; return data.session?.access_token ?? null; },
    async requestEmailOtp(email) { const { error } = await client.auth.signInWithOtp({ email, options: { shouldCreateUser: true } }); if (error) throw error; },
    async verifyEmailOtp(email, otp) { const { data, error } = await client.auth.verifyOtp({ email, token: otp, type: 'email' }); if (error) throw error; if (!data.session?.access_token) throw new Error('Authentication did not return a session'); return data.session.access_token; },
    async signOut() { const { error } = await client.auth.signOut({ scope: 'local' }); if (error) throw error; },
  };
}
