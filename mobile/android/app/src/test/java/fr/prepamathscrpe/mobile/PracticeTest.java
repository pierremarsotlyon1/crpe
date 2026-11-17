package fr.prepamathscrpe.mobile;

import static org.junit.Assert.*;

import java.time.Instant;
import org.junit.Test;

public class PracticeTest {
  private final Instant now = Instant.parse("2026-11-17T12:00:00Z");

  @Test
  public void cancellationIncludesOnlyTheReservedOrderNonce() throws Exception {
    String id = java.util.UUID.randomUUID().toString();
    assertEquals("/v1/me/store-orders/google?orderId=" + id, PlayOrder.path(id));
    org.junit.Assert.assertThrows(
        IllegalArgumentException.class, () -> PlayOrder.path("autre-commande"));
    assertEquals(64, PlayOrder.digest("test-token").length());
    assertNotEquals(PlayOrder.digest("test-token"), PlayOrder.digest("second-test-token"));
  }

  @Test
  public void playReceiptRequiresMemberAndServerBeforeAccessAndReplaysItsToken() throws Exception {
    int purchased = com.android.billingclient.api.Purchase.PurchaseState.PURCHASED,
        pending = com.android.billingclient.api.Purchase.PurchaseState.PENDING;
    var references = new java.util.ArrayList<String>();
    PlayReceiptDelivery.Verify record =
        (sku, ref) -> {
          assertEquals("fixture", sku);
          references.add(ref);
          return "active";
        };
    assertEquals(
        "other_account",
        PlayReceiptDelivery.deliver("owner", "other", purchased, "fixture", "test-token", record));
    assertEquals(
        "pending",
        PlayReceiptDelivery.deliver("owner", "owner", pending, "fixture", "test-token", record));
    assertTrue(references.isEmpty());
    org.junit.Assert.assertThrows(
        java.io.IOException.class,
        () ->
            PlayReceiptDelivery.deliver(
                "owner",
                "owner",
                purchased,
                "fixture",
                "test-token",
                (sku, ref) -> {
                  references.add(ref);
                  throw new java.io.IOException();
                }));
    assertEquals(
        "active",
        PlayReceiptDelivery.deliver("owner", "owner", purchased, "fixture", "test-token", record));
    assertEquals(java.util.List.of("test-token", "test-token"), references);
    assertEquals(
        com.android.billingclient.api.BillingFlowParams.ProductDetailsParams
            .SubscriptionProductReplacementParams.ReplacementMode.WITHOUT_PRORATION,
        PlaySubscriptions.replacement("suivi", "suivi", "suivi", "suivi"));
    assertEquals(
        com.android.billingclient.api.BillingFlowParams.ProductDetailsParams
            .SubscriptionProductReplacementParams.ReplacementMode.DEFERRED,
        PlaySubscriptions.replacement("suivi", "autonomie", "suivi", "autonomie"));
    assertEquals(
        com.android.billingclient.api.BillingFlowParams.ProductDetailsParams
            .SubscriptionProductReplacementParams.ReplacementMode.WITH_TIME_PRORATION,
        PlaySubscriptions.replacement("autonomie", "suivi", "autonomie", "suivi"));
  }

  @Test
  public void offlineAccessRequiresAnUnexpiredServerDate() {
    assertFalse(FeatureAccess.current("", now));
    assertFalse(FeatureAccess.current("2026-11-17T12:00:00Z", now));
    assertTrue(FeatureAccess.current("2026-11-18T00:00:00Z", now));
  }

  @Test
  public void playPlansAvoidDuplicateProvidersAndUseStoreBillingPeriods() {
    assertTrue(PlaySubscriptions.accepts(""));
    assertTrue(PlaySubscriptions.accepts("google"));
    assertFalse(PlaySubscriptions.accepts("apple"));
    assertFalse(PlaySubscriptions.accepts("stripe"));
    assertEquals("mois", PlaySubscriptions.duration("P1M"));
    assertEquals("an", PlaySubscriptions.duration("P1Y"));
    assertEquals("semaine", PlaySubscriptions.duration("P1W"));
    assertFalse(FeatureAccess.https("https://private@exemple.test"));
  }

  @Test
  public void retriesArePrioritizedWhenDue() {
    var failed = Practice.priority(false, false, "2026-11-16T08:00:00Z", null, now);
    var fresh = Practice.priority(false, null, null, null, now);
    assertTrue(failed.eligible());
    assertTrue(fresh.eligible());
    assertTrue(failed.compareTo(fresh) < 0);
  }

  @Test
  public void futureSkillsAndUnconfirmedWorkWait() {
    assertFalse(Practice.priority(false, null, null, "2026-11-20T12:00:00Z", now).eligible());
    assertFalse(Practice.priority(true, null, null, null, now).eligible());
    assertFalse(Practice.priority(false, true, "2026-11-18T00:00:00Z", null, now).eligible());
    assertTrue(Practice.priority(false, true, "2026-11-17T12:00:00Z", null, now).eligible());
  }

  @Test
  public void transportFailuresStayInOutbox() {
    for (int transientStatus : new int[] {401, 403, 408, 429, 500, 503})
      assertFalse(Practice.permanent(transientStatus));
    assertTrue(Practice.permanent(409));
    assertTrue(Practice.permanent(404));
  }

  @Test
  public void copyPollingStopsInBackgroundAndAtTeacherHandoff() {
    java.util.Set<String> labels = new java.util.HashSet<>();
    for (String stage :
        new String[] {
          "queued", "processing", "retrying", "teacher_review", "completed", "failed"
        }) {
      labels.add(Practice.correctionLabel(stage));
      assertEquals(
          java.util.Set.of("queued", "processing", "retrying").contains(stage),
          Practice.shouldPoll(true, true, true, java.util.List.of(stage)));
    }
    assertEquals(6, labels.size());
    assertFalse(Practice.shouldPoll(true, false, true, java.util.List.of("processing")));
    assertFalse(Practice.shouldPoll(false, true, true, java.util.List.of("processing")));
    assertFalse(Practice.shouldPoll(true, true, false, java.util.List.of("processing")));
  }
}
