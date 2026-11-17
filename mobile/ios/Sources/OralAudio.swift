import AVFoundation
import Foundation
import SwiftUI

enum OralAudioError: LocalizedError {
  case microphone, recording, size, invalidSession
  var errorDescription: String? {
    switch self {
    case .microphone:
      "Autorisez le microphone dans Réglages pour enregistrer, ou saisissez votre texte."
    case .recording:
      "Impossible de lire cet enregistrement. Vous pouvez le refaire ou saisir votre texte."
    case .size:
      "L’enregistrement doit faire moins de 20 Mio. Transcrivez votre exposé en plusieurs parties."
    case .invalidSession: "Cette séance orale est indisponible."
    }
  }
}

enum OralClip {
  static let limit = 20 * 1024 * 1024
  static func url(folder: URL, session: String, slot: Int) throws -> URL {
    guard let id = UUID(uuidString: session), (-1...2).contains(slot) else {
      throw OralAudioError.invalidSession
    }
    return folder.appendingPathComponent("oral-\(id.uuidString)-\(slot).m4a")
  }
  static func multipart(_ data: Data, boundary: String) throws -> Data {
    guard data.count > 12, data.count <= limit else { throw OralAudioError.size }
    guard data[4..<8].elementsEqual("ftyp".utf8) else { throw OralAudioError.recording }
    var body = Data(
      "--\(boundary)\r\nContent-Disposition: form-data; name=\"audio\"; filename=\"oral.m4a\"\r\nContent-Type: audio/mp4\r\n\r\n"
        .utf8)
    body.append(data)
    body.append(contentsOf: "\r\n--\(boundary)--\r\n".utf8)
    return body
  }
}
extension MemberAPI {
  func transcribe(_ id: String, file: URL) async throws -> String {
    let length = try file.resourceValues(forKeys: [.fileSizeKey]).fileSize ?? 0
    guard length > 0, length <= OralClip.limit else { throw OralAudioError.size }
    let boundary = UUID().uuidString
    let body = try OralClip.multipart(
      Data(contentsOf: file, options: .mappedIfSafe), boundary: boundary)
    let data = try await raw(
      APIPath.transcribeOral.replacingOccurrences(of: "{id}", with: id), method: "POST", body: body,
      type: "multipart/form-data; boundary=\(boundary)", voice: true)
    let text = try JSONDecoder().decode(Transcript.self, from: data).text.trimmingCharacters(
      in: .whitespacesAndNewlines)
    guard !text.isEmpty else { throw OralAudioError.recording }
    return text
  }
  func questionAudio(_ id: String, index: Int) async throws -> Data {
    try await raw(
      APIPath.speakQuestion.replacingOccurrences(of: "{id}", with: id), method: "POST",
      body: JSONEncoder().encode(SpeechInput(index: index)), voice: true)
  }
}

@MainActor
final class OralAudio: NSObject, ObservableObject, AVAudioRecorderDelegate, AVAudioPlayerDelegate {
  @Published private(set) var slot: Int?
  @Published private(set) var began: Date?
  @Published private(set) var message = ""
  private var recorder: AVAudioRecorder?
  private var player: AVAudioPlayer?

  func start(file: URL, slot: Int, seconds: TimeInterval) async throws {
    guard await AVAudioApplication.requestRecordPermission() else {
      throw OralAudioError.microphone
    }
    try Task.checkCancellation()
    stop()
    let audio = AVAudioSession.sharedInstance()
    do {
      try audio.setCategory(.playAndRecord, mode: .default, options: .defaultToSpeaker)
      try audio.setActive(true)
      let settings: [String: Any] = [
        AVFormatIDKey: kAudioFormatMPEG4AAC, AVSampleRateKey: 24000,
        AVNumberOfChannelsKey: 1, AVEncoderBitRateKey: 48000,
      ]
      let next = try AVAudioRecorder(url: file, settings: settings)
      next.delegate = self
      // A clip at 48 kb/s stays under 20 MiB for at least 45 minutes.
      guard next.record(forDuration: max(1, min(seconds, 2700))) else {
        throw OralAudioError.recording
      }
      recorder = next
      self.slot = slot
      began = Date()
      message = ""
    } catch {
      deactivate()
      throw error
    }
  }
  func play(_ bytes: Data) throws {
    stop()
    let audio = AVAudioSession.sharedInstance()
    do {
      try audio.setCategory(.playback, mode: .spokenAudio)
      try audio.setActive(true)
      let next = try AVAudioPlayer(data: bytes)
      next.delegate = self
      guard next.play() else { throw OralAudioError.recording }
      player = next
    } catch {
      deactivate()
      throw error
    }
  }
  private func deactivate() {
    try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
  }
  func stop() {
    guard recorder != nil || player != nil else { return }
    recorder?.delegate = nil
    recorder?.stop()
    player?.stop()
    recorder = nil
    player = nil
    slot = nil
    began = nil
    deactivate()
  }
  nonisolated func audioRecorderDidFinishRecording(
    _ recorder: AVAudioRecorder, successfully flag: Bool
  ) {
    Task { @MainActor in
      guard self.recorder === recorder else { return }
      stop()
      message =
        flag
        ? "Enregistrement terminé. Vous pouvez le transcrire."
        : "L’enregistrement a été interrompu."
    }
  }
  nonisolated func audioPlayerDidFinishPlaying(_ player: AVAudioPlayer, successfully flag: Bool) {
    Task { @MainActor in if self.player === player { stop() } }
  }
}
