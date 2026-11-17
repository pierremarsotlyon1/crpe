package fr.prepamathscrpe.mobile;

import android.net.Uri;
import com.android.billingclient.api.*;
import com.android.billingclient.api.BillingFlowParams.ProductDetailsParams;
import com.android.billingclient.api.BillingFlowParams.ProductDetailsParams.SubscriptionProductReplacementParams;
import java.time.Period;
import java.util.*;
import org.json.*;

final class PlaySubscriptions implements PurchasesUpdatedListener {
  private final MainActivity view;
  private BillingClient client;
  private JSONObject checkoutOrder;
  private String checkoutProduct = "";
  private long checkoutStarted;
  private final Set<String> ownedBeforeCheckout = new HashSet<>();
  private static final String OTHER_MEMBER =
      "Cet achat est associé à un autre compte CRPE. Reconnectez-vous au compte utilisé lors de la"
          + " souscription.";
  private String owner = "", notice = "";
  private JSONObject catalog;
  private List<ProductDetails> products = List.of();
  private List<Purchase> owned = List.of();
  private final Set<String> verifying = new HashSet<>();
  private boolean connecting,
      shown,
      checkout,
      restoreRequested,
      loadedOffers,
      loading,
      awaitingRetry;
  private long generation;

  PlaySubscriptions(MainActivity view) {
    this.view = view;
  }

  static boolean accepts(String provider) {
    return provider.isEmpty() || provider.equals("google");
  }

  static String duration(String iso) {
    try {
      Period p = Period.parse(iso);
      if (p.getYears() > 0 && p.getMonths() == 0 && p.getDays() == 0)
        return p.getYears() == 1 ? "an" : p.getYears() + " ans";
      if (p.getMonths() > 0 && p.getDays() == 0 && p.getYears() == 0)
        return p.getMonths() == 1 ? "mois" : p.getMonths() + " mois";
      if (p.getDays() > 0 && p.getMonths() == 0 && p.getYears() == 0)
        return p.getDays() == 7 ? "semaine" : p.getDays() == 1 ? "jour" : p.getDays() + " jours";
    } catch (RuntimeException ignored) {
    }
    return iso;
  }

  private boolean current(String id) {
    try {
      return !view.isDestroyed()
          && view.repo != null
          && view.repo.session != null
          && id.equals(view.repo.owner());
    } catch (Exception e) {
      return false;
    }
  }

  private void display() {
    if (shown && !view.isDestroyed()) view.render();
  }

  private void state(String text) {
    notice = text;
    display();
  }

  void leave() {
    shown = false;
  }

  void stop() {
    generation++;
    checkoutOrder = null;
    checkoutProduct = "";
    ownedBeforeCheckout.clear();
    leave();
    owner = "";
    catalog = null;
    owned = List.of();
    products = List.of();
    verifying.clear();
    loading = false;
    connecting = false;
    checkout = false;
    awaitingRetry = false;
    loadedOffers = false;
    restoreRequested = false;
    if (client != null) {
      client.endConnection();
      client = null;
    }
  }

  void resume() {
    if (view.repo == null || view.repo.session == null) {
      stop();
      return;
    }
    try {
      String id = view.repo.owner();
      if (!id.equals(owner)) {
        boolean wasShown = shown;
        stop();
        owner = id;
        shown = wasShown;
      }
      load();
    } catch (Exception ignored) {
    }
  }

  void retry() {
    if (awaitingRetry && current(owner)) resume();
  }

  private void load() {
    if (loading || !current(owner)) return;
    final String id = owner;
    final long version = ++generation;
    final MemberAPI api;
    try {
      api = view.repo.api();
    } catch (Exception e) {
      return;
    }
    loading = true;
    MainActivity.worker.execute(
        () -> {
          JSONObject listing = null;
          try {
            listing = api.json("GET", ApiPath.getStoreProducts + "?platform=google", null);
            PlayOrder.clearConfirmed(view.repo.db, id, api);
          } catch (Exception ignored) {
          }
          final JSONObject result = listing;
          view.runOnUiThread(
              () -> {
                if (version != generation || !current(id)) return;
                loading = false;
                catalog = result;
                products = List.of();
                loadedOffers = false;
                if (result == null) {
                  state(
                      "Les informations de Google Play sont indisponibles. Réessayez dans un"
                          + " instant.");
                  return;
                }
                connect();
                display();
              });
        });
  }

  private void connect() {
    if (!current(owner)) return;
    if (client == null)
      client =
          BillingClient.newBuilder(view)
              .setListener(this)
              .enablePendingPurchases(
                  PendingPurchasesParams.newBuilder()
                      .enableOneTimeProducts()
                      .enablePrepaidPlans()
                      .build())
              .enableAutoServiceReconnection()
              .build();
    if (client.isReady()) {
      query();
      return;
    }
    if (connecting) return;
    connecting = true;
    BillingClient active = client;
    active.startConnection(
        new BillingClientStateListener() {
          public void onBillingServiceDisconnected() {
            view.runOnUiThread(
                () -> {
                  if (active == client) {
                    connecting = false;
                    state("Connexion à Google Play interrompue.");
                  }
                });
          }

          public void onBillingSetupFinished(BillingResult result) {
            view.runOnUiThread(
                () -> {
                  if (active != client) return;
                  connecting = false;
                  if (result.getResponseCode() == BillingClient.BillingResponseCode.OK) {
                    query();
                  } else state("Google Play n’est pas disponible sur cet appareil.");
                });
          }
        });
  }

  private boolean selling() {
    return catalog != null
        && catalog.optBoolean("enabled")
        && view.repo.access != null
        && view.repo.access.optBoolean("salesEnabled")
        && accepts(catalog.optString("activeProvider"));
  }

  private void query() {
    if (client == null || !client.isReady() || !current(owner)) return;
    final String id = owner;
    final BillingClient active = client;
    client.queryPurchasesAsync(
        QueryPurchasesParams.newBuilder()
            .setProductType(BillingClient.ProductType.SUBS)
            .includeSuspendedSubscriptions(true)
            .build(),
        (result, all) ->
            view.runOnUiThread(
                () -> {
                  if (active != client || !current(id)) return;
                  if (result.getResponseCode() != BillingClient.BillingResponseCode.OK) {
                    state(
                        "La restauration Google Play est indisponible. Vérifiez que le Play Store"
                            + " est installé et connecté.");
                    return;
                  }
                  owned = List.copyOf(all);
                  boolean match = false;
                  for (Purchase p : all) {
                    if (matches(p)) {
                      match = true;
                      confirm(p);
                    }
                  }
                  if (restoreRequested && !match)
                    state(
                        all.isEmpty()
                            ? "Aucun abonnement Google Play à restaurer pour ce compte."
                            : OTHER_MEMBER);
                  restoreRequested = false;
                  display();
                }));
    if (!shown || !selling()) return;
    Set<String> ids = new LinkedHashSet<>();
    for (JSONObject item : Repository.list(catalog.optJSONArray("items")))
      ids.add(item.optString("productId"));
    List<QueryProductDetailsParams.Product> request = new ArrayList<>();
    for (String product : ids)
      if (!product.isEmpty())
        request.add(
            QueryProductDetailsParams.Product.newBuilder()
                .setProductId(product)
                .setProductType(BillingClient.ProductType.SUBS)
                .build());
    if (request.isEmpty()) {
      loadedOffers = true;
      display();
      return;
    }
    client.queryProductDetailsAsync(
        QueryProductDetailsParams.newBuilder().setProductList(request).build(),
        (result, details) ->
            view.runOnUiThread(
                () -> {
                  if (active != client || !current(id)) return;
                  loadedOffers = true;
                  products =
                      result.getResponseCode() == BillingClient.BillingResponseCode.OK
                          ? details.getProductDetailsList()
                          : List.of();
                  display();
                }));
  }

  private boolean matches(Purchase purchase) {
    AccountIdentifiers ids = purchase.getAccountIdentifiers();
    return catalog != null
        && ids != null
        && catalog.optString("accountToken").equals(ids.getObfuscatedAccountId())
        && !catalog.optString("accountToken").isEmpty();
  }

  private void confirm(Purchase p) {
    confirm(p, false);
  }

  private void confirm(Purchase p, boolean checkoutResult) {
    if (catalog == null || p.getProducts().isEmpty()) return;
    boolean added = verifying.add(p.getPurchaseToken());
    if (!added && !checkoutResult) return;
    String account = catalog.optString("accountToken"),
        attached =
            p.getAccountIdentifiers() == null
                ? null
                : p.getAccountIdentifiers().getObfuscatedAccountId();
    String token = p.getPurchaseToken(), id = owner;
    MemberAPI api;
    try {
      api = view.repo.api();
    } catch (Exception ex) {
      verifying.remove(token);
      return;
    }
    MainActivity.worker.execute(
        () -> {
          String status = "";
          try {
            status =
                PlayReceiptDelivery.deliver(
                    account,
                    attached,
                    p.getPurchaseState(),
                    p.getProducts().get(0),
                    token,
                    (product, reference) -> {
                      JSONObject body =
                          Repository.object(
                              "platform", "google", "productId", product, "reference", reference);
                      return api.json("POST", ApiPath.verifyStorePurchase, body)
                          .getString("status");
                    });
            if (Set.of("active", "expired", "revoked").contains(status)) {
              try {
                PlayOrder.confirmed(view.repo.db, id, token, api);
              } catch (Exception retryLease) {
              }
            }
          } catch (MemberAPI.Failure failure) {
            if (Set.of(400, 403, 404, 409, 422).contains(failure.status)) status = "rejected";
          } catch (Exception transientFailure) {
          }
          final String checked = status;
          view.runOnUiThread(
              () -> {
                if (!current(id)) return;
                verifying.remove(token);
                awaitingRetry = checked.isEmpty() || checked.equals("pending");
                notice =
                    switch (checked) {
                      case "active" -> "Abonnement confirmé.";
                      case "other_account" -> OTHER_MEMBER;
                      case "rejected" ->
                          "Cette transaction ne peut pas être attribuée à ce compte CRPE. Vérifiez"
                              + " le compte utilisé lors de la souscription.";
                      case "expired" -> "Cet abonnement a expiré.";
                      case "revoked" -> "Cet achat a été révoqué.";
                      case "pending" -> "Achat en attente de confirmation.";
                      default ->
                          "Votre achat sera vérifié dès que le service sera disponible. Vous pouvez"
                              + " aussi restaurer vos achats.";
                    };
                if (Set.of("active", "expired", "revoked").contains(checked)) view.refreshMember();
                display();
              });
        });
  }

  private void restore() {
    restoreRequested = true;
    notice = "";
    resume();
    display();
  }

  @Override
  public void onPurchasesUpdated(BillingResult result, List<Purchase> purchases) {
    view.runOnUiThread(
        () -> {
          if (!current(owner)) return;
          checkout = false;
          int code = result.getResponseCode();
          if (code == BillingClient.BillingResponseCode.OK && purchases != null) {
            if (catalog == null) {
              awaitingRetry = true;
              state("Votre achat sera vérifié dès que la connexion au service sera rétablie.");
              return;
            }
            boolean belongs = false;
            for (Purchase p : purchases) {
              if (!matches(p)) continue;
              belongs = true;
              boolean boundNow = false;
              if (checkoutOrder != null
                  && p.getProducts().contains(checkoutProduct)
                  && p.getPurchaseTime() >= checkoutStarted
                  && !ownedBeforeCheckout.contains(p.getPurchaseToken())) {
                try {
                  PlayOrder.bind(view.repo.db, owner, checkoutOrder, p.getPurchaseToken());
                  checkoutOrder = null;
                  boundNow = true;
                } catch (Exception failure) {
                  state("Impossible de conserver la réservation. Restaurez vos achats.");
                }
              }
              confirm(p, boundNow);
            }
            if (!belongs && !purchases.isEmpty()) state(OTHER_MEMBER);
            return;
          }
          if (code == BillingClient.BillingResponseCode.USER_CANCELED) {
            JSONObject cancelled = checkoutOrder;
            checkoutOrder = null;
            if (cancelled != null)
              try {
                String holder = owner;
                MemberAPI api = view.repo.api();
                MainActivity.worker.execute(
                    () -> {
                      try {
                        PlayOrder.cancel(view.repo.db, holder, cancelled, api);
                      } catch (Exception retryLease) {
                      }
                    });
              } catch (Exception ignored) {
              }
            state("Achat annulé.");
          } else if (code == BillingClient.BillingResponseCode.ITEM_ALREADY_OWNED) {
            restore();
          } else
            state(
                "Google Play n’a pas confirmé l’achat. Restaurez vos achats si le paiement a"
                    + " abouti.");
        });
  }

  private boolean isCurrent(String product, String base) {
    return "google".equals(catalog.optString("activeProvider"))
        && product.equals(catalog.optString("activeProductId"))
        && base.equals(catalog.optString("activeBasePlanId"));
  }

  static int replacement(String oldProduct, String newProduct, String oldTier, String newTier) {
    if (oldProduct.equals(newProduct))
      return SubscriptionProductReplacementParams.ReplacementMode.WITHOUT_PRORATION;
    if ("suivi".equals(oldTier) && "autonomie".equals(newTier))
      return SubscriptionProductReplacementParams.ReplacementMode.DEFERRED;
    return SubscriptionProductReplacementParams.ReplacementMode.WITH_TIME_PRORATION;
  }

  private void buy(
      JSONObject item, ProductDetails details, ProductDetails.SubscriptionOfferDetails offer) {
    if (checkout || loading || client == null || !client.isReady() || !selling()) return;
    String id = owner,
        prev = catalog.optString("activeProductId"),
        productId = item.optString("productId"),
        base = item.optString("basePlanId");
    if (isCurrent(productId, base)) {
      state("Cette formule est déjà active.");
      return;
    }
    ProductDetailsParams.Builder product =
        ProductDetailsParams.newBuilder()
            .setProductDetails(details)
            .setOfferToken(offer.getOfferToken());
    BillingFlowParams.Builder flow =
        BillingFlowParams.newBuilder().setObfuscatedAccountId(catalog.optString("accountToken"));
    if ("google".equals(catalog.optString("activeProvider"))) {
      Purchase old = null;
      for (Purchase p : owned)
        if (matches(p)
            && p.getPurchaseState() == Purchase.PurchaseState.PURCHASED
            && p.getProducts().contains(prev)) old = p;
      if (old == null) {
        state(
            "Restaurez votre abonnement Google Play avec le compte utilisé pour le premier achat.");
        return;
      }
      product.setSubscriptionProductReplacementParams(
          SubscriptionProductReplacementParams.newBuilder()
              .setOldProductId(prev)
              .setReplacementMode(
                  replacement(
                      prev, productId, view.repo.access.optString("tier"), item.optString("tier")))
              .build());
      flow.setSubscriptionUpdateParams(
          BillingFlowParams.SubscriptionUpdateParams.newBuilder()
              .setOldPurchaseToken(old.getPurchaseToken())
              .build());
    }
    flow.setProductDetailsParamsList(List.of(product.build()));
    try {
      MemberAPI api = view.repo.api();
      JSONObject request = Repository.object("platform", "google", "productId", productId);
      if (!base.isEmpty()) request.put("basePlanId", base);
      request.put("clientId", UUID.randomUUID().toString());
      if ("google".equals(catalog.optString("activeProvider"))) {
        request.put("expectedActiveProductId", prev);
        String previousBase = catalog.optString("activeBasePlanId");
        if (!previousBase.isEmpty()) request.put("expectedActiveBasePlanId", previousBase);
      }
      checkout = true;
      display();
      MainActivity.worker.execute(
          () -> {
            String error = null;
            JSONObject reservation = null;
            try {
              reservation = PlayOrder.reserve(api, request);
              UUID.fromString(reservation.getString("id"));
            } catch (Exception e) {
              error =
                  e instanceof MemberAPI.Failure f && f.status == 409
                      ? "Un abonnement ou une autre commande est déjà en cours."
                      : "L’achat ne peut pas être réservé pour le moment.";
            }
            final String failure = error;
            final JSONObject reserved = reservation;
            view.runOnUiThread(
                () -> {
                  if (!current(id)) return;
                  if (failure != null || client == null || !client.isReady() || !shown) {
                    checkout = false;
                    if (failure != null) {
                      load();
                      view.refreshMember();
                    }
                    state(failure != null ? failure : "Rouvrez la boutique pour réessayer.");
                    return;
                  }
                  checkoutOrder = reserved;
                  checkoutProduct = productId;
                  checkoutStarted = System.currentTimeMillis() - 1000;
                  ownedBeforeCheckout.clear();
                  for (Purchase old : owned) ownedBeforeCheckout.add(old.getPurchaseToken());
                  BillingResult launch = client.launchBillingFlow(view, flow.build());
                  if (launch.getResponseCode() != BillingClient.BillingResponseCode.OK)
                    onPurchasesUpdated(launch, null);
                });
          });
    } catch (Exception e) {
      checkout = false;
      state("Impossible de préparer cet achat.");
    }
  }

  void show(String tier) {
    boolean opening = !shown;
    shown = true;

    if (opening) {
      notice = "";
      resume();
    }
    if (tier.isEmpty()) view.title("Mon abonnement");
    if (catalog != null) {
      String provider = catalog.optString("activeProvider", "");
      if (!accepts(provider)) {
        view.label(
            "apple".equals(provider)
                ? "Votre abonnement est géré par l’App Store."
                : "Votre abonnement est géré depuis l’espace membre web.",
            17);
      } else if (selling()) {
        boolean found = false;
        for (JSONObject item : Repository.list(catalog.optJSONArray("items"))) {
          if (!tier.isEmpty() && !tier.equals(item.optString("tier"))) continue;
          for (ProductDetails detail : products)
            if (detail.getProductId().equals(item.optString("productId"))
                && detail.getSubscriptionOfferDetails() != null) {
              for (ProductDetails.SubscriptionOfferDetails offer :
                  detail.getSubscriptionOfferDetails()) {
                if (offer.getInstallmentPlanDetails() != null
                    || offer.getOfferId() != null
                    || !offer.getBasePlanId().equals(item.optString("basePlanId"))) continue;
                found = true;
                view.heading(detail.getName());
                view.label(detail.getDescription(), 16);
                for (ProductDetails.PricingPhase phase :
                    offer.getPricingPhases().getPricingPhaseList()) {
                  view.label(
                      phase.getFormattedPrice() + " / " + duration(phase.getBillingPeriod()), 18);
                }
                if (!isCurrent(item.optString("productId"), item.optString("basePlanId"))) {
                  android.widget.Button button =
                      view.button(
                          "Choisir la formule " + FeatureAccess.tier(item.optString("tier")),
                          true,
                          () -> buy(item, detail, offer));
                  if (checkout || loading) button.setEnabled(false);
                }
              }
            }
        }
        if (!found && loadedOffers)
          view.label("Aucune offre disponible dans Google Play pour cette formule.", 16);
        if (found) {
          view.label(
              "L’abonnement se renouvelle automatiquement. Vous pouvez le résilier dans Google"
                  + " Play.",
              15);
          for (String key : List.of("termsUrl", "privacyUrl"))
            if (FeatureAccess.https(catalog.optString(key)))
              view.button(
                  "termsUrl".equals(key) ? "Conditions d’utilisation" : "Confidentialité",
                  false,
                  () -> view.external(catalog.optString(key)));
        }
      } else if (view.repo.access != null && view.repo.access.optBoolean("salesEnabled"))
        view.label("Les achats Google Play ne sont pas disponibles pour le moment.", 16);
      if ("google".equals(provider)) {
        Uri.Builder manage =
            Uri.parse("https://play.google.com/store/account/subscriptions").buildUpon();
        String sku = catalog.optString("activeProductId");
        if (!sku.isEmpty())
          manage
              .appendQueryParameter("sku", sku)
              .appendQueryParameter("package", view.getPackageName());
        view.button(
            "Gérer mon abonnement Google Play",
            false,
            () -> view.external(manage.build().toString()));
      }
    }
    if (!notice.isEmpty()) view.label(notice, 16);
    view.button("Restaurer mes achats Google Play", false, this::restore);
  }
}
