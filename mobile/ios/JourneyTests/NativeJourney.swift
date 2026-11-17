import XCTest

final class NativeJourney: XCTestCase {
  func testEmailCourseAndOfflineAttempt() throws {
    let app = XCUIApplication()
    app.launch()
    func activate(_ field: XCUIElement) {
      field.tap()
      if !app.keyboards.firstMatch.waitForExistence(timeout: 2) { field.tap() }
    }
    let email = try XCTUnwrap(
      Bundle(for: Self.self).object(forInfoDictionaryKey: "CRPETestEmail") as? String)
    let input = app.textFields["login-email"]
    XCTAssertTrue(input.waitForExistence(timeout: 12))
    input.tap()
    input.typeText(email)
    app.buttons["request-login"].tap()
    let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
    let open = springboard.alerts.buttons.matching(
      NSPredicate(format: "label IN %@", ["Ouvrir", "Open"])
    ).firstMatch
    if open.waitForExistence(timeout: 15) { open.tap() }
    XCTAssertTrue(app.tabBars.buttons["Parcours"].waitForExistence(timeout: 45))
    app.tabBars.buttons["Parcours"].tap()
    XCTAssertTrue(app.navigationBars["Mon parcours"].exists)
    app.tabBars.buttons["Journée"].tap()
    let start = app.buttons["Commencer ma séance"]
    XCTAssertTrue(
      start.waitForExistence(timeout: 20),
      "Publish at least one eligible exercise in the test database.")
    start.tap()
    let title = app.staticTexts["practice-title"].label
    let numeric = app.textFields["Réponse numérique"]
    if numeric.exists {
      activate(numeric)
      numeric.typeText("42")
    } else {
      let choice = app.buttons.matching(identifier: "practice-choice").firstMatch
      XCTAssertTrue(choice.exists)
      choice.tap()
    }
    app.buttons["Valider ma réponse"].tap()
    XCTAssertTrue(
      app.staticTexts.containing(NSPredicate(format: "label BEGINSWITH %@", "Réponse enregistrée."))
        .firstMatch.waitForExistence(timeout: 12))
    app.terminate()
    app.launch()
    XCTAssertTrue(app.tabBars.buttons["Compte"].waitForExistence(timeout: 15))
    app.tabBars.buttons["Compte"].tap()
    app.buttons["Synchroniser maintenant"].tap()
    app.tabBars.buttons["Journée"].tap()
    XCTAssertTrue(
      app.staticTexts["Juste"].waitForExistence(timeout: 4)
        || app.staticTexts["À retravailler"].waitForExistence(timeout: 12))
    app.tabBars.buttons["Copies"].tap()
    XCTAssertTrue(app.navigationBars["Mes copies"].exists)
    if Bundle(for: Self.self).object(forInfoDictionaryKey: "CRPETestPhoto") as? String == "yes" {
      app.buttons["copy-exercise-picker"].tap()
      app.buttons[title].firstMatch.tap()
      app.buttons["Choisir une photo"].tap()
      let image = app.images.matching(identifier: "PXGGridLayout-Info").firstMatch
      XCTAssertTrue(image.waitForExistence(timeout: 12), app.debugDescription)
      let point = image.frame
      app.coordinate(withNormalizedOffset: .zero).withOffset(
        CGVector(dx: point.midX, dy: point.midY)
      ).tap()
      XCTAssertTrue(app.images["Aperçu de la copie"].waitForExistence(timeout: 12))
      app.buttons["Transmettre ma copie"].tap()
      let marker = app.staticTexts.containing(
        NSPredicate(format: "label BEGINSWITH %@", "Dernière mise à jour ·")
      ).firstMatch
      if !marker.waitForExistence(timeout: 5) { app.swipeUp() }
      XCTAssertTrue(marker.waitForExistence(timeout: 15), app.debugDescription)
    }
    if let mock = Bundle(for: Self.self).object(forInfoDictionaryKey: "CRPETestMock") as? String,
      !mock.isEmpty
    {
      app.tabBars.buttons["Activités"].tap()
      let locked = app.buttons.containing(
        NSPredicate(format: "label CONTAINS %@", "Rendez-vous hebdomadaire")
      ).firstMatch
      XCTAssertTrue(locked.waitForExistence(timeout: 10), app.debugDescription)
      locked.tap()
      XCTAssertTrue(
        app.staticTexts["Cette activité est incluse dans la formule Suivi."].waitForExistence(
          timeout: 8))
      app.navigationBars.buttons.element(boundBy: 0).tap()
      func enterMock() {
        let mocks = app.buttons["Sujets blancs"]
        XCTAssertTrue(mocks.waitForExistence(timeout: 10), app.debugDescription)
        mocks.tap()
        let subject = app.buttons["mock-start-" + mock]
        for _ in 0..<12 where !subject.isHittable { app.swipeUp() }
        XCTAssertTrue(subject.isHittable, app.debugDescription)
        subject.tap()
      }
      enterMock()
      let numeric = app.textFields["mock-answer-numeric"].firstMatch
      let written = app.textFields["mock-answer-text"].firstMatch
      XCTAssertTrue(numeric.waitForExistence(timeout: 10), app.debugDescription)
      activate(numeric)
      numeric.typeText("7")
      activate(written)
      written.typeText("Je regroupe les unites.")
      XCTAssertTrue(
        app.staticTexts["Réponses synchronisées."].waitForExistence(timeout: 8),
        app.debugDescription)
      app.terminate()
      app.launch()
      app.tabBars.buttons["Activités"].tap()
      enterMock()
      XCTAssertTrue(written.waitForExistence(timeout: 10), app.debugDescription)
      XCTAssertEqual(written.value as? String, "Je regroupe les unites.")
      let finish = app.buttons["Rendre ma copie"]
      if !finish.isHittable { app.swipeUp() }
      finish.tap()
      app.sheets.buttons["Rendre ma copie"].tap()
      XCTAssertTrue(
        app.staticTexts["Résultat provisoire"].waitForExistence(timeout: 10), app.debugDescription)
    }
    app.tabBars.buttons["Compte"].tap()
    app.buttons["Mon abonnement"].tap()
    XCTAssertTrue(app.buttons["Restaurer mes achats App Store"].waitForExistence(timeout: 10))
    XCTAssertFalse(
      app.buttons.containing(NSPredicate(format: "label BEGINSWITH %@", "Choisir la formule"))
        .firstMatch.exists)
    app.navigationBars.buttons.element(boundBy: 0).tap()
    app.buttons["Activer le rappel"].tap()
    let permission = springboard.alerts.buttons.matching(
      NSPredicate(format: "label IN %@", ["Allow", "Autoriser"])
    ).firstMatch
    if permission.waitForExistence(timeout: 5) { permission.tap() }
    XCTAssertTrue(
      app.staticTexts["Rappel actif à 19 h."].waitForExistence(timeout: 10), app.debugDescription)
    app.buttons["Désactiver"].tap()
    XCTAssertTrue(app.staticTexts["Le rappel est désactivé."].waitForExistence(timeout: 8))
    app.buttons["Supprimer mon compte"].tap()
    app.sheets.buttons["Supprimer mon compte"].tap()
    XCTAssertTrue(input.waitForExistence(timeout: 12))
  }
}
