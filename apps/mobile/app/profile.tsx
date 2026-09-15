import { router } from 'expo-router';
import { Pressable, StyleSheet, Text, View } from 'react-native';
import { useAuth } from '../src/auth/AuthContext';

export default function Profile() {
  const { state, error, signOut } = useAuth();
  const user = state.user;

  async function logout() {
    try {
      await signOut();
      router.replace('/home');
    } catch {
      // The sign-out failure is surfaced through the auth context.
    }
  }

  if (!user) {
    return (
      <View style={styles.screen}>
        <Text style={styles.body}>Passport unavailable.</Text>
      </View>
    );
  }

  return (
    <View style={styles.screen}>
      <Text style={styles.label}>HIA PASSPORT</Text>
      <Text style={styles.name}>{user.displayName}</Text>
      <Text style={styles.username}>@{user.username}</Text>
      <View style={styles.card}>
        <Text style={styles.meta}>LOCALE</Text>
        <Text style={styles.value}>{user.locale}</Text>
        <Text style={styles.meta}>ROLE</Text>
        <Text style={styles.value}>{user.roles.join(', ')}</Text>
      </View>
      {error ? <Text style={styles.error}>{error}</Text> : null}
      <Pressable style={styles.button} onPress={logout}>
        <Text style={styles.buttonText}>Log out</Text>
      </Pressable>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, padding: 24, backgroundColor: '#f4f1e8' },
  label: { fontSize: 12, fontWeight: '800', letterSpacing: 1.5, color: '#d96c39' },
  name: { marginTop: 10, fontSize: 36, fontWeight: '900', color: '#102f2a' },
  username: { fontSize: 18, color: '#46615a' },
  body: { fontSize: 16, color: '#46615a' },
  card: { marginTop: 30, padding: 22, borderRadius: 16, backgroundColor: '#dce7df' },
  meta: { marginTop: 8, fontSize: 11, fontWeight: '800', letterSpacing: 1, color: '#6d827b' },
  value: { marginTop: 4, marginBottom: 12, fontSize: 17, fontWeight: '700', color: '#102f2a' },
  button: { marginTop: 'auto', padding: 17, borderRadius: 14, borderWidth: 1, borderColor: '#a62d26', alignItems: 'center' },
  buttonText: { fontWeight: '800', color: '#a62d26' },
  error: { marginTop: 14, color: '#a62d26' },
});
