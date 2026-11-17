package fr.prepamathscrpe.mobile;

import android.content.Context;
import java.time.Instant;
import java.util.*;
import org.json.*;

final class Repository {
  final PracticeDB db;
  final MemberSession keychain;
  JSONObject session, snapshot, access;
  private final Context context;
  JSONArray answers = new JSONArray(), copies = new JSONArray();

  Repository(Context context) throws Exception {
    this.context = context.getApplicationContext();
    db = new PracticeDB(context);
    keychain = new MemberSession(context);
    session = keychain.read();
    load();
  }

  String owner() throws JSONException {
    return session.getJSONObject("user").getString("id");
  }

  MemberAPI api() throws JSONException {
    return new MemberAPI(session == null ? null : session.getString("token"));
  }

  void load() throws Exception {
    if (session == null) {
      snapshot = null;
      access = null;
      answers = new JSONArray();
      copies = new JSONArray();
      return;
    }
    String owner = owner();
    access = db.rows(owner, "access").optJSONObject(0);
    snapshot = db.rows(owner, "snapshot").optJSONObject(0);
    answers = db.rows(owner, "answer");
    copies = db.rows(owner, "copy");
  }

  static JSONObject object(String... pairs) throws JSONException {
    JSONObject result = new JSONObject();
    for (int i = 0; i < pairs.length; i += 2) result.put(pairs[i], pairs[i + 1]);
    return result;
  }

  void request(String email) throws Exception {
    api().json("POST", ApiPath.requestLogin, object("email", email.trim(), "client", "mobile"));
  }

  void login(String token) throws Exception {
    if (session != null) return;
    JSONObject member =
        api().json("POST", ApiPath.verifyLogin, object("token", token, "client", "mobile"));
    if (member.getString("token").trim().isEmpty()) throw new JSONException("Missing session");
    UUID.fromString(member.getJSONObject("user").getString("id"));
    keychain.save(member);
    session = member;
    load();
  }

  void queueAnswer(String id, String value) throws Exception {
    if (!allows("practice")) throw new MemberAPI.Failure(403);
    String key = UUID.randomUUID().toString();
    JSONObject input =
        object(
            "clientId",
            key,
            "exerciseId",
            id,
            "answer",
            value.trim(),
            "performedAt",
            Instant.now().toString());
    db.put(owner(), "answer", key, new JSONObject().put("input", input), null);
    load();
  }

  void queueCopy(String id, String text, byte[] image) throws Exception {
    if (!allows("tutor")) throw new MemberAPI.Failure(403);
    String key = UUID.randomUUID().toString();
    db.put(
        owner(),
        "copy",
        key,
        object("clientId", key, "exerciseId", id, "text", text.trim())
            .put("hasImage", image != null),
        image);
    load();
  }

  JSONObject feature(String key) {
    for (JSONObject row : list(access == null ? null : access.optJSONArray("features")))
      if (key.equals(row.optString("key"))) return row;
    return null;
  }

  boolean allows(String key) {
    JSONObject row = feature(key);
    if (row == null || !row.optBoolean("allowed")) return false;
    if (session != null
        && "admin".equals(session.optJSONObject("user").optString("role"))
        && "suivi".equals(access.optString("tier"))
        && access.isNull("validUntil")) return true;
    String required = row.optString("requiredTier");
    if (!access.optBoolean("salesEnabled") && required.equals("autonomie")) return true;
    Instant now = Instant.now();
    if (FeatureAccess.current(access.optString("validUntil"), now)) return true;
    for (JSONObject provider : list(access.optJSONArray("providers"))) {
      String tier = provider.optString("tier");
      if ((tier.equals("suivi") || tier.equals(required))
          && FeatureAccess.current(provider.optString("expiresAt"), now)) return true;
    }
    return false;
  }

  void sync() throws Exception {
    if (session == null) return;
    MemberAPI net = api();
    String owner = owner();
    try {
      access = net.json("GET", ApiPath.getMemberAccess, null);
      db.put(owner, "access", "current", access, null);
      if (!allows("course")) {
        db.remove(owner, "snapshot", null);
        java.io.File[] files =
            LocalFiles.folder(context, owner).listFiles((dir, name) -> name.endsWith(".pdf"));
        if (files != null) for (java.io.File file : files) file.delete();
      }
      if (!allows("mock")) db.remove(owner, "mock", null);
      for (JSONObject row : list(answers)) {
        if (!allows("practice")) break;
        if (row.has("receipt") || row.has("rejection")) continue;
        JSONObject input = row.getJSONObject("input");
        try {
          JSONObject receipt = net.json("POST", ApiPath.gradeAttempt, input);
          if (!input.getString("clientId").equalsIgnoreCase(receipt.getString("clientId"))
              || !input.getString("exerciseId").equals(receipt.getString("exerciseId")))
            throw new JSONException("Mismatched receipt");
          row.put("receipt", receipt);
        } catch (MemberAPI.Failure e) {
          if (!Practice.permanent(e.status)) throw e;
          row.put("rejection", e.status);
        }
        db.put(owner, "answer", input.getString("clientId"), row, null);
      }
      for (JSONObject copy : list(copies)) {
        if (!allows("tutor")) break;
        if (copy.has("rejection")) continue;
        String id = copy.getString("clientId");
        try {
          net.submit(copy, db.picture(owner, id));
          db.remove(owner, "copy", id);
        } catch (MemberAPI.Failure e) {
          if (!Practice.permanent(e.status)) throw e;
          copy.put("rejection", e.status);
          db.put(owner, "copy", id, copy, null);
        }
      }
      JSONObject fresh =
          new JSONObject()
              .put(
                  "catalog",
                  allows("course")
                      ? net.json("GET", ApiPath.listCatalog, null).getJSONArray("items")
                      : new JSONArray())
              .put(
                  "daily",
                  allows("practice")
                      ? net.json("GET", ApiPath.listPractice + "?mode=daily", null)
                          .getJSONArray("items")
                      : new JSONArray())
              .put(
                  "progress",
                  allows("practice") ? net.json("GET", ApiPath.getProgress, null) : null)
              .put(
                  "submissions",
                  allows("tutor")
                      ? net.json("GET", ApiPath.getSubmissions, null).getJSONArray("items")
                      : new JSONArray())
              .put("updated", Instant.now().toString());
      db.put(owner, "snapshot", "current", fresh, null);
    } catch (MemberAPI.Failure e) {
      if (e.status == 401) {
        keychain.clear();
        session = null;
      }
      if (e.status == 403) db.remove(owner, "snapshot", null);
      throw e;
    } finally {
      load();
    }
  }

  void logout(boolean delete) throws Exception {
    String owner = owner();
    MemberAPI net = api();
    if (delete) net.json("DELETE", ApiPath.deleteMember, null);
    else
      try {
        net.json("POST", ApiPath.logout, null);
      } catch (java.io.IOException ignored) {
      }
    keychain.clear();
    session = null;
    db.remove(owner, null, null);
    load();
  }

  static List<JSONObject> list(JSONArray array) {
    List<JSONObject> result = new ArrayList<>();
    if (array != null)
      for (int i = 0; i < array.length(); i++) {
        JSONObject o = array.optJSONObject(i);
        if (o != null) result.add(o);
      }
    return result;
  }

  boolean hasPending() {
    for (JSONObject a : list(answers))
      if (allows("practice") && !a.has("receipt") && !a.has("rejection")) return true;
    for (JSONObject copy : list(copies)) if (allows("tutor") && !copy.has("rejection")) return true;
    return false;
  }

  List<JSONObject> catalog() {
    List<JSONObject> items = list(snapshot == null ? null : snapshot.optJSONArray("catalog"));
    items.removeIf(i -> !"validated".equals(i.optString("status")));
    items.sort(Comparator.comparingInt(i -> i.optInt("position")));
    return items;
  }

  List<JSONObject> exercises() {
    List<JSONObject> items = catalog();
    Set<String> exams = new HashSet<>();
    for (JSONObject i : items) if ("mock".equals(i.optString("kind"))) exams.add(i.optString("id"));
    items.removeIf(
        i ->
            !"crpe".equals(i.optString("examId"))
                || !"exercise".equals(i.optString("kind"))
                || exams.contains(i.optString("parentId")));
    return items;
  }

  List<JSONObject> due() {
    if (snapshot != null
        && snapshot.has("daily")
        && Instant.parse(snapshot.optString("updated"))
            .atZone(java.time.ZoneId.systemDefault())
            .toLocalDate()
            .equals(java.time.LocalDate.now())) {
      Set<String> pending = new HashSet<>();
      for (JSONObject row : list(answers))
        if (!row.has("receipt") && !row.has("rejection"))
          pending.add(row.optJSONObject("input").optString("exerciseId"));
      List<JSONObject> daily = list(snapshot.optJSONArray("daily"));
      daily.removeIf(
          i -> pending.contains(i.optString("id")) || !"validated".equals(i.optString("status")));
      return daily;
    }
    Map<String, String> reviews = new HashMap<>();
    JSONObject progress = snapshot == null ? null : snapshot.optJSONObject("progress");
    if (progress != null)
      for (JSONObject s : list(progress.optJSONArray("skills")))
        reviews.put(s.optString("skill"), s.optString("nextReviewAt"));
    Map<String, JSONObject> latest = new HashMap<>();
    for (JSONObject a : list(answers))
      if (!a.has("rejection")) latest.put(a.optJSONObject("input").optString("exerciseId"), a);
    Map<String, Practice.Priority> ranks = new HashMap<>();
    List<JSONObject> items = new ArrayList<>();
    Instant now = Instant.now();
    for (JSONObject item : exercises()) {
      if (!"exercise".equals(item.optString("kind"))
          || !("choice".equals(item.optString("format"))
              || "numeric".equals(item.optString("format")))) continue;
      String id = item.optString("id");
      JSONObject row = latest.get(id), receipt = row == null ? null : row.optJSONObject("receipt");
      Practice.Priority rank =
          Practice.priority(
              row != null && receipt == null,
              receipt == null ? null : receipt.optBoolean("correct"),
              receipt == null ? null : receipt.optString("nextReviewAt"),
              reviews.get(item.optString("skill")),
              now);
      if (rank.eligible()) {
        ranks.put(id, rank);
        items.add(item);
      }
    }
    items.sort(Comparator.comparing(i -> ranks.get(i.optString("id"))));
    return items.subList(0, Math.min(5, items.size()));
  }
}
