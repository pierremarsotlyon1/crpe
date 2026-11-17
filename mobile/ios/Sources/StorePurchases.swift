import StoreKit
import SwiftUI
import UIKit

enum StoreAccess {
  static func accepts(provider: String?, platform: String) -> Bool {
    provider == nil || provider == platform
  }
  static func message(_ status: String) -> String {
    switch status {
    case "active": "Abonnement confirmé."
    case "expired": "Cet abonnement a expiré."
    case "revoked": "Cet achat a été révoqué."
    default: "Achat en attente de confirmation."
    }
  }
}

enum ReceiptDeliveryError: Error { case otherMember }

@MainActor enum AppleReceiptDelivery {
  static let otherMember =
    "Cet achat est associé à un autre compte CRPE. Reconnectez-vous au compte utilisé lors de la souscription."
  static func deliver(
    _ transaction: StoreKit.Transaction, owner: String,
    verify: (StorePurchaseInput) async throws -> StoreReceipt,
    finish: () async throws -> Void
  ) async throws -> StoreReceipt {
    guard transaction.appAccountToken?.uuidString.lowercased() == owner.lowercased() else {
      throw ReceiptDeliveryError.otherMember
    }
    let request = StorePurchaseInput(
      platform: "apple", productId: transaction.productID, reference: String(transaction.id))
    let receipt = try await verify(request)
    if ["active", "expired", "revoked"].contains(receipt.status) { try await finish() }
    return receipt
  }
}

struct ReceiptOrder: Codable {
  let reference: String
  let order: StoreOrder
  var confirmed: Bool
}
@MainActor final class StorePurchases: ObservableObject {
  @Published private(set) var catalog: StoreProducts?
  @Published private(set) var products: [Product] = []
  @Published private(set) var busy = false
  @Published private(set) var notice = ""
  private var retry = false
  private var processing = Set<UInt64>()
  private var bound: String?

  func catalogFor(_ model: MemberModel) async throws -> StoreProducts {
    guard let owner = model.session?.user.id else { throw AppError.status(401) }
    let fresh = try await model.authorizedAPI().read(
      APIPath.getStoreProducts + "?platform=apple", StoreProducts.self)
    try Task.checkCancellation()
    guard model.session?.user.id == owner else { throw AppError.status(401) }
    catalog = fresh
    return fresh
  }
  func load(_ model: MemberModel, duringCheckout: Bool = false) async {
    guard !busy || duringCheckout else { return }
    do {
      let owner = model.session?.user.id
      let fresh = try await catalogFor(model)
      guard fresh.enabled, model.access?.salesEnabled == true,
        StoreAccess.accepts(provider: fresh.activeProvider, platform: "apple")
      else {
        products = []
        return
      }
      let received = try await Product.products(for: fresh.items.map(\.productId))
      try Task.checkCancellation()
      guard model.session?.user.id == owner else { return }
      products = received.filter { $0.type == .autoRenewable }
      if products.isEmpty { notice = "L’App Store ne propose aucune formule pour ce compte." }
    } catch {
      if !Task.isCancelled {
        products = []
        notice = "Informations de l’App Store indisponibles. Réessayez dans un instant."
      }
    }
  }
  func watch(_ model: MemberModel, owner: String) async {
    bound = owner
    products = []
    catalog = nil
    notice = ""
    retry = false
    await withTaskGroup(of: Void.self) { group in
      group.addTask {
        for await result in StoreKit.Transaction.updates {
          guard !Task.isCancelled else { return }
          _ = await self.confirm(result, model: model, owner: owner)
        }
      }
      group.addTask {
        await self.resume(model, owner: owner)
        while !Task.isCancelled {
          do { try await Task.sleep(for: .seconds(60)) } catch { return }
          if await self.retry {
            await self.resume(model, owner: owner)
          } else {
            await self.releaseReady(model, owner: owner)
          }
        }
      }
      await group.waitForAll()
    }
    if bound == owner {
      bound = nil
      products = []
      catalog = nil
      notice = ""
      retry = false
    }
  }
  private func release(_ record: ReceiptOrder, owner: String, model: MemberModel) async {
    guard model.session?.user.id == owner, let id = UUID(uuidString: record.order.id) else {
      return
    }
    do {
      if (Practice.date(record.order.expiresAt) ?? .distantPast) > Date() {
        let path =
          APIPath.cancelStoreOrder.replacingOccurrences(of: "{platform}", with: "apple")
          + "?orderId=" + id.uuidString
        _ = try await model.authorizedAPI().raw(path, method: "DELETE")
      }
      guard model.session?.user.id == owner else { return }
      try model.removePrivate(kind: "receipt-order", id: record.reference)
    } catch AppError.status(let status) where [404, 409].contains(status) {
      if model.session?.user.id == owner {
        try? model.removePrivate(kind: "receipt-order", id: record.reference)
      }
    } catch {}
  }
  private func releaseReady(_ model: MemberModel, owner: String) async {
    for record in (try? model.privateRows(ReceiptOrder.self, kind: "receipt-order")) ?? [] {
      if record.confirmed || (Practice.date(record.order.expiresAt) ?? .distantPast) <= Date() {
        await release(record, owner: owner, model: model)
      }
    }
  }
  func resume(_ model: MemberModel, owner: String) async {
    await releaseReady(model, owner: owner)
    for await result in StoreKit.Transaction.unfinished {
      if Task.isCancelled || model.session?.user.id != owner { return }
      _ = await confirm(result, model: model, owner: owner)
    }
  }
  @discardableResult
  private func confirm(
    _ result: VerificationResult<StoreKit.Transaction>, model: MemberModel, owner: String,
    explicit: Bool = false, order: StoreOrder? = nil
  ) async -> Bool {
    guard model.session?.user.id == owner, !Task.isCancelled else { return false }
    guard case .verified(let transaction) = result else {
      notice = "L’App Store n’a pas pu authentifier cet achat."
      return false
    }
    guard transaction.productType == .autoRenewable else { return false }
    let reference = String(transaction.id)
    if let order, transaction.appAccountToken?.uuidString.lowercased() == owner.lowercased() {
      do {
        try model.persistPrivate(
          ReceiptOrder(reference: reference, order: order, confirmed: false), kind: "receipt-order",
          id: reference)
      } catch {
        notice = model.message(error)
        return false
      }
    }
    let inserted = processing.insert(transaction.id).inserted
    guard inserted || order != nil else { return false }
    defer { if inserted { processing.remove(transaction.id) } }
    do {
      let api = try model.authorizedAPI()
      let verified = try await AppleReceiptDelivery.deliver(transaction, owner: owner) { body in
        let data = try await api.raw(
          APIPath.verifyStorePurchase, method: "POST", body: JSONEncoder().encode(body))
        return try JSONDecoder().decode(StoreReceipt.self, from: data)
      } finish: {
        guard model.session?.user.id == owner else { throw CancellationError() }
        if var record = try model.privateRows(ReceiptOrder.self, kind: "receipt-order").first(
          where: { $0.reference == reference })
        {
          record.confirmed = true
          try model.persistPrivate(record, kind: "receipt-order", id: reference)
          await release(record, owner: owner, model: model)
        }
        await transaction.finish()
      }
      guard model.session?.user.id == owner else { return false }
      notice = StoreAccess.message(verified.status)
      if ["active", "expired", "revoked"].contains(verified.status) {
        await model.sync()
        _ = try? await catalogFor(model)
        retry = false
        return true
      }
      retry = true
    } catch ReceiptDeliveryError.otherMember {
      if explicit { notice = AppleReceiptDelivery.otherMember }
    } catch AppError.status(let status) where [400, 403, 404, 409, 422].contains(status) {
      retry = false
      if explicit {
        notice =
          "Cette transaction ne peut pas être attribuée à ce compte CRPE. Vérifiez le compte utilisé lors de la souscription."
      }
    } catch {
      if model.session?.user.id == owner {
        retry = true
        notice =
          "Votre achat sera vérifié dès que le service sera disponible. Vous pouvez aussi restaurer vos achats."
      }
    }
    return false
  }
  func buy(_ product: Product, tier: String, model: MemberModel) async {
    guard !busy, let owner = model.session?.user.id else { return }
    guard let displayed = catalog else { return }
    busy = true
    notice = ""
    defer { busy = false }
    do {
      let fresh = try await catalogFor(model)
      guard
        displayed.activeProvider == fresh.activeProvider
          && displayed.activeProductId == fresh.activeProductId
      else {
        await model.sync()
        await load(model, duringCheckout: true)
        notice = "Votre abonnement vient de changer. Choisissez à nouveau la formule souhaitée."
        return
      }
      guard fresh.enabled, model.access?.salesEnabled == true,
        StoreAccess.accepts(provider: fresh.activeProvider, platform: "apple"),
        fresh.items.contains(where: { $0.productId == product.id && $0.tier == tier }),
        fresh.activeProductId != product.id, let account = UUID(uuidString: fresh.accountToken)
      else {
        notice = "Cette formule ne peut pas être souscrite pour ce compte."
        return
      }
      let api = try model.authorizedAPI()
      let input = StoreOrderInput(
        platform: "apple", productId: product.id, basePlanId: nil, clientId: UUID().uuidString,
        expectedActiveProductId: fresh.activeProvider == "apple" ? fresh.activeProductId : nil,
        expectedActiveBasePlanId: nil)
      let payload = try JSONEncoder().encode(input)
      func reserve() async throws -> Data {
        try await api.raw(APIPath.reserveStoreOrder, method: "POST", body: payload)
      }
      let bytes: Data
      do { bytes = try await reserve() } catch let network as URLError
        where [.networkConnectionLost, .timedOut].contains(network.code)
      { bytes = try await reserve() } catch AppError.status(let status)
        where (500...599).contains(status)
      { bytes = try await reserve() }
      let order = try JSONDecoder().decode(StoreOrder.self, from: bytes)
      guard UUID(uuidString: order.id) != nil else { throw AppError.invalidResponse }
      guard model.session?.user.id == owner else { return }
      let result = try await product.purchase(options: [.appAccountToken(account)])
      switch result {
      case .success(let transaction):
        _ = await confirm(transaction, model: model, owner: owner, explicit: true, order: order)
      case .pending: notice = "Achat en attente de confirmation par l’App Store."
      case .userCancelled:
        let cancellation = ReceiptOrder(
          reference: "cancel-" + order.id, order: order, confirmed: true)
        if model.session?.user.id == owner {
          try? model.persistPrivate(cancellation, kind: "receipt-order", id: cancellation.reference)
          await release(cancellation, owner: owner, model: model)
        }
        notice = "Achat annulé."
      @unknown default: notice = "L’App Store n’a pas confirmé cet achat."
      }
    } catch AppError.status(409) {
      await model.sync()
      await load(model, duringCheckout: true)
      notice = "Un abonnement ou une autre commande est déjà en cours. Actualisez votre espace."
    } catch {
      notice =
        "Impossible de terminer l’achat pour le moment. Restaurez vos achats si l’App Store l’a confirmé."
    }
  }
  func restore(_ model: MemberModel) async {
    guard let owner = model.session?.user.id, !busy else { return }
    busy = true
    defer { busy = false }
    do {
      try await AppStore.sync()
      await resume(model, owner: owner)
      var found = false
      var other = false
      for await result in StoreKit.Transaction.currentEntitlements {
        guard model.session?.user.id == owner else { return }
        if case .verified(let transaction) = result,
          transaction.appAccountToken?.uuidString.lowercased() == owner.lowercased()
        {
          found = true
          _ = await confirm(result, model: model, owner: owner, explicit: true)
        } else if case .verified(let transaction) = result,
          transaction.productType == .autoRenewable
        {
          other = true
        }
      }
      await model.sync()
      if !found && !retry {
        notice =
          other
          ? AppleReceiptDelivery.otherMember
          : "Aucun abonnement App Store actif à restaurer pour ce compte."
      }
    } catch {
      notice = "La restauration n’a pas abouti. Vérifiez votre compte App Store et réessayez."
    }
  }
  func manage() async {
    guard
      let scene = UIApplication.shared.connectedScenes.compactMap({ $0 as? UIWindowScene }).first(
        where: { $0.activationState == .foregroundActive })
    else { return }
    do { try await AppStore.showManageSubscriptions(in: scene) } catch {
      notice = "La gestion de l’abonnement est indisponible pour le moment."
    }
  }
}
struct StoreScreen: View {
  @EnvironmentObject var model: MemberModel
  @EnvironmentObject var store: StorePurchases
  var tier: String?
  private func period(_ product: Product) -> String {
    guard let p = product.subscription?.subscriptionPeriod else { return "" }
    let unit =
      switch p.unit {
      case .day: p.value == 1 ? "jour" : "jours"
      case .week: p.value == 1 ? "semaine" : "semaines"
      case .month: "mois"
      case .year: p.value == 1 ? "an" : "ans"
      @unknown default: "périodes"
      }
    return p.value == 1 ? unit : "\(p.value) " + unit
  }
  var body: some View {
    List {
      if let requested = tier {
        Text("Cette activité est incluse dans la formule " + FeatureAccess.tier(requested) + ".")
      }
      if let active = model.access?.tier, active != "none" {
        LabeledContent("Formule actuelle", value: FeatureAccess.tier(active))
      }
      if let listing = store.catalog {
        if let other = listing.activeProvider, other != "apple" {
          Text(
            other == "google"
              ? "Votre abonnement est géré par Google Play."
              : "Votre abonnement est géré depuis l’espace membre web.")
        } else if listing.enabled && model.access?.salesEnabled == true {
          let offers = listing.items.filter { tier == nil || $0.tier == tier }
          ForEach(offers, id: \.productId) { item in
            if let product = store.products.first(where: { $0.id == item.productId }) {
              Section(product.displayName) {
                Text(product.description)
                Text(product.displayPrice + " / " + period(product)).font(.headline)
                if listing.activeProductId != product.id {
                  Button("Choisir la formule " + FeatureAccess.tier(item.tier)) {
                    Task { await store.buy(product, tier: item.tier, model: model) }
                  }
                  .disabled(store.busy)
                }
              }
            }
          }
          if offers.isEmpty || store.products.isEmpty {
            Text("Aucune offre disponible dans l’App Store pour cette formule.")
          }
          if !store.products.isEmpty {
            Text(
              "L’abonnement se renouvelle automatiquement. Vous pouvez le résilier depuis votre compte App Store."
            )
            if let url = FeatureAccess.https(listing.termsUrl) {
              Link("Conditions d’utilisation", destination: url)
            }
            if let url = FeatureAccess.https(listing.privacyUrl) {
              Link("Confidentialité", destination: url)
            }
          }
        } else if model.access?.salesEnabled == true {
          Text("Les achats App Store ne sont pas disponibles pour le moment.")
        }
        if listing.activeProvider == "apple" {
          Button("Gérer mon abonnement App Store") { Task { await store.manage() } }
        }
      }
      Button("Restaurer mes achats App Store") { Task { await store.restore(model) } }.disabled(
        store.busy)
      if store.busy { ProgressView() }
      if !store.notice.isEmpty { Text(store.notice) }
    }.navigationTitle(tier.map { FeatureAccess.tier($0) } ?? "Mon abonnement")
      .task { await store.load(model) }
      .refreshable {
        await model.sync()
        await store.load(model)
      }
  }
}
