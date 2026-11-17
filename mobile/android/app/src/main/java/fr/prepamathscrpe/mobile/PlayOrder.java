package fr.prepamathscrpe.mobile;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.time.Instant;
import java.util.UUID;
import org.json.JSONObject;

final class PlayOrder {
  static JSONObject reserve(MemberAPI api, JSONObject input) throws Exception {
    try {
      return api.json("POST", ApiPath.reserveStoreOrder, input);
    } catch (MemberAPI.Failure failure) {
      if (failure.status < 500 || failure.status >= 600) throw failure;
    } catch (java.net.SocketTimeoutException | java.net.SocketException transientFailure) {
    }
    return api.json("POST", ApiPath.reserveStoreOrder, input);
  }

  static String digest(String reference) throws Exception {
    byte[] bytes =
        MessageDigest.getInstance("SHA-256").digest(reference.getBytes(StandardCharsets.UTF_8));
    StringBuilder result = new StringBuilder();
    for (byte value : bytes) result.append(String.format(java.util.Locale.ROOT, "%02x", value));
    return result.toString();
  }

  static String path(String order) {
    return ApiPath.cancelStoreOrder.replace("{platform}", "google")
        + "?orderId="
        + UUID.fromString(order);
  }

  static void bind(PracticeDB db, String owner, JSONObject order, String token) throws Exception {
    String digest = digest(token);
    db.put(
        owner,
        "receipt-order",
        digest,
        Repository.object(
            "digest",
            digest,
            "id",
            order.getString("id"),
            "expiresAt",
            order.getString("expiresAt")),
        null);
  }

  static void confirmed(PracticeDB db, String owner, String token, MemberAPI api) throws Exception {
    String digest = digest(token);
    for (JSONObject record : Repository.list(db.rows(owner, "receipt-order"))) {
      if (digest.equals(record.optString("digest"))) {
        record.put("confirmed", true);
        db.put(owner, "receipt-order", digest, record, null);
        release(db, owner, record, api);
      }
    }
  }

  static void clearConfirmed(PracticeDB db, String owner, MemberAPI api) throws Exception {
    for (JSONObject record : Repository.list(db.rows(owner, "receipt-order")))
      if (record.optBoolean("confirmed")
          || !Instant.parse(record.getString("expiresAt")).isAfter(Instant.now()))
        release(db, owner, record, api);
  }

  static void cancel(PracticeDB db, String owner, JSONObject order, MemberAPI api)
      throws Exception {
    String key = "cancel-" + order.getString("id");
    JSONObject record =
        Repository.object(
                "digest",
                key,
                "id",
                order.getString("id"),
                "expiresAt",
                order.getString("expiresAt"))
            .put("confirmed", true);
    db.put(owner, "receipt-order", key, record, null);
    release(db, owner, record, api);
  }

  private static void release(PracticeDB db, String owner, JSONObject record, MemberAPI api)
      throws Exception {
    try {
      if (Instant.parse(record.getString("expiresAt")).isAfter(Instant.now()))
        api.json("DELETE", path(record.getString("id")), null);
    } catch (MemberAPI.Failure problem) {
      if (problem.status != 404 && problem.status != 409) throw problem;
    }
    db.remove(owner, "receipt-order", record.getString("digest"));
  }
}
