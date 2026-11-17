package fr.prepamathscrpe.mobile;

import android.content.Context;
import android.content.SharedPreferences;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;
import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;
import org.json.JSONObject;

final class MemberSession {
  private static final String KEY = "crpe.member.aes";
  private final SharedPreferences prefs;

  MemberSession(Context context) {
    prefs = context.getSharedPreferences("member-session", Context.MODE_PRIVATE);
  }

  private SecretKey key() throws Exception {
    KeyStore keys = KeyStore.getInstance("AndroidKeyStore");
    keys.load(null);
    if (keys.containsAlias(KEY)) return (SecretKey) keys.getKey(KEY, null);
    KeyGenerator generator =
        KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore");
    generator.init(
        new KeyGenParameterSpec.Builder(
                KEY, KeyProperties.PURPOSE_ENCRYPT | KeyProperties.PURPOSE_DECRYPT)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .build());
    return generator.generateKey();
  }

  JSONObject read() throws Exception {
    String encoded = prefs.getString("data", null);
    if (encoded == null) return null;
    Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
    cipher.init(
        Cipher.DECRYPT_MODE,
        key(),
        new GCMParameterSpec(128, Base64.decode(prefs.getString("iv", ""), Base64.NO_WRAP)));
    return new JSONObject(
        new String(cipher.doFinal(Base64.decode(encoded, Base64.NO_WRAP)), StandardCharsets.UTF_8));
  }

  void save(JSONObject value) throws Exception {
    Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
    cipher.init(Cipher.ENCRYPT_MODE, key());
    String data =
        Base64.encodeToString(
            cipher.doFinal(value.toString().getBytes(StandardCharsets.UTF_8)), Base64.NO_WRAP);
    if (!prefs
        .edit()
        .putString("data", data)
        .putString("iv", Base64.encodeToString(cipher.getIV(), Base64.NO_WRAP))
        .commit()) throw new IllegalStateException("Session storage");
  }

  void clear() {
    if (!prefs.edit().clear().commit()) throw new IllegalStateException("Session storage");
  }
}
