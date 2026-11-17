import Foundation
import SQLite3

enum AppError: Error, LocalizedError {
  case status(Int)
  case storage, invalidResponse, configuration, invalidPhoto, permissions
  var errorDescription: String? {
    switch self {
    case .status(400), .status(422): "Vérifiez les informations saisies."
    case .status(401): "Votre lien ou votre session a expiré. Demandez un nouveau lien."
    case .status(403): "Votre compte n’a pas accès à ce contenu."
    case .status(404): "Cet exercice n’est plus disponible."
    case .status(409): "La demande entre en conflit avec un envoi précédent."
    case .status(413), .invalidPhoto: "Choisissez une photo lisible de moins de 8 Mo."
    case .status(429): "La limite d’envois est atteinte. Réessayez plus tard."
    case .storage: "Impossible d’enregistrer les données sur cet appareil."
    case .configuration: "Le service de connexion n’est pas configuré pour cette version."
    case .permissions: "Autorisez les notifications dans les réglages pour recevoir votre rappel."
    default: "Le service est indisponible. Réessayez dans un instant."
    }
  }
}

struct Snapshot: Codable {
  var catalog: [Content]
  var dailyIDs: [String]?
  var progress: Progress?
  var submissions: [Submission]
  var updated: Date
}
struct LocalAnswer: Codable {
  var input: AttemptInput
  var receipt: Attempt?
  var rejection: Int?
}
struct CopyDraft: Codable {
  var id: String
  var exerciseId: String
  var text: String
  var hasImage: Bool
  var rejection: Int?
}
final class LearningStore {
  private let db: OpaquePointer
  private let transient = unsafeBitCast(-1, to: sqlite3_destructor_type.self)
  init(url: URL) throws {
    var ptr: OpaquePointer?
    guard
      sqlite3_open_v2(
        url.path, &ptr, SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE | SQLITE_OPEN_FULLMUTEX, nil)
        == SQLITE_OK, let ptr
    else {
      if let ptr { sqlite3_close(ptr) }
      throw AppError.storage
    }
    db = ptr
    guard
      sqlite3_exec(
        ptr,
        "PRAGMA secure_delete=ON; CREATE TABLE IF NOT EXISTS records(owner TEXT NOT NULL,kind TEXT NOT NULL,id TEXT NOT NULL,payload BLOB NOT NULL,image BLOB,PRIMARY KEY(owner,kind,id));",
        nil, nil, nil) == SQLITE_OK
    else { throw AppError.storage }
  }
  deinit { sqlite3_close(db) }
  private func statement<T>(_ sql: String, _ values: [String], _ work: (OpaquePointer) throws -> T)
    throws -> T
  {
    var raw: OpaquePointer?
    guard sqlite3_prepare_v2(db, sql, -1, &raw, nil) == SQLITE_OK, let raw else {
      throw AppError.storage
    }
    defer { sqlite3_finalize(raw) }
    for (i, value) in values.enumerated() {
      guard sqlite3_bind_text(raw, Int32(i + 1), value, -1, transient) == SQLITE_OK else {
        throw AppError.storage
      }
    }
    return try work(raw)
  }
  func save<T: Encodable>(_ value: T, owner: String, kind: String, id: String, image: Data? = nil)
    throws
  {
    let payload = try JSONEncoder().encode(value)
    try statement(
      "INSERT INTO records(owner,kind,id,payload,image) VALUES(?,?,?,?,?) ON CONFLICT(owner,kind,id) DO UPDATE SET payload=excluded.payload,image=coalesce(excluded.image,records.image)",
      [owner, kind, id]
    ) { stmt in
      let bind = { (data: Data, index: Int32) in
        data.withUnsafeBytes {
          sqlite3_bind_blob(stmt, index, $0.baseAddress, Int32(data.count), self.transient)
        }
      }
      guard bind(payload, 4) == SQLITE_OK else { throw AppError.storage }
      if let image, bind(image, 5) != SQLITE_OK { throw AppError.storage }
      guard sqlite3_step(stmt) == SQLITE_DONE else { throw AppError.storage }
    }
  }
  func rows<T: Decodable>(_ type: T.Type, owner: String, kind: String) throws -> [T] {
    try statement(
      "SELECT payload FROM records WHERE owner=? AND kind=? ORDER BY rowid", [owner, kind]
    ) { stmt in
      var rows: [T] = []
      while true {
        let result = sqlite3_step(stmt)
        if result == SQLITE_DONE { return rows }
        guard result == SQLITE_ROW, let bytes = sqlite3_column_blob(stmt, 0) else {
          throw AppError.storage
        }
        rows.append(
          try JSONDecoder().decode(
            T.self, from: Data(bytes: bytes, count: Int(sqlite3_column_bytes(stmt, 0)))))
      }
    }
  }
  func image(owner: String, id: String) throws -> Data? {
    try statement("SELECT image FROM records WHERE owner=? AND kind='copy' AND id=?", [owner, id]) {
      stmt in
      guard sqlite3_step(stmt) == SQLITE_ROW, let bytes = sqlite3_column_blob(stmt, 0) else {
        return nil
      }
      return Data(bytes: bytes, count: Int(sqlite3_column_bytes(stmt, 0)))
    }
  }
  func remove(owner: String, kind: String? = nil, id: String? = nil) throws {
    var query = "DELETE FROM records WHERE owner=?"
    var args = [owner]
    if let kind {
      query += " AND kind=?"
      args.append(kind)
    }
    if let id {
      query += " AND id=?"
      args.append(id)
    }
    try statement(query, args) { stmt in
      guard sqlite3_step(stmt) == SQLITE_DONE else { throw AppError.storage }
    }
  }
}

enum Practice {
  static func exercises(in catalog: [Content]) -> [Content] {
    let exams = Set(catalog.filter { $0.kind == "mock" }.map(\.id))
    return catalog.filter {
      $0.examId == "crpe" && $0.status == "validated" && $0.kind == "exercise"
        && !exams.contains($0.parentId ?? "")
    }
  }
  static func date(_ value: String) -> Date? {
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    if let date = formatter.date(from: value) { return date }
    formatter.formatOptions = [.withInternetDateTime]
    return formatter.date(from: value)
  }
  static func due(_ catalog: [Content], answers: [LocalAnswer], skills: [SkillProgress], now: Date)
    -> [Content]
  {
    var latest = [String: LocalAnswer]()
    for row in answers where row.rejection == nil {
      let prior = latest[row.input.exerciseId]?.input.performedAt ?? ""
      if prior <= (row.input.performedAt ?? "") { latest[row.input.exerciseId] = row }
    }
    var skillDates = [String: Date]()
    for skill in skills { skillDates[skill.skill] = date(skill.nextReviewAt) }
    return exercises(in: catalog).compactMap { item -> (Int, Date, Content)? in
      guard item.kind == "exercise", item.status == "validated",
        ["choice", "numeric"].contains(item.format)
      else { return nil }
      if let prior = latest[item.id] {
        guard let receipt = prior.receipt, let next = date(receipt.nextReviewAt), next <= now else {
          return nil
        }
        return (receipt.correct ? 1 : 0, next, item)
      }
      let next = skillDates[item.skill] ?? .distantPast
      return next <= now ? (skillDates[item.skill] == nil ? 2 : 1, next, item) : nil
    }.sorted { ($0.0, $0.1, $0.2.position, $0.2.id) < ($1.0, $1.1, $1.2.position, $1.2.id) }.map(
      \.2)
  }
  static func token(from url: URL) -> String? {
    guard url.scheme == "crpemaths", url.host == "auth", url.user == nil, url.password == nil,
      url.port == nil, ["", "/"].contains(url.path), let fragment = url.fragment
    else { return nil }
    var parts = URLComponents()
    parts.percentEncodedQuery = fragment
    guard let value = parts.queryItems?.first(where: { $0.name == "token" })?.value,
      value.range(of: "^[A-Za-z0-9_-]{16,256}$", options: .regularExpression) != nil
    else { return nil }
    return value
  }
}

enum CopyTracking {
  static func label(_ stage: String) -> String {
    switch stage {
    case "queued": "En attente de correction"
    case "processing": "Correction en cours"
    case "retrying": "Nouvelle tentative prévue"
    case "teacher_review": "En relecture par l’enseignante"
    case "completed": "Correction terminée"
    case "failed": "La correction a échoué"
    default: "État de correction indisponible"
    }
  }
  static func shouldPoll(visible: Bool, active: Bool, signedIn: Bool, stages: [String]) -> Bool {
    visible && active && signedIn
      && stages.contains { ["queued", "processing", "retrying"].contains($0) }
  }
}
