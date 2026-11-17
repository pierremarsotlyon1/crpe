package fr.prepamathscrpe.mobile;

import java.io.*;
import java.net.*;
import java.nio.charset.StandardCharsets;
import java.util.UUID;
import org.json.JSONObject;

final class MemberAPI {
  static final class Failure extends IOException {
    final int status;

    Failure(int status) {
      super("HTTP " + status);
      this.status = status;
    }
  }

  static String message(Exception error) {
    if (error instanceof Failure e)
      return switch (e.status) {
        case 400, 422 -> "Vérifiez les informations saisies.";
        case 401 -> "Votre lien ou votre session a expiré.";
        case 403 -> "Votre compte n’a pas accès à ce contenu.";
        case 404 -> "Cet exercice n’est plus disponible.";
        case 409 -> "Cet envoi entre en conflit avec un envoi précédent.";
        case 413, 415 -> "Choisissez une photo lisible de moins de 8 Mo.";
        case 429 -> "La limite a été atteinte. Réessayez plus tard.";
        default -> "Le service est indisponible. Réessayez plus tard.";
      };
    return "Connexion indisponible. Les envois enregistrés restent sur cet appareil.";
  }

  private final String token;
  private volatile HttpURLConnection active;
  private volatile boolean cancelled;

  void cancel() {
    cancelled = true;
    HttpURLConnection connection = active;
    if (connection != null) connection.disconnect();
  }

  MemberAPI(String token) {
    this.token = token;
  }

  private interface Sender {
    void write(OutputStream out) throws Exception;
  }

  private byte[] send(String method, String path, String type, Sender sender, int max)
      throws Exception {
    URL url = new URL(BuildConfig.API_URL.replaceAll("/$", "") + path);
    if (!url.getProtocol().equals("https")
        && !(BuildConfig.DEBUG && url.getProtocol().equals("http")))
      throw new IOException("Invalid API origin");
    HttpURLConnection c = (HttpURLConnection) url.openConnection();
    active = c;
    try {
      if (cancelled) throw new InterruptedIOException();
      c.setInstanceFollowRedirects(false);
      c.setConnectTimeout(12000);
      c.setReadTimeout(path.endsWith("/transcribe") || path.endsWith("/speech") ? 75000 : 22000);
      c.setRequestMethod(method);
      c.setUseCaches(false);
      if (token != null) c.setRequestProperty("Authorization", "Bearer " + token);
      if (sender != null) {
        c.setDoOutput(true);
        c.setChunkedStreamingMode(32768);
        c.setRequestProperty("Content-Type", type);
        try (OutputStream out = c.getOutputStream()) {
          sender.write(out);
        }
      }
      int status = c.getResponseCode();
      if (status < 200 || status >= 300) throw new Failure(status);
      try (InputStream in = c.getInputStream();
          ByteArrayOutputStream out = new ByteArrayOutputStream()) {
        byte[] chunk = new byte[16384];
        int n;
        while ((n = in.read(chunk)) != -1) {
          if (out.size() + n > max) throw new IOException("Response exceeds limit");
          out.write(chunk, 0, n);
        }
        return out.toByteArray();
      }
    } finally {
      active = null;
      c.disconnect();
    }
  }

  byte[] file(String path) throws Exception {
    return send("GET", path, null, null, 32 * 1024 * 1024);
  }

  byte[] speak(String sessionId, int index) throws Exception {
    String path = ApiPath.speakQuestion.replace("{id}", UUID.fromString(sessionId).toString());
    byte[] input = new JSONObject().put("index", index).toString().getBytes(StandardCharsets.UTF_8);
    return send("POST", path, "application/json", out -> out.write(input), 8 * 1024 * 1024);
  }

  String transcribe(String sessionId, File audio) throws Exception {
    OralRecording.validate(audio);
    String path = ApiPath.transcribeOral.replace("{id}", UUID.fromString(sessionId).toString());
    String boundary = "crpe-" + UUID.randomUUID();
    byte[] response =
        send(
            "POST",
            path,
            "multipart/form-data; boundary=" + boundary,
            out -> {
              line(out, "--" + boundary);
              line(out, "Content-Disposition: form-data; name=\"audio\"; filename=\"oral.m4a\"");
              line(out, "Content-Type: audio/mp4");
              line(out, "");
              try (InputStream in = new FileInputStream(audio)) {
                byte[] chunk = new byte[32 * 1024];
                int count;
                while ((count = in.read(chunk)) != -1) out.write(chunk, 0, count);
              }
              line(out, "");
              line(out, "--" + boundary + "--");
            },
            256 * 1024);
    String result =
        new JSONObject(new String(response, StandardCharsets.UTF_8)).getString("text").trim();
    if (result.isEmpty()) throw new IOException("Missing transcription");
    return result;
  }

  JSONObject json(String method, String path, JSONObject payload) throws Exception {
    byte[] bytes =
        send(
            method,
            path,
            "application/json",
            payload == null
                ? null
                : out -> out.write(payload.toString().getBytes(StandardCharsets.UTF_8)),
            32 * 1024 * 1024);
    return bytes.length == 0
        ? new JSONObject()
        : new JSONObject(new String(bytes, StandardCharsets.UTF_8));
  }

  private static void line(OutputStream out, String value) throws IOException {
    out.write((value + "\r\n").getBytes(StandardCharsets.UTF_8));
  }

  JSONObject submit(JSONObject copy, byte[] picture) throws Exception {
    String boundary = "crpe-" + UUID.randomUUID();
    byte[] bytes =
        send(
            "POST",
            ApiPath.submitWork,
            "multipart/form-data; boundary=" + boundary,
            out -> {
              for (String field : new String[] {"clientId", "exerciseId", "text"}) {
                String value = copy.optString(field);
                if (value.isEmpty()) continue;
                line(out, "--" + boundary);
                line(out, "Content-Disposition: form-data; name=\"" + field + "\"");
                line(out, "");
                line(out, value);
              }
              if (picture != null) {
                line(out, "--" + boundary);
                line(out, "Content-Disposition: form-data; name=\"image\"; filename=\"copie.jpg\"");
                line(out, "Content-Type: image/jpeg");
                line(out, "");
                out.write(picture);
                line(out, "");
              }
              line(out, "--" + boundary + "--");
            },
            1024 * 1024);
    return new JSONObject(new String(bytes, StandardCharsets.UTF_8));
  }
}
