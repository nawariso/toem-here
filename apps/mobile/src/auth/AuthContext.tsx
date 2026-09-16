import React, { createContext, useCallback, useContext, useEffect, useMemo, useReducer, useState } from 'react';
import { createApiClient } from '../api/client';
import { createAuthController, type ProfileInput } from './controller';
import {
  createSupabaseAuthProvider,
  createSupabaseClient,
  manageSupabaseAutoRefresh,
} from './supabase-adapter';
import { initialAuthState, reduceAuth, type AuthState } from './state';

const supabaseUrl = process.env.EXPO_PUBLIC_SUPABASE_URL ?? '';
const supabaseKey = process.env.EXPO_PUBLIC_SUPABASE_PUBLISHABLE_KEY ?? '';
const apiUrl = process.env.EXPO_PUBLIC_API_URL ?? '';

const CONFIG_ERROR = 'App configuration is incomplete';

type ContextValue = {
  state: AuthState;
  error: string | null;
  requestOtp(email: string): Promise<void>;
  verifyOtp(email: string, otp: string): Promise<void>;
  updateProfile(input: ProfileInput): Promise<void>;
  signOut(): Promise<void>;
};

const AuthContext = createContext<ContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(reduceAuth, initialAuthState);
  const [error, setError] = useState<string | null>(null);

  // Built once so a misconfigured build degrades to GUEST instead of crashing.
  const authServices = useMemo(() => {
    try {
      const client = createSupabaseClient(supabaseUrl, supabaseKey);
      return {
        client,
        controller: createAuthController(createSupabaseAuthProvider(client), createApiClient(apiUrl)),
      };
    } catch {
      return null;
    }
  }, []);
  const controller = authServices?.controller ?? null;

  useEffect(() => {
    if (!authServices) return;
    return manageSupabaseAutoRefresh(authServices.client);
  }, [authServices]);

  useEffect(() => {
    let active = true;
    (async () => {
      if (!controller) {
        if (active) {
          setError(CONFIG_ERROR);
          dispatch({ type: 'SESSION_RESOLVED' });
        }
        return;
      }
      try {
        const user = await controller.restore();
        if (!active) return;
        if (user) dispatch({ type: 'BOOTSTRAP_SUCCEEDED', user });
        else dispatch({ type: 'SESSION_RESOLVED' });
      } catch {
        if (!active) return;
        setError('Unable to restore session');
        dispatch({ type: 'SESSION_RESOLVED' });
      }
    })();
    return () => {
      active = false;
    };
  }, [controller]);

  const run = useCallback(async (action: () => Promise<void>) => {
    setError(null);
    try {
      await action();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Request failed');
      throw cause;
    }
  }, []);

  const value = useMemo<ContextValue>(
    () => ({
      state,
      error,
      requestOtp: (email) =>
        run(async () => {
          if (!controller) throw new Error(CONFIG_ERROR);
          await controller.requestOtp(email);
        }),
      verifyOtp: (email, otp) =>
        run(async () => {
          if (!controller) throw new Error(CONFIG_ERROR);
          const user = await controller.verifyOtp(email, otp);
          dispatch({ type: 'BOOTSTRAP_SUCCEEDED', user });
        }),
      updateProfile: (input) =>
        run(async () => {
          if (!controller) throw new Error(CONFIG_ERROR);
          const user = await controller.updateProfile(input);
          dispatch({ type: 'PROFILE_UPDATED', user });
        }),
      signOut: () =>
        run(async () => {
          if (controller) await controller.signOut();
          dispatch({ type: 'SIGNED_OUT' });
        }),
    }),
    [controller, error, run, state],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): ContextValue {
  const value = useContext(AuthContext);
  if (!value) throw new Error('useAuth must be used inside AuthProvider');
  return value;
}
