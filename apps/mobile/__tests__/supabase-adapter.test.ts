import type { SupabaseClient } from '@supabase/supabase-js';
import type { AppStateStatus } from 'react-native';
import { manageSupabaseAutoRefresh } from '../src/auth/supabase-adapter';

type StateListener = (state: AppStateStatus) => void;

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function settle(): Promise<void> {
  return new Promise((resolve) => setImmediate(resolve));
}

function harness(initialState: AppStateStatus = 'active', initializationGate?: Promise<void>) {
  const listeners: { active: boolean; listener: StateListener }[] = [];
  const remove = jest.fn();
  const appState = {
    currentState: initialState,
    addEventListener: jest.fn((_event: 'change', nextListener: StateListener) => {
      const registration = { active: true, listener: nextListener };
      listeners.push(registration);
      return {
        remove: () => {
          registration.active = false;
          remove();
        },
      };
    }),
  };
  const auth = {
    startAutoRefresh: jest.fn(),
    stopAutoRefresh: jest.fn(),
    initialize: jest.fn(),
  };
  const initialization = initializationGate
    ? initializationGate.then(() => {
        // Supabase does this internally at the end of React Native initialization.
        auth.startAutoRefresh();
        return { error: null };
      })
    : Promise.resolve({ error: null });
  auth.initialize.mockReturnValue(initialization);
  const client = { auth } as unknown as SupabaseClient;

  return {
    appState,
    auth,
    remove,
    change(state: AppStateStatus) {
      const activeListeners = listeners.filter((registration) => registration.active);
      if (activeListeners.length === 0) throw new Error('AppState listener was not registered');
      for (const registration of activeListeners) registration.listener(state);
    },
    client,
  };
}

describe('Supabase React Native auto-refresh lifecycle', () => {
  it('refreshes only while active and removes the listener during cleanup', async () => {
    const test = harness('active');

    const cleanup = manageSupabaseAutoRefresh(test.client, test.appState);
    await settle();

    expect(test.appState.addEventListener).toHaveBeenCalledTimes(1);
    expect(test.auth.startAutoRefresh).toHaveBeenCalledTimes(1);
    expect(test.auth.stopAutoRefresh).not.toHaveBeenCalled();

    test.change('background');
    await settle();
    expect(test.auth.stopAutoRefresh).toHaveBeenCalledTimes(1);

    test.change('active');
    await settle();
    expect(test.auth.startAutoRefresh).toHaveBeenCalledTimes(2);

    cleanup();
    await settle();
    expect(test.remove).toHaveBeenCalledTimes(1);
    expect(test.auth.stopAutoRefresh).toHaveBeenCalledTimes(2);
  });

  it('does not start refresh when registered while backgrounded', async () => {
    const test = harness('background');

    manageSupabaseAutoRefresh(test.client, test.appState);
    await settle();

    expect(test.auth.startAutoRefresh).not.toHaveBeenCalled();
    expect(test.auth.stopAutoRefresh).toHaveBeenCalledTimes(1);
  });

  it('reasserts background state after Supabase initialization starts refresh', async () => {
    const gate = deferred();
    const test = harness('active', gate.promise);

    manageSupabaseAutoRefresh(test.client, test.appState);
    test.change('background');
    gate.resolve();
    await settle();

    expect(test.auth.startAutoRefresh).toHaveBeenCalledTimes(1);
    expect(test.auth.stopAutoRefresh).toHaveBeenCalledTimes(1);
    expect(test.auth.stopAutoRefresh.mock.invocationCallOrder[0]).toBeGreaterThan(
      test.auth.startAutoRefresh.mock.invocationCallOrder[0]!,
    );
  });

  it('stops refresh again when initialization finishes after cleanup', async () => {
    const gate = deferred();
    const test = harness('active', gate.promise);

    const cleanup = manageSupabaseAutoRefresh(test.client, test.appState);
    cleanup();
    gate.resolve();
    await settle();

    expect(test.remove).toHaveBeenCalledTimes(1);
    expect(test.auth.startAutoRefresh).toHaveBeenCalledTimes(1);
    expect(test.auth.stopAutoRefresh).toHaveBeenCalledTimes(2);
    expect(test.auth.stopAutoRefresh.mock.invocationCallOrder[1]).toBeGreaterThan(
      test.auth.startAutoRefresh.mock.invocationCallOrder[0]!,
    );
  });

  it('leaves only the current active lifecycle refreshing after a Strict Mode remount', async () => {
    const gate = deferred();
    const test = harness('active', gate.promise);

    const cleanupFirstMount = manageSupabaseAutoRefresh(test.client, test.appState);
    cleanupFirstMount();
    manageSupabaseAutoRefresh(test.client, test.appState);
    gate.resolve();
    await settle();

    expect(test.appState.addEventListener).toHaveBeenCalledTimes(2);
    expect(test.remove).toHaveBeenCalledTimes(1);
    const lastStart = test.auth.startAutoRefresh.mock.invocationCallOrder.at(-1)!;
    const lastStop = test.auth.stopAutoRefresh.mock.invocationCallOrder.at(-1);
    expect(lastStart).toBeDefined();
    if (lastStop !== undefined) expect(lastStart).toBeGreaterThan(lastStop);
  });
});
