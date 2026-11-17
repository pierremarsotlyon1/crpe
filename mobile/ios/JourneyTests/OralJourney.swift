import XCTest

final class OralJourney: XCTestCase {
  func testRecordedTalkQuestionsAndCriteria() throws {
    let email =
      Bundle(for: Self.self).object(forInfoDictionaryKey: "CRPETestEmail") as? String ?? ""
    try XCTSkipUnless(
      email.hasPrefix("oral-ios-"), "Requires the isolated oral API and audio fixtures")
    let app = XCUIApplication()
    app.launch()
    let login = app.textFields["login-email"]
    XCTAssertTrue(login.waitForExistence(timeout: 15))
    login.tap()
    login.typeText(email)
    app.buttons["request-login"].tap()
    let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    let open = springboard.alerts.buttons.matching(
      NSPredicate(format: "label IN %@", ["Ouvrir", "Open"])
    ).firstMatch
    if open.waitForExistence(timeout: 8) { open.tap() }
    let activities = app.tabBars.buttons["Activités"]
    XCTAssertTrue(activities.waitForExistence(timeout: 50))
    activities.tap()
    app.buttons["Entraînement à l’oral"].tap()
    let draw = app.buttons["oral-start"]
    XCTAssertTrue(draw.waitForExistence(timeout: 15))
    draw.tap()
    func reveal(_ query: XCUIElement, down: Bool = true) {
      for _ in 0..<6 {
        if query.exists && query.isHittable { return }
        if down { app.swipeUp() } else { app.swipeDown() }
      }
    }
    let transcribe = app.buttons["Transcrire l’enregistrement"].firstMatch
    // The runner supplies two real AAC test clips in this new test member's private sandbox.
    let first = app.buttons["oral-record--1"]
    if first.exists { Thread.sleep(forTimeInterval: 2) }
    reveal(transcribe)
    XCTAssertTrue(transcribe.exists, app.debugDescription)
    transcribe.tap()
    let notice = app.staticTexts["oral-status"]
    let completed = NSPredicate(format: "label BEGINSWITH %@", "Transcription ajoutée.")
    expectation(for: completed, evaluatedWith: notice)
    waitForExpectations(timeout: 20)
    let listen = app.buttons["Écouter la question"].firstMatch
    reveal(listen)
    listen.tap()
    let second = app.buttons["Transcrire l’enregistrement"].firstMatch
    reveal(second)
    XCTAssertTrue(second.waitForExistence(timeout: 8))
    second.tap()
    let answer = app.descendants(matching: .any).matching(identifier: "oral-text-0").firstMatch
    reveal(answer)
    expectation(for: NSPredicate(format: "value CONTAINS %@", "Transcription de test"), evaluatedWith: answer)
    waitForExpectations(timeout: 20)
    for index in 1...2 {
      let field = app.descendants(matching: .any).matching(identifier: "oral-text-\(index)")
        .firstMatch
      reveal(field)
      if index == 2 && !field.exists { break }
      XCTAssertTrue(field.exists)
      field.tap()
      field.typeText("Argument de test.")
      // List scrolling dismisses the software keyboard.
      app.swipeUp()
    }
    let submit = app.buttons["oral-submit"]
    reveal(submit)
    XCTAssertTrue(submit.isEnabled)
    submit.tap()
    app.buttons["Transmettre"].tap()
    let refresh = app.buttons["Actualiser les séances"]
    reveal(refresh)
    refresh.tap()
    let teacher = app.staticTexts["Évaluation enseignante"]
    for _ in 0..<3 where !teacher.exists {
      Thread.sleep(forTimeInterval: 1)
      if refresh.isHittable { refresh.tap() }
      reveal(teacher)
    }
    XCTAssertTrue(teacher.exists, app.debugDescription)
    let all = app.buttons["Toutes mes séances"]
    reveal(all)
    all.tap()
    XCTAssertTrue(app.staticTexts["Progression par critère"].waitForExistence(timeout: 6))
  }
}
