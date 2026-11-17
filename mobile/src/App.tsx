import { StatusBar } from "expo-status-bar";
import { Platform, ScrollView, StyleSheet, Text, View } from "react-native";

export default function App() {
  return (
    <View style={styles.container}>
      <StatusBar style="dark" />
      <ScrollView contentContainerStyle={styles.content}>
        <Text style={styles.brand}>Prépa Maths CRPE</Text>
        <View style={styles.message}>
          <Text style={styles.eyebrow}>EN PRÉPARATION</Text>
          <Text accessibilityRole="header" style={styles.title}>
            Les maths du CRPE, pas à pas.
          </Text>
          <Text style={styles.description}>
            Votre espace de préparation aux mathématiques du CRPE prend forme.
          </Text>
          <Text style={styles.note}>
            L’application sera disponible prochainement.
          </Text>
        </View>
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#f7f5ef" },
  content: {
    flexGrow: 1,
    paddingHorizontal: 28,
    paddingTop: 80,
    paddingBottom: 48,
  },
  brand: { fontSize: 17, fontWeight: "600", color: "#183e35" },
  message: {
    flex: 1,
    justifyContent: "center",
    gap: 24,
    paddingVertical: 72,
    maxWidth: 520,
  },
  eyebrow: {
    fontSize: 11,
    fontWeight: "600",
    letterSpacing: 2,
    color: "#59655d",
  },
  title: {
    fontFamily: Platform.select({ ios: "Georgia", android: "serif" }),
    fontSize: 42,
    lineHeight: 48,
    color: "#183e35",
  },
  description: { fontSize: 18, lineHeight: 28, color: "#59655d" },
  note: { fontSize: 14, lineHeight: 22, color: "#59655d" },
});
