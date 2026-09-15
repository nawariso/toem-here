/* eslint-disable import/first -- jest.mock factories must be declared before the modules they mock are imported. */
import React from 'react';
import { render, waitFor, fireEvent, type RenderResult } from '@testing-library/react-native';

// ES imports are hoisted above const initializers, so a jest.mock factory must
// create its own mocks; the test reads them back from the mocked module.
jest.mock('expo-router', () => ({
  router: { replace: jest.fn(), push: jest.fn() },
  Stack: Object.assign(({ children }: { children?: React.ReactNode }) => children ?? null, {
    Screen: () => null,
  }),
}));

jest.mock('expo-secure-store', () => ({
  getItemAsync: jest.fn().mockResolvedValue(null),
  setItemAsync: jest.fn().mockResolvedValue(undefined),
  deleteItemAsync: jest.fn().mockResolvedValue(undefined),
}));

jest.mock('../src/auth/controller', () => {
  const controller = {
    restore: jest.fn(),
    requestOtp: jest.fn(),
    verifyOtp: jest.fn(),
    updateProfile: jest.fn(),
    signOut: jest.fn(),
  };
  return { createAuthController: () => controller, __controller: controller };
});
jest.mock('../src/auth/supabase-adapter', () => ({
  createSupabaseClient: () => ({}),
  createSupabaseAuthProvider: () => ({}),
}));
jest.mock('../src/api/client', () => ({ createApiClient: () => ({}) }));

import { router } from 'expo-router';
import { AuthProvider } from '../src/auth/AuthContext';
import Splash from '../app/index';
import Home from '../app/home';
import Passport from '../app/passport';
import PassportSetup from '../app/passport-setup';
import Profile from '../app/profile';

const replace = router.replace as unknown as jest.Mock;
const push = router.push as unknown as jest.Mock;
const controller = (
  jest.requireMock('../src/auth/controller') as {
    __controller: Record<'restore' | 'requestOtp' | 'verifyOtp' | 'updateProfile' | 'signOut', jest.Mock>;
  }
).__controller;

const incomplete = {
  id: 'u1',
  username: null,
  displayName: null,
  avatarUrl: null,
  locale: 'th',
  roles: ['USER'],
  profileComplete: false,
};
const complete = { ...incomplete, username: 'mickey', displayName: 'Mickey', profileComplete: true };

// render and fireEvent are async and already act-wrapped in RNTL 14; wrapping
// them in another act(...) creates overlapping act scopes and drops the event.
function renderScreen(Screen: React.ComponentType): Promise<RenderResult> {
  return render(
    <AuthProvider>
      <Screen />
    </AuthProvider>,
  );
}

async function renderSettled(Screen: React.ComponentType): Promise<RenderResult> {
  const view = await renderScreen(Screen);
  await waitFor(() => expect(controller.restore).toHaveBeenCalled());
  return view;
}

beforeEach(() => {
  jest.clearAllMocks();
  controller.restore.mockResolvedValue(null);
  controller.requestOtp.mockResolvedValue(undefined);
  controller.verifyOtp.mockResolvedValue(incomplete);
  controller.updateProfile.mockResolvedValue(complete);
  controller.signOut.mockResolvedValue(undefined);
});

describe('guest state', () => {
  it('sends a launching app with no session from Splash to Home', async () => {
    const view = await renderScreen(Splash);
    expect(view.getByText('TOEM HERE')).toBeTruthy();
    await waitFor(() => expect(replace).toHaveBeenCalledWith('/home'));
  });

  it('shows the Coming Soon placeholder and passport wording, never "Register Account"', async () => {
    const view = await renderSettled(Home);
    expect(view.getByText('Scan a Hia')).toBeTruthy();
    expect(view.getByText('COMING SOON')).toBeTruthy();
    expect(view.getByText('Create Your Hia Passport')).toBeTruthy();
    expect(view.queryByText(/Register Account/i)).toBeNull();
  });

  it('navigates a guest from Home to the passport screen', async () => {
    const view = await renderSettled(Home);
    await fireEvent.press(view.getByText('Create Your Hia Passport'));
    expect(push).toHaveBeenCalledWith('/passport');
  });
});

describe('login navigation', () => {
  it('requests an OTP, verifies it, and routes an incomplete profile to Passport Setup', async () => {
    const view = await renderSettled(Passport);

    await fireEvent.changeText(view.getByPlaceholderText('you@example.com'), 'person@example.com');
    await fireEvent.press(view.getByText('Email me a code'));
    expect(controller.requestOtp).toHaveBeenCalledWith('person@example.com');

    await fireEvent.changeText(view.getByPlaceholderText('6-digit code'), '123456');
    await fireEvent.press(view.getByText('Verify code'));
    expect(controller.verifyOtp).toHaveBeenCalledWith('person@example.com', '123456');
    await waitFor(() => expect(replace).toHaveBeenCalledWith('/passport-setup'));
  });

  it('surfaces a provider failure instead of navigating', async () => {
    controller.requestOtp.mockRejectedValue(new Error('Email rate limit exceeded'));
    const view = await renderSettled(Passport);

    await fireEvent.changeText(view.getByPlaceholderText('you@example.com'), 'person@example.com');
    await fireEvent.press(view.getByText('Email me a code'));

    await waitFor(() => expect(view.getByText('Email rate limit exceeded')).toBeTruthy());
    expect(replace).not.toHaveBeenCalledWith('/passport-setup');
  });
});

describe('authenticated states', () => {
  it('routes a restored complete profile to Home', async () => {
    controller.restore.mockResolvedValue(complete);
    await renderScreen(Splash);
    await waitFor(() => expect(replace).toHaveBeenCalledWith('/home'));
  });

  it('routes a restored incomplete profile to Passport Setup', async () => {
    controller.restore.mockResolvedValue(incomplete);
    await renderScreen(Splash);
    await waitFor(() => expect(replace).toHaveBeenCalledWith('/passport-setup'));
  });

  it('offers Hia Passport access on Home once authenticated', async () => {
    controller.restore.mockResolvedValue(complete);
    const view = await renderSettled(Home);
    await waitFor(() => expect(view.getByText('View Hia Passport')).toBeTruthy());
    await fireEvent.press(view.getByText('View Hia Passport'));
    expect(push).toHaveBeenCalledWith('/profile');
  });

  it('completes passport setup and moves the user to Profile', async () => {
    controller.restore.mockResolvedValue(incomplete);
    const view = await renderSettled(PassportSetup);

    await fireEvent.changeText(view.getByPlaceholderText('mickey'), 'mickey');
    await fireEvent.changeText(view.getByPlaceholderText('Mickey'), 'Mickey');
    await fireEvent.press(view.getByText('Create Hia Passport'));

    expect(controller.updateProfile).toHaveBeenCalledWith({ username: 'mickey', displayName: 'Mickey', locale: 'th' });
    await waitFor(() => expect(replace).toHaveBeenCalledWith('/profile'));
  });

  it('shows a taken username instead of navigating away', async () => {
    controller.restore.mockResolvedValue(incomplete);
    controller.updateProfile.mockRejectedValue(new Error('Username is already taken'));
    const view = await renderSettled(PassportSetup);

    await fireEvent.changeText(view.getByPlaceholderText('mickey'), 'mickey');
    await fireEvent.changeText(view.getByPlaceholderText('Mickey'), 'Mickey');
    await fireEvent.press(view.getByText('Create Hia Passport'));

    await waitFor(() => expect(view.getByText('Username is already taken')).toBeTruthy());
    expect(replace).not.toHaveBeenCalledWith('/profile');
  });

  it('shows the internal user and role on Profile', async () => {
    controller.restore.mockResolvedValue(complete);
    const view = await renderSettled(Profile);
    await waitFor(() => expect(view.getByText('Mickey')).toBeTruthy());
    expect(view.getByText('@mickey')).toBeTruthy();
    expect(view.getByText('USER')).toBeTruthy();
  });
});

describe('logout navigation', () => {
  it('signs out through the provider and returns to guest Home', async () => {
    controller.restore.mockResolvedValue(complete);
    const view = await renderSettled(Profile);
    await waitFor(() => expect(view.getByText('Mickey')).toBeTruthy());

    await fireEvent.press(view.getByText('Log out'));

    expect(controller.signOut).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(replace).toHaveBeenCalledWith('/home'));
  });
});
