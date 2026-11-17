import XCTest

@testable import PrepaCRPE

final class PracticeTests: XCTestCase {
  func item(_ id: String, format: String = "choice", status: String = "validated") -> Content {
    Content(
      id: id, examId: "crpe", kind: "exercise", title: "Fixture", body: "", skill: id, position: 1,
      format: format, choices: [], criteria: [], videoUrl: "", status: status, sourceTitle: "",
      sourceUrl: "", sourceYear: 0, durationMinutes: 0, prepMinutes: 0, points: 1, documents: [])
  }
  func entry(_ id: String, _ date: String, correct: Bool) -> LocalAnswer {
    let input = AttemptInput(
      clientId: UUID().uuidString, exerciseId: id, answer: "fixture",
      performedAt: "2026-11-10T12:00:00Z")
    return LocalAnswer(
      input: input,
      receipt: Attempt(
        clientId: input.clientId, exerciseId: id, correct: correct, feedback: "", nextReviewAt: date
      ))
  }
  func testNativeStoreDoesNotOfferDuplicateSubscriptionsAndHandlesUnverifiedStates() {
    XCTAssertTrue(StoreAccess.accepts(provider: nil, platform: "apple"))
    XCTAssertTrue(StoreAccess.accepts(provider: "apple", platform: "apple"))
    XCTAssertFalse(StoreAccess.accepts(provider: "stripe", platform: "apple"))
    XCTAssertFalse(StoreAccess.accepts(provider: "google", platform: "apple"))
    XCTAssertEqual(StoreAccess.message("future-state"), StoreAccess.message("pending"))
    XCTAssertNotEqual(StoreAccess.message("expired"), StoreAccess.message("active"))
    XCTAssertNil(FeatureAccess.https("https://private@exemple.test"))
  }
  func testPaidOfflineAccessExpiresAndFreeBaseRemainsAvailable() {
    let now = Date(timeIntervalSince1970: 1_763_320_000)
    let dates = ISO8601DateFormatter()
    let live = Capability(key: "live", label: "Direct", requiredTier: "suivi", allowed: true)
    let course = Capability(key: "course", label: "Cours", requiredTier: "autonomie", allowed: true)
    var access = MemberAccess(
      tier: "suivi", salesEnabled: true, features: [live, course],
      validUntil: dates.string(from: now.addingTimeInterval(-1)))
    XCTAssertFalse(FeatureAccess.eligible(live, access: access, at: now))
    XCTAssertFalse(FeatureAccess.eligible(course, access: access, at: now))
    access.providers = [
      ActiveProvider(
        provider: "apple", tier: "autonomie", planKey: "annual",
        expiresAt: dates.string(from: now.addingTimeInterval(60)))
    ]
    XCTAssertTrue(FeatureAccess.eligible(course, access: access, at: now))
    XCTAssertFalse(FeatureAccess.eligible(live, access: access, at: now))
    access.providers = nil
    access.salesEnabled = false
    XCTAssertTrue(FeatureAccess.eligible(course, access: access, at: now))
    XCTAssertFalse(FeatureAccess.eligible(live, access: access, at: now))
  }
  func testSpacingAndPending() {
    let now = Date(timeIntervalSince1970: 1_794_916_800)
    let failed = entry("failed", "2026-11-11T12:00:00Z", correct: false)
    let future = entry("future", "2026-11-30T12:00:00Z", correct: true)
    var pending = entry("pending", "2026-11-10T12:00:00Z", correct: true)
    pending.receipt = nil
    let questions = [
      item("new"), item("future"), item("draft", status: "draft"), item("copy", format: "text"),
      item("failed"), item("pending"),
    ]
    XCTAssertEqual(
      Practice.due(questions, answers: [future, failed, pending], skills: [], now: now).map(\.id),
      ["failed", "new"])
  }
  func testScopeRestartAndReceiptReplay() throws {
    let folder = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: folder) }
    let db = folder.appendingPathComponent("db.sqlite")
    var answer = entry("exercise", "2026-11-30T12:00:00Z", correct: true)
    let receipt = answer.receipt
    answer.receipt = nil
    do {
      let store = try LearningStore(url: db)
      try store.save(answer, owner: "first", kind: "answer", id: answer.input.clientId)
    }
    let store = try LearningStore(url: db)
    XCTAssertTrue(try store.rows(LocalAnswer.self, owner: "second", kind: "answer").isEmpty)
    XCTAssertEqual(
      try store.rows(LocalAnswer.self, owner: "first", kind: "answer").first?.input, answer.input)
    answer.receipt = receipt
    for _ in 0..<2 {
      try store.save(answer, owner: "first", kind: "answer", id: answer.input.clientId)
    }
    XCTAssertEqual(try store.rows(LocalAnswer.self, owner: "first", kind: "answer").count, 1)
    let draft = CopyDraft(id: "photo", exerciseId: "exercise", text: "", hasImage: true)
    try store.save(draft, owner: "first", kind: "copy", id: "photo", image: Data([1, 2, 3]))
    XCTAssertEqual(try store.image(owner: "first", id: "photo"), Data([1, 2, 3]))
    try store.remove(owner: "first")
    XCTAssertNil(try store.image(owner: "first", id: "photo"))
  }
  func testMagicLinkBoundary() {
    XCTAssertEqual(
      Practice.token(from: URL(string: "crpemaths://auth#token=abcdefghijklmnop")!),
      "abcdefghijklmnop")
    for path in [
      "https://auth#token=abcdefghijklmnop", "crpemaths://other#token=abcdefghijklmnop",
      "crpemaths://auth?token=abcdefghijklmnop", "crpemaths://user@auth#token=abcdefghijklmnop",
    ] { XCTAssertNil(Practice.token(from: URL(string: path)!)) }
  }
  func testCorrectionPollingStopsOnExitAndTerminalStages() {
    let all = ["queued", "processing", "retrying", "teacher_review", "completed", "failed"]
    XCTAssertEqual(Set(all.map(CopyTracking.label)).count, 6)
    for stage in all {
      XCTAssertEqual(
        CopyTracking.shouldPoll(visible: true, active: true, signedIn: true, stages: [stage]),
        ["queued", "processing", "retrying"].contains(stage))
    }
    XCTAssertFalse(
      CopyTracking.shouldPoll(visible: false, active: true, signedIn: true, stages: ["processing"]))
    XCTAssertFalse(
      CopyTracking.shouldPoll(visible: true, active: false, signedIn: true, stages: ["processing"]))
    XCTAssertFalse(
      CopyTracking.shouldPoll(visible: true, active: true, signedIn: false, stages: ["processing"]))
  }
}
