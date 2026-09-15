import { useEffect } from 'react';
import { ActivityIndicator, StyleSheet, Text, View } from 'react-native';
import { router } from 'expo-router';
import { useAuth } from '../src/auth/AuthContext';
import { routeForState } from '../src/auth/state';

export default function Splash() {
  const { state } = useAuth();

  useEffect(() => {
    if (state.status !== 'UNKNOWN') router.replace(routeForState(state));
  }, [state]);

  return (
    <View style={styles.screen}>
      <Text style={styles.mark}>TOEM HERE</Text>
      <Text style={styles.subtitle}>Every Hia has a story.</Text>
      <ActivityIndicator color="#d96c39" style={styles.loader} />
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: '#102f2a' },
  mark: { fontSize: 36, fontWeight: '900', letterSpacing: 2, color: '#f4f1e8' },
  subtitle: { marginTop: 8, color: '#b7c9bf' },
  loader: { marginTop: 28 },
});
