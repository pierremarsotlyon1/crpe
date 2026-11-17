import SwiftUI

struct SavedMock: Codable {
  var session: MockSession
  var answers: [String: String]
}
struct MockScreen: View {
  @EnvironmentObject var model: MemberModel
  @State private var draft: SavedMock?
  @State private var notice = ""
  @State private var busy = false
  @State private var confirm = false
  @State private var history: [MockReview] = []
  @State private var autosave: Task<Void, Never>?
  @State private var clock = Date()

  private func persist(_ value: SavedMock) throws {
    try model.persistPrivate(value, kind: "mock", id: value.session.id)
  }
  private func historyList() async {
    do {
      history = try await model.authorizedAPI().read(APIPath.listCompletedMocks, MockReviews.self)
        .items
    } catch { if !Task.isCancelled { notice = model.message(error) } }
  }
  private func open(_ id: String, history: Bool = false) async {
    guard !busy, let owner = model.session?.user.id, model.allows("mock") else { return }
    autosave?.cancel()
    busy = true
    notice = ""
    defer { busy = false }
    do {
      let cached = try model.privateRows(SavedMock.self, kind: "mock").first {
        $0.session.subjectId == id && $0.session.status == "active" && !history
          || history && $0.session.id == id
      }
      draft = cached
      let api = try model.authorizedAPI()
      let next: MockSession
      if let existing = history ? id : cached?.session.id {
        next = try await api.read(
          APIPath.getMock.replacingOccurrences(of: "{id}", with: existing),
          MockSession.self)
      } else {
        let bytes = try await api.raw(
          APIPath.startMock, method: "POST", body: JSONEncoder().encode(MockStart(subjectId: id)))
        next = try JSONDecoder().decode(MockSession.self, from: bytes)
      }
      guard model.session?.user.id == owner, model.allows("mock") else { return }
      let local =
        next.status == "active"
        ? (cached?.answers ?? [:]).filter { cached?.session.answers[$0.key] != $0.value } : [:]
      let value = SavedMock(session: next, answers: next.answers.merging(local) { _, new in new })
      try persist(value)
      draft = value
      if !local.isEmpty { await transmit(finish: false) }
    } catch { notice = model.message(error) }
  }
  private func edit(_ id: String, _ answer: String) {
    guard var value = draft, value.session.status == "active", !busy,
      answer.utf8.count
        <= (value.session.items.first { $0.id == id }?.format == "text" ? 48000 : 4096)
    else { return }
    value.answers[id] = answer
    do {
      try persist(value)
      draft = value
      notice = "Réponse enregistrée sur cet appareil."
    } catch { notice = model.message(error) }
    autosave?.cancel()
    autosave = Task {
      do { try await Task.sleep(for: .seconds(1)) } catch { return }
      await transmit(finish: false)
    }
  }
  private func transmit(finish: Bool) async {
    guard let value = draft, let owner = model.session?.user.id, model.allows("mock"),
      value.session.status == "active"
    else { return }
    if finish {
      autosave?.cancel()
      busy = true
    }
    defer { if finish { busy = false } }
    do {
      let api = try model.authorizedAPI()
      let path = (finish ? APIPath.finishMock : APIPath.saveMockAnswers).replacingOccurrences(
        of: "{id}", with: value.session.id)
      let payload = MockFinish(
        answers: value.answers.map { MockAnswer(exerciseId: $0.key, answer: $0.value) })
      let bytes = try await api.raw(
        path, method: finish ? "POST" : "PUT", body: JSONEncoder().encode(payload))
      try Task.checkCancellation()
      let received = try JSONDecoder().decode(MockSession.self, from: bytes)
      guard draft?.session.id == received.id, model.session?.user.id == owner, model.allows("mock")
      else { return }
      let next = SavedMock(
        session: received,
        answers: received.status == "active" ? draft?.answers ?? received.answers : received.answers
      )
      try persist(next)
      draft = next
      notice = received.status == "active" ? "Réponses synchronisées." : ""
    } catch { if !Task.isCancelled { notice = model.message(error) } }
  }
  var body: some View {
    List {
      if let value = draft {
        if value.session.status == "active" {
          let deadline = Practice.date(value.session.deadline) ?? clock
          let seconds = max(0, Int(deadline.timeIntervalSince(clock)))
          Section("Temps restant") {
            Text(String(format: "%02d min %02d s", seconds / 60, seconds % 60)).monospacedDigit()
          }
          ForEach(value.session.items, id: \.id) { item in
            Section(item.title) {
              FormulaText(value: item.body)
              if item.format == "choice" {
                ForEach(item.choices, id: \.self) { choice in
                  Button {
                    edit(item.id, choice)
                  } label: {
                    HStack {
                      FormulaText(value: choice)
                      if value.answers[item.id] == choice { Image(systemName: "checkmark") }
                    }
                  }.disabled(seconds == 0 || busy)
                }
              } else {
                TextField(
                  item.format == "text" ? "Votre raisonnement" : "Votre réponse",
                  text: Binding(get: { draft?.answers[item.id] ?? "" }, set: { edit(item.id, $0) }),
                  axis: .vertical
                ).accessibilityIdentifier("mock-answer-" + item.format).accessibilityLabel(
                  item.format == "text" ? "Votre raisonnement" : "Votre réponse"
                ).disabled(seconds == 0 || busy)
              }
            }
          }
          Button("Enregistrer mes réponses") { Task { await transmit(finish: false) } }.disabled(
            busy || seconds == 0)
          Button(seconds == 0 ? "Obtenir mon résultat" : "Rendre ma copie") { confirm = true }
            .disabled(busy)
        } else {
          Section(
            value.session.gradingStatus == "pending_review" ? "Résultat provisoire" : "Résultat"
          ) {
            Text(
              "\(value.session.score.formatted()) / \(value.session.maxScore.formatted()) points"
            ).bold()
            ForEach(value.session.results, id: \.exerciseId) { result in
              VStack(alignment: .leading) {
                Text(value.session.items.first { $0.id == result.exerciseId }?.title ?? "Question")
                  .bold()
                if let points = result.points {
                  Text("\(points.formatted()) / \(result.maxPoints.formatted())")
                } else {
                  Text("Réponse écrite en attente de relecture")
                }
                if let teacher = result.teacherFeedback { FormulaText(value: teacher) }
                FormulaText(value: result.feedback)
              }
            }
            if let feedback = value.session.teacherFeedback, !feedback.isEmpty {
              FormulaText(value: feedback)
            }
            if model.allows("mock_video"), let raw = value.session.reviewVideoUrl,
              let url = URL(string: raw),
              url.scheme == "https"
            {
              Link("Voir le retour vidéo", destination: url)
            }
          }
        }
        Button("Revenir aux sujets") {
          autosave?.cancel()
          draft = nil
          Task { await historyList() }
        }
      } else {
        let subjects = (model.snapshot?.catalog ?? []).filter {
          $0.kind == "mock" && $0.status == "validated"
        }
        if subjects.isEmpty { Text("Aucun sujet blanc validé n’est disponible.") }
        ForEach(subjects, id: \.id) { item in
          Section(item.title) {
            FormulaText(value: item.body)
            Text("Durée · \(item.durationMinutes) min")
            Button("Ouvrir le sujet") { Task { await open(item.id) } }.accessibilityIdentifier(
              "mock-start-" + item.id
            ).disabled(busy)
          }
        }
        if !history.isEmpty {
          Section("Mes résultats") {
            ForEach(history, id: \.id) { entry in
              Button(
                entry.title + " · "
                  + (Practice.date(entry.startedAt)?.formatted(date: .abbreviated, time: .shortened)
                    ?? "")
              ) {
                Task { await open(entry.id, history: true) }
              }.disabled(busy)
            }
          }
        }
      }
    }.safeAreaInset(edge: .top) {
      if !notice.isEmpty {
        Text(notice).font(.footnote).frame(maxWidth: .infinity, alignment: .leading)
          .padding(.horizontal).padding(.vertical, 8).background(.regularMaterial)
      }
    }.navigationTitle("Sujets blancs").task { await historyList() }.task {
      while !Task.isCancelled {
        clock = Date()
        do { try await Task.sleep(for: .seconds(1)) } catch { return }
      }
    }
    .confirmationDialog(
      "Rendre définitivement ma copie ?", isPresented: $confirm, titleVisibility: .visible
    ) {
      Button("Rendre ma copie") { Task { await transmit(finish: true) } }
      Button("Annuler", role: .cancel) {}
    }
  }
}
