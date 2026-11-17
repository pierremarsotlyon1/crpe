import Foundation
import Security

enum Secrets {
  static let query: [String: Any] = [
    kSecClass as String: kSecClassGenericPassword,
    kSecAttrService as String: "fr.prepamathscrpe.session", kSecAttrAccount as String: "member",
  ]
  static func restore() throws -> Session? {
    var request = query
    request[kSecReturnData as String] = true
    var raw: CFTypeRef?
    let status = SecItemCopyMatching(request as CFDictionary, &raw)
    if status == errSecItemNotFound { return nil }
    guard status == errSecSuccess, let data = raw as? Data else { throw AppError.storage }
    return try JSONDecoder().decode(Session.self, from: data)
  }
  static func save(_ session: Session) throws {
    let values: [String: Any] = [
      kSecValueData as String: try JSONEncoder().encode(session),
      kSecAttrAccessible as String: kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
    ]
    let status = SecItemUpdate(query as CFDictionary, values as CFDictionary)
    if status == errSecSuccess { return }
    guard status == errSecItemNotFound,
      SecItemAdd(query.merging(values) { _, b in b } as CFDictionary, nil) == errSecSuccess
    else { throw AppError.storage }
  }
  static func clear() throws {
    let status = SecItemDelete(query as CFDictionary)
    guard status == errSecSuccess || status == errSecItemNotFound else { throw AppError.storage }
  }
}
private final class NoRedirects: NSObject, URLSessionTaskDelegate {
  func urlSession(
    _ session: URLSession, task: URLSessionTask,
    willPerformHTTPRedirection response: HTTPURLResponse, newRequest request: URLRequest,
    completionHandler: @escaping (URLRequest?) -> Void
  ) {
    completionHandler(nil)
  }
}
struct MemberAPI {
  let origin: URL
  let token: String?
  init(_ token: String? = nil) throws {
    guard let raw = Bundle.main.object(forInfoDictionaryKey: "CRPEAPIURL") as? String,
      let url = URL(string: raw), url.host != nil, url.user == nil, url.password == nil,
      url.query == nil, url.fragment == nil, ["", "/"].contains(url.path)
    else { throw AppError.configuration }
    #if DEBUG
      guard ["http", "https"].contains(url.scheme) else { throw AppError.configuration }
    #else
      guard url.scheme == "https" else { throw AppError.configuration }
    #endif
    self.origin = url
    self.token = token
  }
  private static func network(requestTimeout: TimeInterval, resourceTimeout: TimeInterval)
    -> URLSession
  {
    let config = URLSessionConfiguration.ephemeral
    config.timeoutIntervalForRequest = requestTimeout
    config.timeoutIntervalForResource = resourceTimeout
    config.httpShouldSetCookies = false
    config.urlCache = nil
    return URLSession(configuration: config, delegate: NoRedirects(), delegateQueue: nil)
  }
  static let network = network(requestTimeout: 18, resourceTimeout: 50)
  private static let voiceNetwork = network(requestTimeout: 75, resourceTimeout: 100)
  func raw(
    _ path: String, method: String = "GET", body: Data? = nil, type: String = "application/json",
    voice: Bool = false
  ) async throws -> Data {
    guard let url = URL(string: path, relativeTo: origin)?.absoluteURL, url.host == origin.host,
      url.scheme == origin.scheme, url.port == origin.port
    else { throw AppError.configuration }
    var request = URLRequest(url: url)
    request.httpMethod = method
    request.httpBody = body
    if body != nil { request.setValue(type, forHTTPHeaderField: "Content-Type") }
    if let token { request.setValue("Bearer " + token, forHTTPHeaderField: "Authorization") }
    let (data, response) = try await (voice ? Self.voiceNetwork : Self.network).data(for: request)
    guard let response = response as? HTTPURLResponse else { throw AppError.invalidResponse }
    guard (200..<300).contains(response.statusCode) else {
      throw AppError.status(response.statusCode)
    }
    return data
  }
  func read<T: Decodable>(_ path: String, _ type: T.Type) async throws -> T {
    try JSONDecoder().decode(type, from: await raw(path))
  }
  func submit(_ copy: CopyDraft, image: Data?) async throws -> Submission {
    let boundary = "crpe-" + UUID().uuidString
    var payload = Data()
    func line(_ text: String) {
      payload.append(contentsOf: text.utf8)
      payload.append(contentsOf: [13, 10])
    }
    for (name, value) in [
      ("clientId", copy.id), ("exerciseId", copy.exerciseId), ("text", copy.text),
    ] where !value.isEmpty {
      line("--" + boundary)
      line("Content-Disposition: form-data; name=\"" + name + "\"")
      line("")
      line(value)
    }
    if let image {
      line("--" + boundary)
      line("Content-Disposition: form-data; name=\"image\"; filename=\"copie.jpg\"")
      line("Content-Type: image/jpeg")
      line("")
      payload.append(image)
      line("")
    }
    line("--" + boundary + "--")
    return try JSONDecoder().decode(
      Submission.self,
      from: await raw(
        APIPath.submitWork, method: "POST", body: payload,
        type: "multipart/form-data; boundary=" + boundary))
  }
}
