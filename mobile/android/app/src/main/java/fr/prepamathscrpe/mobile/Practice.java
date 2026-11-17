package fr.prepamathscrpe.mobile;

import java.time.Instant;
import java.util.Comparator;

final class Practice {
  record Priority(boolean eligible, int rank, Instant next) implements Comparable<Priority> {
    public int compareTo(Priority other) {
      return Comparator.comparingInt(Priority::rank)
          .thenComparing(Priority::next)
          .compare(this, other);
    }
  }

  static Priority priority(
      boolean pending, Boolean correct, String review, String skillReview, Instant now) {
    String value = correct == null ? skillReview : review;
    Instant next = Instant.MIN;
    if (value != null && !value.isEmpty()) {
      try {
        next = Instant.parse(value);
      } catch (RuntimeException e) {
        next = Instant.MAX;
      }
    }
    int rank =
        Boolean.FALSE.equals(correct)
            ? 0
            : correct != null || (skillReview != null && !skillReview.isEmpty()) ? 1 : 2;
    return new Priority(!pending && !next.isAfter(now), rank, next);
  }

  static boolean permanent(int status) {
    return status == 400
        || status == 404
        || status == 409
        || status == 413
        || status == 415
        || status == 422;
  }

  static String correctionLabel(String stage) {
    return switch (stage) {
      case "queued" -> "En attente de correction";
      case "processing" -> "Correction en cours";
      case "retrying" -> "Nouvelle tentative prévue";
      case "teacher_review" -> "En relecture par l’enseignante";
      case "completed" -> "Correction terminée";
      case "failed" -> "La correction a échoué";
      default -> "État de correction indisponible";
    };
  }

  static boolean shouldPoll(
      boolean visible, boolean foreground, boolean signedIn, Iterable<String> stages) {
    if (!visible || !foreground || !signedIn) return false;
    for (String stage : stages)
      if ("queued".equals(stage) || "processing".equals(stage) || "retrying".equals(stage))
        return true;
    return false;
  }
}
