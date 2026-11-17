package fr.prepamathscrpe.mobile;

import com.android.billingclient.api.Purchase;
import java.util.Set;

final class PlayReceiptDelivery {
  interface Verify {
    String purchase(String product, String token) throws Exception;
  }

  static String deliver(
      String account, String attached, int state, String product, String token, Verify server)
      throws Exception {
    if (account == null || account.isBlank() || !account.equals(attached)) return "other_account";
    if (state == Purchase.PurchaseState.PENDING) return "pending";
    if (state != Purchase.PurchaseState.PURCHASED || product.isBlank() || token.isBlank())
      return "rejected";
    String result = server.purchase(product, token);
    if (!Set.of("active", "pending", "revoked", "expired").contains(result))
      throw new java.io.IOException("Unknown verification state");
    return result;
  }
}
