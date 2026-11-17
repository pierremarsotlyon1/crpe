import XCTest

@testable import PrepaCRPE

final class OralPracticeTests: XCTestCase {
  func testDraftPreservesTeacherQuestionsAndRejectsEmptyResponse() throws {
    let item = OralSession(
      id: UUID().uuidString, subjectId: UUID().uuidString, title: "Oral", body: "Sujet",
      criteria: ["Clarté"], questions: ["Pourquoi ?", "Comment ?"], prepMinutes: 10,
      durationMinutes: 15, createdAt: "", status: "active", transcript: "", answers: [],
      feedback: [], feedbackSource: "")
    var draft = OralDraft(item)
    XCTAssertFalse(draft.complete)
    XCTAssertTrue(draft.edit("Exposé", slot: -1))
    XCTAssertTrue(draft.edit("Argument", slot: 0))
    XCTAssertFalse(draft.complete)
    XCTAssertFalse(draft.edit("Hors sujet", slot: 2))
    XCTAssertTrue(draft.edit("Démonstration", slot: 1))
    XCTAssertFalse(draft.edit(String(repeating: "é", count: 8001), slot: 1))
    let restored = try JSONDecoder().decode(OralDraft.self, from: JSONEncoder().encode(draft))
    XCTAssertTrue(restored.complete)
    XCTAssertEqual(restored.value(1), "Démonstration")
  }
  func testRecordingMultipartAndScopedPaths() throws {
    var audio = Data([0, 0, 0, 24])
    audio.append(contentsOf: "ftypM4A 0000".utf8)
    let part = try OralClip.multipart(audio, boundary: "test")
    XCTAssertNotNil(part.range(of: audio))
    XCTAssertNotNil(part.range(of: Data("name=\"audio\"".utf8)))
    XCTAssertThrowsError(
      try OralClip.multipart(Data(repeating: 0, count: OralClip.limit + 1), boundary: "test"))
    let folder = URL(fileURLWithPath: "/fixture")
    let id = UUID().uuidString
    XCTAssertNotEqual(
      try OralClip.url(folder: folder.appendingPathComponent("A"), session: id, slot: -1),
      try OralClip.url(folder: folder.appendingPathComponent("B"), session: id, slot: -1))
    XCTAssertThrowsError(try OralClip.url(folder: folder, session: "../other", slot: 0))
  }
}
