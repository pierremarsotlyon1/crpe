import StoreKit
import StoreKitTest
import XCTest

@testable import PrepaCRPE

final class StoreKitTests: XCTestCase {
  @MainActor func testStoreKitBindsPurchaseToAccountAndExpiresSubscription() async throws {
    let fixture = try XCTUnwrap(
      Bundle(for: Self.self).url(forResource: "Subscriptions", withExtension: "storekit"))
    let testing = try SKTestSession(contentsOf: fixture)
    testing.resetToDefaultState()
    testing.disableDialogs = true
    testing.clearTransactions()
    defer { testing.clearTransactions() }
    let id = "test.crpe.store.subscription"
    let catalog = try await Product.products(for: [id])
    let subscription = try XCTUnwrap(catalog.first)
    XCTAssertEqual(subscription.type, .autoRenewable)
    XCTAssertFalse(subscription.displayPrice.isEmpty)
    let member = UUID()
    let purchase = try await subscription.purchase(options: [.appAccountToken(member)])
    guard case .success(let response) = purchase else {
      return XCTFail("The local purchase must complete")
    }
    let transaction = try response.payloadValue
    XCTAssertEqual(transaction.appAccountToken, member)
    XCTAssertFalse(String(transaction.id).isEmpty)
    // The test endpoint is deliberately isolated from Apple production verification.
    var submitted: [String] = []
    var finished = 0
    do {
      _ = try await AppleReceiptDelivery.deliver(transaction, owner: UUID().uuidString) { _ in
        XCTFail("A foreign transaction must not be sent")
        return StoreReceipt(status: "active")
      } finish: {
        XCTFail("A foreign transaction must not be finished")
      }
      XCTFail("Expected an account mismatch")
    } catch ReceiptDeliveryError.otherMember {}
    do {
      _ = try await AppleReceiptDelivery.deliver(transaction, owner: member.uuidString) { input in
        submitted.append(input.reference)
        throw AppError.status(503)
      } finish: {
        finished += 1
      }
      XCTFail("Expected verification to retry")
    } catch AppError.status(503) {}
    for status in ["pending", "active"] {
      _ = try await AppleReceiptDelivery.deliver(transaction, owner: member.uuidString) { input in
        XCTAssertEqual(input.productId, id)
        submitted.append(input.reference)
        return StoreReceipt(status: status)
      } finish: {
        finished += 1
        await transaction.finish()
      }
      XCTAssertEqual(finished, status == "pending" ? 0 : 1)
    }
    XCTAssertEqual(Set(submitted), [String(transaction.id)])
    try testing.expireSubscription(productIdentifier: id)
    let info = try XCTUnwrap(subscription.subscription)
    var states: [Product.SubscriptionInfo.Status] = []
    for _ in 0..<20 {
      states = try await info.status
      if states.contains(where: { $0.state == .expired }) { break }
      try await Task.sleep(for: .milliseconds(200))
    }
    XCTAssertTrue(states.contains(where: { $0.state == .expired }))

  }
  @MainActor func testAskToBuyRemainsPendingUntilTheStoreApproves() async throws {
    let fixture = try XCTUnwrap(
      Bundle(for: Self.self).url(forResource: "Subscriptions", withExtension: "storekit"))
    let testing = try SKTestSession(contentsOf: fixture)
    testing.clearTransactions()
    testing.resetToDefaultState()
    testing.disableDialogs = true
    testing.askToBuyEnabled = true
    defer {
      testing.askToBuyEnabled = false
      testing.clearTransactions()
    }
    let id = "test.crpe.store.subscription"
    let offers = try await Product.products(for: [id])
    let product = try XCTUnwrap(offers.first)
    let purchase = try await product.purchase(options: [.appAccountToken(UUID())])
    guard case .pending = purchase else {
      return XCTFail("Ask to Buy must return a pending transaction")
    }
    let transaction = try XCTUnwrap(testing.allTransactions().first)
    try testing.declineAskToBuyTransaction(identifier: transaction.identifier)
    var active = false
    for await entry in StoreKit.Transaction.currentEntitlements {
      if case .verified(let item) = entry, item.productID == id { active = true }
    }
    XCTAssertFalse(active)
  }
}
