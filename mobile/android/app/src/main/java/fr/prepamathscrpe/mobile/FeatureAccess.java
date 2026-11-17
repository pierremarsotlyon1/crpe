package fr.prepamathscrpe.mobile;

import java.net.URI;

final class FeatureAccess {
  static boolean https(String raw) {
    try {
      URI url = URI.create(raw);
      return "https".equals(url.getScheme()) && url.getHost() != null && url.getUserInfo() == null;
    } catch (RuntimeException invalid) {
      return false;
    }
  }

  static boolean current(String expiry, java.time.Instant now) {
    try {
      return java.time.Instant.parse(expiry).isAfter(now);
    } catch (java.time.DateTimeException e) {
      return false;
    }
  }

  static String tier(String tier) {
    return "suivi".equals(tier) ? "Suivi" : "Autonomie";
  }
}
