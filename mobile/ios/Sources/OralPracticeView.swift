import SwiftUI

struct OralDraft: Codable {
  var id: String
  var transcript: String
  var answers: [String]
  var speakingSince: Date?
  init(_ item: OralSession) {
    id = item.id
    transcript = item.transcript
    answers = item.questions.indices.map { $0 < item.answers.count ? item.answers[$0] : "" }
  }
  func value(_ slot: Int) -> String {
    slot == -1 ? transcript : (answers.indices.contains(slot) ? answers[slot] : "")
  }
  mutating func edit(_ value: String, slot: Int) -> Bool {
    guard value.utf8.count <= (slot == -1 ? 120000 : 16000) else { return false }
    if slot == -1 {
      transcript = value
    } else if answers.indices.contains(slot) {
      answers[slot] = value
    } else {
      return false
    }
    return true
  }
  var complete: Bool {
    ([transcript] + answers).allSatisfy {
      !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }
      && (2...3).contains(answers.count)
  }
}

struct OralPracticeView: View {
  @EnvironmentObject var model: MemberModel
  @Environment(\.scenePhase) private var phase
  @StateObject private var audio = OralAudio()
  @State private var items: [OralSession] = []
  @State private var current: OralSession?
  @State private var draft: OralDraft?
  @State private var status = ""
  @State private var busy = false
  @State private var confirm = false
  @State private var work: Task<Void, Never>?
  @State private var clock = Date()

  private func report(_ error: Error, voice: Bool) -> String {
    if case AppError.status(503) = error, voice {
      return
        "Le service vocal est indisponible. Vous pouvez saisir votre texte et lire les questions."
    }
    if case AppError.status(404) = error {
      return "Aucun sujet oral validé disponible pour cette demande."
    }
    if case AppError.status(413) = error { return OralAudioError.size.localizedDescription }
    if let audio = error as? OralAudioError { return audio.localizedDescription }
    return model.message(error)
  }
  private func run(
    voice: Bool = false, _ action: @escaping @MainActor (MemberAPI, String) async throws -> Void
  ) {
    guard !busy, let member = model.session, let token = member.token else { return }
    status = ""
    busy = true
    work = Task { @MainActor in
      defer { busy = false }
      do { try await action(MemberAPI(token), member.user.id) } catch {
        if !Task.isCancelled, model.session?.user.id == member.user.id {
          status = report(error, voice: voice)
        }
      }
    }
  }
  private func matches(_ owner: String) throws {
    try Task.checkCancellation()
    guard model.session?.user.id == owner, model.allows("oral") else { throw AppError.status(403) }
  }
  private func open(_ item: OralSession) throws {
    audio.stop()
    let cached = try model.privateRows(OralDraft.self, kind: "oral-draft").first {
      $0.id == item.id
    }
    current = item
    draft = item.status == "active" ? cached ?? OralDraft(item) : OralDraft(item)
  }
  private func refresh() {
    run { api, owner in
      let list = try await api.read(APIPath.listOral, OralSessions.self)
      try matches(owner)
      items = list.items
      if let id = current?.id, let next = items.first(where: { $0.id == id }) { try open(next) }
    }
  }
  private func start() {
    run { api, owner in
      let data = try await api.raw(
        APIPath.startOral, method: "POST", body: JSONEncoder().encode(OralStart()))
      try matches(owner)
      let item = try JSONDecoder().decode(OralSession.self, from: data)
      items.insert(item, at: 0)
      try open(item)
    }
  }
  private func save(_ value: OralDraft) throws {
    try model.persistPrivate(value, kind: "oral-draft", id: value.id)
    draft = value
  }
  private func edit(_ value: String, _ slot: Int) {
    guard var next = draft else { return }
    guard next.edit(value, slot: slot) else {
      status = "Le texte dépasse la longueur acceptée."
      return
    }
    do { try save(next) } catch { status = model.message(error) }
  }
  private func clip(_ owner: String, _ item: OralSession, _ slot: Int) throws -> URL {
    try OralClip.url(folder: model.exports(owner: owner), session: item.id, slot: slot)
  }
  private func record(_ item: OralSession, _ slot: Int) {
    run { _, owner in
      try matches(owner)
      let file = try clip(owner, item, slot)
      var next = draft ?? OralDraft(item)
      let seconds = Double(item.durationMinutes * 60)
      let remaining =
        slot == -1
        ? (next.speakingSince ?? Date()).addingTimeInterval(seconds).timeIntervalSinceNow : seconds
      try await audio.start(
        file: file, slot: slot, seconds: max(1, remaining > 0 ? remaining : seconds))
      do {
        try matches(owner)
        if slot == -1 && next.speakingSince == nil {
          next.speakingSince = Date()
          try save(next)
        }
      } catch {
        audio.stop()
        throw error
      }
    }
  }
  private func transcribe(_ item: OralSession, _ slot: Int) {
    run(voice: true) { api, owner in
      let file = try clip(owner, item, slot)
      let text = try await api.transcribe(item.id, file: file)
      try matches(owner)
      guard var next = draft, next.id == item.id else { return }
      let old = next.value(slot)
      guard next.edit(old.isEmpty ? text : old + "\n" + text, slot: slot) else {
        throw AppError.status(422)
      }
      try save(next)
      try FileManager.default.removeItem(at: file)
      status = "Transcription ajoutée. Relisez-la avant d’envoyer."
    }
  }
  private func finish() {
    guard let item = current, let value = draft, value.complete else { return }
    run { api, owner in
      let data = try await api.raw(
        APIPath.finishOral.replacingOccurrences(of: "{id}", with: item.id), method: "POST",
        body: JSONEncoder().encode(OralFinish(transcript: value.transcript, answers: value.answers))
      )
      try matches(owner)
      let result = try JSONDecoder().decode(OralSession.self, from: data)
      items = items.map { $0.id == result.id ? result : $0 }
      try open(result)
      for slot in -1..<item.questions.count {
        if let file = try? clip(owner, item, slot) { try? FileManager.default.removeItem(at: file) }
      }
    }
  }
  private func listen(_ item: OralSession, _ index: Int) {
    run(voice: true) { api, owner in
      let data = try await api.questionAudio(item.id, index: index)
      try matches(owner)
      try audio.play(data)
    }
  }
  private func timeLeft(_ deadline: Date) -> String {
    let seconds = max(0, Int(ceil(deadline.timeIntervalSince(clock))))
    return String(format: "%02d:%02d", seconds / 60, seconds % 60)
  }
  private func label(_ session: OralSession) -> String {
    switch session.status {
    case "active": "En cours"
    case "reviewed": "Évaluée"
    default: "En attente d’évaluation"
    }
  }
  @ViewBuilder private func editor(_ item: OralSession, _ slot: Int) -> some View {
    let active = audio.slot == slot
    let file = model.session.flatMap { try? clip($0.user.id, item, slot) }
    let saved = file.map { FileManager.default.fileExists(atPath: $0.path) } ?? false
    if active {
      Button("Arrêter l’enregistrement", systemImage: "stop.circle.fill") { audio.stop() }
        .accessibilityIdentifier("oral-stop")
    } else if saved {
      Button("Transcrire l’enregistrement", systemImage: "text.bubble") { transcribe(item, slot) }
      Button("Effacer l’enregistrement", role: .destructive) {
        if let file {
          do {
            try FileManager.default.removeItem(at: file)
            status = "Enregistrement effacé."
          } catch { status = model.message(error) }
        }
      }
    } else {
      Button(slot == -1 ? "Enregistrer mon exposé" : "Enregistrer ma réponse", systemImage: "mic") {
        record(item, slot)
      }
      .accessibilityIdentifier("oral-record-\(slot)")
    }
    TextField(
      slot == -1 ? "Texte de mon exposé" : "Texte de ma réponse",
      text: Binding(get: { draft?.value(slot) ?? "" }, set: { edit($0, slot) }), axis: .vertical
    )
    .lineLimit(2...8).disabled(busy).accessibilityIdentifier("oral-text-\(slot)")
  }
  @ViewBuilder private func overview() -> some View {
    Section {
      Button("Tirer un sujet", systemImage: "shuffle") { start() }.accessibilityIdentifier(
        "oral-start")
    }
    if !items.isEmpty {
      Section("Mes séances") {
        ForEach(items, id: \.id) { item in
          Button {
            do {
              try open(item)
              status = ""
            } catch { status = model.message(error) }
          } label: {
            VStack(alignment: .leading) {
              Text(item.title)
              Text(label(item)).font(.caption)
            }
          }
        }
      }
    }
    let criteria = Set(items.flatMap { $0.feedback.map(\.criterion) }).sorted()
    if !criteria.isEmpty {
      Section("Progression par critère") {
        ForEach(criteria, id: \.self) { key in
          let marks = items.reversed().compactMap { $0.feedback.first { $0.criterion == key } }
          VStack(alignment: .leading) {
            Text(key).bold()
            Text(
              marks.map { $0.status == "validated" ? "Validé" : "À travailler" }.joined(
                separator: " → ")
            )
            .font(.subheadline)
          }
        }
      }
    }
  }
  var body: some View {
    List {
      if !status.isEmpty { Text(status).accessibilityIdentifier("oral-status") }
      if !audio.message.isEmpty { Text(audio.message) }
      if busy { ProgressView() }
      if let item = current {
        Section(item.title) {
          FormulaText(value: item.body)
          Text(label(item))
        }
        Section("Critères de l’oral") {
          ForEach(item.criteria.indices, id: \.self) { Text(item.criteria[$0]) }
        }
        if item.status == "active" {
          Section("Préparation · \(item.prepMinutes) min") {
            if let start = Practice.date(item.createdAt) {
              Text(
                "Temps restant  "
                  + timeLeft(start.addingTimeInterval(Double(item.prepMinutes * 60)))
              ).monospacedDigit()
            }
          }
          Section("Exposé · \(item.durationMinutes) min") {
            if let since = draft?.speakingSince {
              Text(
                "Temps indicatif restant  "
                  + timeLeft(since.addingTimeInterval(Double(item.durationMinutes * 60)))
              ).monospacedDigit()
            } else {
              Button("Démarrer le chronomètre de l’exposé") {
                if var next = draft {
                  next.speakingSince = Date()
                  do { try save(next) } catch { status = model.message(error) }
                }
              }
            }
            editor(item, -1)
          }.disabled(busy || (audio.slot != nil && audio.slot != -1))
          ForEach(item.questions.indices, id: \.self) { i in
            Section("Question \(i + 1)") {
              FormulaText(value: item.questions[i])
              Button("Écouter la question", systemImage: "speaker.wave.2") { listen(item, i) }
                .disabled(audio.slot != nil)
              editor(item, i)
            }.disabled(busy || (audio.slot != nil && audio.slot != i))
          }
          Button("Envoyer mon oral") { confirm = true }.disabled(
            busy || audio.slot != nil || draft?.complete != true
          ).accessibilityIdentifier("oral-submit")
        } else {
          Section("Mon exposé") { Text(item.transcript) }
          ForEach(item.questions.indices, id: \.self) { i in
            Section(item.questions[i]) {
              Text(item.answers.indices.contains(i) ? item.answers[i] : "")
            }
          }
          if !item.feedback.isEmpty {
            Section(item.feedbackSource == "ai" ? "Évaluation par IA" : "Évaluation enseignante") {
              ForEach(item.feedback, id: \.criterion) { f in
                VStack(alignment: .leading) {
                  Text(f.criterion).bold()
                  Text(f.status == "validated" ? "Validé" : "À travailler")
                  Text(f.comment)
                }
              }
            }
          }
        }
        Button("Toutes mes séances") {
          audio.stop()
          current = nil
          draft = nil
          status = ""
        }.disabled(busy)
      } else {
        overview().disabled(busy)
      }
      Button("Actualiser les séances") { refresh() }.disabled(busy || audio.slot != nil)
    }
    .navigationTitle("Oral blanc")
    .task {
      refresh()
      while !Task.isCancelled {
        do { try await Task.sleep(for: .seconds(1)) } catch { break }
        clock = Date()
      }
    }
    .onChange(of: phase) { _, phase in
      if phase == .background {
        audio.stop()
        work?.cancel()
      }
    }
    .onDisappear {
      work?.cancel()
      audio.stop()
    }
    .confirmationDialog(
      "Transmettre l’exposé et les réponses pour évaluation ?", isPresented: $confirm
    ) { Button("Transmettre") { finish() } }
  }
}
