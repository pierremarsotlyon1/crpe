import Foundation
import SwiftUI
import UserNotifications

@MainActor final class MemberModel: ObservableObject {
  @Published private(set) var session: Session?
  @Published private(set) var snapshot: Snapshot?
  @Published private(set) var answers: [LocalAnswer] = []
  @Published private(set) var copies: [CopyDraft] = []
  @Published var notice = ""
  @Published var authenticating = false
  @Published private(set) var syncing = false
  @Published var reminder: Int?
  @Published private(set) var access: MemberAccess?
  @Published var trackingNotice = ""
  private let store: LearningStore?
  private var syncAgain = false
  init() {
    do {
      let folder = try FileManager.default.url(
        for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
      var path = folder.appendingPathComponent("Practice", isDirectory: true)
      try FileManager.default.createDirectory(
        at: path, withIntermediateDirectories: true,
        attributes: [.protectionKey: FileProtectionType.completeUntilFirstUserAuthentication])
      var values = URLResourceValues()
      values.isExcludedFromBackup = true
      try path.setResourceValues(values)
      store = try LearningStore(url: path.appendingPathComponent("practice.sqlite"))
    } catch {
      store = nil
      notice = AppError.storage.localizedDescription
    }
    do {
      session = try Secrets.restore()
      try reload()
    } catch { notice = AppError.storage.localizedDescription }
  }
  func authorizedAPI() throws -> MemberAPI {
    guard let token = session?.token else { throw AppError.status(401) }
    return try MemberAPI(token)
  }
  func privateRows<T: Decodable>(_ type: T.Type, kind: String) throws -> [T] {
    guard let owner = session?.user.id, let store else { throw AppError.storage }
    return try store.rows(type, owner: owner, kind: kind)
  }
  func persistPrivate<T: Encodable>(_ value: T, kind: String, id: String) throws {
    guard let owner = session?.user.id, let store else { throw AppError.storage }
    try store.save(value, owner: owner, kind: kind, id: id)
  }
  func removePrivate(kind: String, id: String) throws {
    guard let owner = session?.user.id, let store else { throw AppError.storage }
    try store.remove(owner: owner, kind: kind, id: id)
  }
  func reload() throws {
    guard let store else { throw AppError.storage }
    guard let owner = session?.user.id else {
      access = nil
      snapshot = nil
      answers = []
      copies = []
      return
    }
    access = try store.rows(MemberAccess.self, owner: owner, kind: "access").first
    snapshot = try store.rows(Snapshot.self, owner: owner, kind: "snapshot").first
    answers = try store.rows(LocalAnswer.self, owner: owner, kind: "answer")
    copies = try store.rows(CopyDraft.self, owner: owner, kind: "copy")
  }
  func request(_ email: String) async {
    guard !authenticating else { return }
    authenticating = true
    defer { authenticating = false }
    do {
      _ = try await MemberAPI().raw(
        APIPath.requestLogin, method: "POST",
        body: JSONEncoder().encode(
          LinkRequest(
            email: email.trimmingCharacters(in: .whitespacesAndNewlines), client: "mobile")))
      notice = "Consultez votre messagerie sur ce téléphone et ouvrez le lien de connexion."
    } catch { notice = message(error) }
  }
  func verify(_ url: URL) async {
    guard let value = Practice.token(from: url), session == nil else { return }
    do {
      let response = try await MemberAPI().raw(
        APIPath.verifyLogin, method: "POST",
        body: JSONEncoder().encode(VerifyRequest(token: value, client: "mobile")))
      let member = try JSONDecoder().decode(Session.self, from: response)
      guard member.token?.isEmpty == false else { throw AppError.invalidResponse }
      try Secrets.save(member)
      session = member
      try reload()
      notice = ""
      await sync()
    } catch { notice = message(error) }
  }
  func queue(_ id: String, answer: String) throws {
    guard allows("practice") else { throw AppError.status(403) }
    guard let owner = session?.user.id, let store else { throw AppError.storage }
    let input = AttemptInput(
      clientId: UUID().uuidString, exerciseId: id, answer: answer,
      performedAt: ISO8601DateFormatter().string(from: Date()))
    try store.save(LocalAnswer(input: input), owner: owner, kind: "answer", id: input.clientId)
    try reload()
    Task { await sync() }
  }
  func queueCopy(_ id: String, text: String, image: Data?) throws {
    guard allows("tutor") else { throw AppError.status(403) }
    guard let owner = session?.user.id, let store else { throw AppError.storage }
    guard image != nil || !text.isEmpty else { throw AppError.invalidResponse }
    let draft = CopyDraft(id: UUID().uuidString, exerciseId: id, text: text, hasImage: image != nil)
    try store.save(draft, owner: owner, kind: "copy", id: draft.id, image: image)
    try reload()
    Task { await sync() }
  }
  func feature(_ key: String) -> Capability? {
    guard let access, var feature = access.features.first(where: { $0.key == key }) else {
      return nil
    }
    feature.allowed = FeatureAccess.eligible(
      feature, access: access, at: Date(), administrator: session?.user.role == "admin")
    return feature
  }
  func refreshOnExpiry() async {
    guard let access else { return }
    let dates = ([access.validUntil].compactMap { $0 } + (access.providers ?? []).map(\.expiresAt))
      .compactMap(Practice.date).filter { $0 > Date() }.sorted()
    for deadline in dates {
      do { try await Task.sleep(for: .seconds(max(0, deadline.timeIntervalSinceNow) + 0.05)) } catch
      { return }
      objectWillChange.send()
      await sync()
    }
  }
  func allows(_ key: String) -> Bool { feature(key)?.allowed == true }
  func refreshAccess() async throws -> MemberAccess {
    guard let member = session, let store else { throw AppError.status(401) }
    let fresh = try await MemberAPI(member.token).read(APIPath.getMemberAccess, MemberAccess.self)
    guard session == member else { throw AppError.status(401) }
    try store.save(fresh, owner: member.user.id, kind: "access", id: "current")
    if fresh.features.first(where: { $0.key == "course" })?.allowed != true {
      let folder = try exports(owner: member.user.id)
      try? FileManager.default.removeItem(at: folder)
      try store.remove(owner: member.user.id, kind: "snapshot")
      snapshot = nil
    }
    if fresh.features.first(where: { $0.key == "mock" })?.allowed != true {
      try store.remove(owner: member.user.id, kind: "mock")
    }
    access = fresh
    return fresh
  }
  func refreshCopies() async {
    guard allows("tutor"), let member = session, !syncing, let store else { return }
    do {
      let response = try await MemberAPI(member.token).read(
        APIPath.getSubmissions, Submissions.self)
      try Task.checkCancellation()
      guard !syncing, session == member, var saved = snapshot else { return }
      saved.submissions = response.items
      try store.save(saved, owner: member.user.id, kind: "snapshot", id: "current")
      snapshot = saved
      trackingNotice = ""
    } catch {
      guard !Task.isCancelled, session == member else { return }
      if case AppError.status(401) = error {
        await sync()
        return
      }
      trackingNotice = "Actualisation indisponible. Le dernier état reçu reste affiché."
    }
  }
  func sync() async {
    guard let member = session, let store else { return }
    if syncing {
      syncAgain = true
      return
    }
    syncing = true
    defer { syncing = false }
    do {
      let api = try MemberAPI(member.token)
      let owner = member.user.id
      repeat {
        syncAgain = false
        _ = try await refreshAccess()
        for var row in answers
        where allows("practice") && row.receipt == nil && row.rejection == nil {
          do {
            let data = try await api.raw(
              APIPath.gradeAttempt, method: "POST", body: JSONEncoder().encode(row.input))
            let receipt = try JSONDecoder().decode(Attempt.self, from: data)
            guard receipt.clientId.lowercased() == row.input.clientId.lowercased(),
              receipt.exerciseId == row.input.exerciseId
            else { throw AppError.invalidResponse }
            row.receipt = receipt
          } catch AppError.status(let status) where [400, 404, 409, 422].contains(status) {
            row.rejection = status
          }
          guard session == member else { return }
          try store.save(row, owner: owner, kind: "answer", id: row.input.clientId)
        }
        for var copy in copies where allows("tutor") && copy.rejection == nil {
          do {
            let image = try store.image(owner: owner, id: copy.id)
            if copy.hasImage && image == nil { throw AppError.storage }
            _ = try await api.submit(copy, image: image)
            guard session == member else { return }
            try store.remove(owner: owner, kind: "copy", id: copy.id)
          } catch AppError.status(let status) where [400, 404, 409, 413, 415, 422].contains(status)
          {
            guard session == member else { return }
            copy.rejection = status
            try store.save(copy, owner: owner, kind: "copy", id: copy.id)
          }
        }
        let catalog =
          allows("course") ? try await api.read(APIPath.listCatalog, Catalog.self).items : []
        let daily =
          allows("practice")
          ? try await api.read(APIPath.listPractice + "?mode=daily", Catalog.self).items : []
        let progress =
          allows("practice") ? try await api.read(APIPath.getProgress, Progress.self) : nil
        let submissions =
          allows("tutor") ? try await api.read(APIPath.getSubmissions, Submissions.self).items : []
        let value = Snapshot(
          catalog: catalog.filter { $0.status == "validated" }, dailyIDs: daily.map(\.id),
          progress: progress, submissions: submissions, updated: Date())
        guard session == member else { return }
        try store.save(value, owner: owner, kind: "snapshot", id: "current")
        try reload()
        notice = ""
      } while syncAgain && session == member
    } catch {
      guard session == member else { return }
      switch error {
      case AppError.status(401):
        try? Secrets.clear()
        session = nil
        try? reload()
        notice = "Votre session a expiré. Reconnectez-vous pour synchroniser vos réponses."
      case AppError.status(403):
        try? store.remove(owner: member.user.id, kind: "snapshot")
        try? reload()
        notice = message(error)
      default:
        try? reload()
        notice =
          error is URLError
          ? "Connexion indisponible. Votre travail est conservé sur cet appareil." : message(error)
      }
    }
  }
  func exit(delete: Bool) async {
    guard let member = session else { return }
    do {
      let api = try MemberAPI(member.token)
      if delete {
        _ = try await api.raw(APIPath.deleteMember, method: "DELETE")
      } else {
        _ = try? await api.raw(APIPath.logout, method: "POST")
      }
      try Secrets.clear()
      session = nil
      try store?.remove(owner: member.user.id)
      try reload()
      let folder = try exports(owner: member.user.id)
      try? FileManager.default.removeItem(at: folder)
      await setReminder(nil)
      notice = ""
    } catch { notice = message(error) }
  }
  func exports(owner: String) throws -> URL {
    let path = FileManager.default.temporaryDirectory.appendingPathComponent(
      owner, isDirectory: true)
    try FileManager.default.createDirectory(
      at: path, withIntermediateDirectories: true,
      attributes: [.protectionKey: FileProtectionType.complete])
    return path
  }
  func file(_ document: Document? = nil) async throws -> URL {
    guard document == nil || allows("course") else { throw AppError.status(403) }
    guard let member = session else { throw AppError.status(401) }
    let folder = try exports(owner: member.user.id)
    let file = folder.appendingPathComponent(
      document.map { $0.id + ".pdf" } ?? "mes-donnees-crpe.json")
    if document != nil && FileManager.default.fileExists(atPath: file.path) { return file }
    let path =
      document.map { APIPath.downloadDocument.replacingOccurrences(of: "{id}", with: $0.id) }
      ?? APIPath.exportMember
    let bytes = try await MemberAPI(member.token).raw(path)
    guard session == member else { throw AppError.status(401) }
    try bytes.write(to: file, options: [.atomic, .completeFileProtection])
    return file
  }
  func loadReminder() async {
    let requests = await UNUserNotificationCenter.current().pendingNotificationRequests()
    reminder =
      (requests.first { $0.identifier == "practice-daily" }?.trigger
      as? UNCalendarNotificationTrigger)?.dateComponents.hour
  }
  func setReminder(_ hour: Int?) async {
    let center = UNUserNotificationCenter.current()
    do {
      if let hour {
        guard try await center.requestAuthorization(options: [.alert, .sound]) else {
          throw AppError.permissions
        }
        let content = UNMutableNotificationContent()
        content.title = "Vos cinq minutes de maths"
        content.body = "Votre entraînement CRPE vous attend."
        let trigger = UNCalendarNotificationTrigger(
          dateMatching: DateComponents(hour: hour, minute: 0), repeats: true)
        try await center.add(
          UNNotificationRequest(identifier: "practice-daily", content: content, trigger: trigger))
      } else {
        center.removePendingNotificationRequests(withIdentifiers: ["practice-daily"])
      }
      reminder = hour
    } catch { notice = message(error) }
  }
  func message(_ error: Error) -> String {
    (error as? AppError)?.localizedDescription
      ?? "Le service est inaccessible. Vérifiez votre connexion puis réessayez."
  }
}
