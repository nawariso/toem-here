import { secureSessionStorage } from '../src/auth/secure-storage';
import * as SecureStore from 'expo-secure-store';

jest.mock('expo-secure-store', () => ({
  getItemAsync: jest.fn().mockResolvedValue('stored'),
  setItemAsync: jest.fn().mockResolvedValue(undefined),
  deleteItemAsync: jest.fn().mockResolvedValue(undefined),
}));

test('Supabase session storage uses Expo SecureStore for reads, writes, and deletion', async () => {
  await expect(secureSessionStorage.getItem('session')).resolves.toBe('stored');
  await secureSessionStorage.setItem('session', 'secret-session');
  await secureSessionStorage.removeItem('session');
  expect(SecureStore.getItemAsync).toHaveBeenCalledWith('session');
  expect(SecureStore.setItemAsync).toHaveBeenCalledWith('session', 'secret-session');
  expect(SecureStore.deleteItemAsync).toHaveBeenCalledWith('session');
});
