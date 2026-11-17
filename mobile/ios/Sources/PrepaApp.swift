import AVFoundation
import PhotosUI
import SwiftUI
import UIKit

@main struct PrepaApp: App {
  @StateObject private var model = MemberModel()
  @StateObject private var store = StorePurchases()
  @Environment(\.scenePhase) private var phase
  var body: some Scene {
    WindowGroup {
      Group {
        if model.session != nil { MemberTabs() } else { Login() }
      }.environmentObject(model).environmentObject(store).preferredColorScheme(.light).tint(
        Color(red: 0.09, green: 0.29, blue: 0.22)
      )
      .task(id: model.session?.user.id) {
        if let owner = model.session?.user.id { await store.watch(model, owner: owner) }
      }
      .task(id: model.access) { await model.refreshOnExpiry() }
      .onOpenURL { url in Task { await model.verify(url) } }
      .task(id: model.session?.user.id) {
        guard model.session != nil else { return }
        await model.loadReminder()
        await model.sync()
        while !Task.isCancelled {
          do { try await Task.sleep(for: .seconds(60)) } catch { return }
          if phase == .active
            && ((model.allows("practice")
              && model.answers.contains { $0.rejection == nil && $0.receipt == nil })
              || (model.allows("tutor") && model.copies.contains { $0.rejection == nil }))
          {
            await model.sync()
          }
        }
      }
      .onChange(of: phase) { _, value in if value == .active { Task { await model.sync() } } }
    }
  }
}
struct Login: View {
  @EnvironmentObject var model: MemberModel
  @State private var email = ""
  var body: some View {
    NavigationStack {
      ScrollView {
        VStack(alignment: .leading, spacing: 23) {
          Label("Prépa Maths CRPE", systemImage: "function").font(.headline)
          Text("Mon espace de révision").font(.system(.largeTitle, design: .serif)).bold()
          Text("Recevez un lien par e-mail pour retrouver votre parcours et vos exercices.")
          TextField("Adresse e-mail", text: $email).accessibilityIdentifier("login-email")
            .keyboardType(.emailAddress).textContentType(.emailAddress).textInputAutocapitalization(
              .never
            ).autocorrectionDisabled().textFieldStyle(.roundedBorder)
          Button("Recevoir mon lien") { Task { await model.request(email) } }.buttonStyle(
            .borderedProminent
          ).disabled(
            email.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || model.authenticating
          ).accessibilityIdentifier("request-login")
          if !model.notice.isEmpty { Text(model.notice).accessibilityIdentifier("status") }
        }.frame(maxWidth: 540, alignment: .leading).padding(28)
      }.background(Color(red: 0.97, green: 0.96, blue: 0.93))
    }
  }
}
struct MemberTabs: View {
  @EnvironmentObject var model: MemberModel
  @State private var tab = 0
  @State private var exercise = ""
  var body: some View {
    TabView(selection: $tab) {
      Group { if model.allows("practice") { Daily() } else { FeatureGate(key: "practice") } }
        .tabItem {
          Label(
            "Journée", systemImage: model.feature("practice")?.allowed == false ? "lock" : "sun.max"
          )
        }.tag(0)
      Group {
        if model.allows("course") {
          Course {
            exercise = $0
            tab = 2
          }
        } else {
          FeatureGate(key: "course")
        }
      }.tabItem {
        Label("Parcours", systemImage: model.feature("course")?.allowed == false ? "lock" : "book")
      }.tag(1)
      Group {
        if model.allows("tutor") {
          Copies(exercise: $exercise, isVisible: tab == 2)
        } else {
          FeatureGate(key: "tutor")
        }
      }.tabItem {
        Label("Copies", systemImage: model.feature("tutor")?.allowed == false ? "lock" : "doc.text")
      }.tag(2)
      Activities(jump: { key in tab = ["practice": 0, "course": 1, "tutor": 2][key] ?? 3 }).tabItem
      { Label("Activités", systemImage: "square.grid.2x2") }.tag(4)
      Account().tabItem { Label("Compte", systemImage: "person") }.tag(3)
    }
  }
}
struct Daily: View {
  @EnvironmentObject var model: MemberModel
  @State private var ids: [String] = []
  @State private var index = 0
  @State private var end: Date?
  @State private var value = ""
  @State private var saved = false
  @State private var count = 0
  @State private var now = Date()
  private var catalog: [Content] { model.snapshot?.catalog ?? [] }
  private var due: [Content] {
    if let snapshot = model.snapshot, Calendar.current.isDate(snapshot.updated, inSameDayAs: now),
      let ids = snapshot.dailyIDs
    {
      let pending = Set(
        model.answers.filter { $0.receipt == nil && $0.rejection == nil }.map(\.input.exerciseId))
      return ids.compactMap { id in pending.contains(id) ? nil : catalog.first { $0.id == id } }
    }
    return Array(
      Practice.due(
        catalog, answers: model.answers, skills: model.snapshot?.progress?.skills ?? [], now: now
      ).prefix(5))
  }
  private var question: Content? {
    ids.indices.contains(index) ? catalog.first { $0.id == ids[index] } : nil
  }
  var body: some View {
    NavigationStack {
      List {
        if !model.notice.isEmpty { Text(model.notice).font(.callout) }
        if let end {
          if let q = question, end > now {
            Section(
              "Séance · \(Int(ceil(end.timeIntervalSince(now)))/60) min \(Int(ceil(end.timeIntervalSince(now)))%60) s"
            ) {
              Text(q.title).font(.headline).accessibilityIdentifier("practice-title")
              FormulaText(value: q.body)
              if saved {
                if let receipt = model.answers.last(where: { $0.input.exerciseId == q.id })?.receipt
                {
                  Text(receipt.correct ? "Réponse juste." : "À retravailler.")
                  FormulaText(value: receipt.feedback)
                } else {
                  Text("Réponse enregistrée. Sa correction apparaîtra après synchronisation.")
                }
                Button("Continuer") {
                  index += 1
                  value = ""
                  saved = false
                }
              } else {
                if q.format == "choice" {
                  ForEach(q.choices, id: \.self) { choice in
                    Button {
                      value = choice
                    } label: {
                      HStack {
                        FormulaText(value: choice)
                        Spacer()
                        Image(systemName: choice == value ? "checkmark.circle.fill" : "circle")
                      }
                    }.accessibilityIdentifier("practice-choice")
                  }
                } else {
                  TextField("Réponse numérique", text: $value).textFieldStyle(.roundedBorder)
                    .textInputAutocapitalization(.never)
                }
                Button("Valider ma réponse") {
                  guard Date() < end else {
                    now = Date()
                    return
                  }
                  do {
                    try model.queue(
                      q.id, answer: value.trimmingCharacters(in: .whitespacesAndNewlines))
                    saved = true
                    count += 1
                  } catch { model.notice = model.message(error) }
                }.disabled(value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
              }
              Button("Terminer la séance") {
                self.end = Date()
                now = Date()
              }
            }
          } else {
            Section("Séance terminée") {
              Text("Réponses enregistrées · \(count)")
              Button("Revenir à ma journée") { self.end = nil }
            }
          }
        } else {
          Section("Cinq minutes de maths") {
            if due.isEmpty {
              Text(
                catalog.contains { $0.kind == "exercise" && $0.format != "text" }
                  ? "Vos exercices reviendront à leur prochaine échéance."
                  : "Les exercices apparaîtront après validation par votre enseignante.")
            } else {
              Text("Exercices à découvrir ou à revoir · \(due.count)")
              Button("Commencer ma séance") {
                ids = due.map(\.id)
                index = 0
                saved = false
                value = ""
                count = 0
                now = Date()
                end = now.addingTimeInterval(300)
              }
            }
          }
          if let progress = model.snapshot?.progress {
            Section("Ma progression") {
              Label("\(progress.streak) jours de suite", systemImage: "flame")
              Text("Réponses justes · \(progress.correct) sur \(progress.attempts)")
              ForEach(progress.skills, id: \.skill) { s in
                Text(
                  "\(s.skill) · \(Practice.date(s.nextReviewAt)?.formatted(date:.abbreviated,time:.omitted) ?? "")"
                )
              }
            }
          }
          if !model.answers.isEmpty {
            Section("Dernières réponses") {
              ForEach(Array(model.answers.suffix(8).reversed()), id: \.input.clientId) { row in
                VStack(alignment: .leading) {
                  Text(catalog.first { $0.id == row.input.exerciseId }?.title ?? "Exercice").bold()
                  Text(row.input.answer)
                  if let r = row.receipt {
                    Text(r.correct ? "Juste" : "À retravailler")
                    FormulaText(value: r.feedback)
                  } else {
                    Text(row.rejection == nil ? "À synchroniser" : "Réponse refusée par le serveur")
                  }
                }
              }
            }
          }
        }
      }.navigationTitle("Ma journée").refreshable { await model.sync() }.task {
        while !Task.isCancelled {
          now = Date()
          try? await Task.sleep(for: .seconds(1))
        }
      }
    }
  }
}
struct Course: View {
  @EnvironmentObject var model: MemberModel
  var onCopy: (String) -> Void
  var body: some View {
    NavigationStack {
      List {
        let chapters = (model.snapshot?.catalog ?? []).filter { $0.kind == "chapter" }.sorted {
          $0.position < $1.position
        }
        if chapters.isEmpty {
          Text("Les chapitres validés par votre enseignante apparaîtront ici.")
        }
        ForEach(chapters, id: \.id) { item in NavigationLink(item.title, value: item.id) }
      }.navigationTitle("Mon parcours").navigationDestination(for: String.self) { id in
        CoursePage(id: id, onCopy: onCopy)
      }
    }
  }
}
struct FileItem: Identifiable {
  let url: URL
  var id: String { url.path }
}
struct CoursePage: View {
  @EnvironmentObject var model: MemberModel
  let id: String
  let onCopy: (String) -> Void
  @State private var file: FileItem?
  @State private var error = ""
  var body: some View {
    let catalog = model.snapshot?.catalog ?? []
    if let item = catalog.first(where: { $0.id == id }) {
      List {
        FormulaText(value: item.body)
        if let video = URL(string: item.videoUrl), video.scheme == "https" {
          Link("Voir la vidéo du cours", destination: video)
        }
        if !item.sourceTitle.isEmpty {
          Text(item.sourceTitle)
          if let source = URL(string: item.sourceUrl), source.scheme == "https" {
            Link("Consulter la source", destination: source)
          }
        }
        if let description = item.videoDescription, !description.isEmpty {
          FormulaText(value: description)
        }
        ForEach(item.documents, id: \.id) { doc in
          Button(doc.title) {
            Task {
              do { file = FileItem(url: try await model.file(doc)) } catch {
                self.error = model.message(error)
              }
            }
          }
        }
        ForEach(catalog.filter { $0.parentId == id }.sorted { $0.position < $1.position }, id: \.id)
        { child in NavigationLink(child.title, value: child.id) }
        if Practice.exercises(in: catalog).contains(where: { $0.id == item.id }) {
          Button("Envoyer ma copie") { onCopy(id) }
        }
        if !error.isEmpty { Text(error) }
      }.navigationTitle(item.title).sheet(item: $file) { ShareFile(url: $0.url) }
    } else {
      Text("Ce contenu n’est plus disponible.")
    }
  }
}
struct Copies: View {
  @EnvironmentObject var model: MemberModel
  @Environment(\.scenePhase) private var scenePhase
  @Binding var exercise: String
  let isVisible: Bool
  @State private var text = ""
  @State private var photo: PhotosPickerItem?
  @State private var bytes: Data?
  @State private var camera = false
  @State private var message = ""
  @State private var converting = false
  var body: some View {
    NavigationStack {
      Form {
        let exercises = Practice.exercises(in: model.snapshot?.catalog ?? [])
        Section("Déposer une copie") {
          if exercises.isEmpty {
            Text("Le dépôt sera disponible dès la publication d’un exercice.")
          } else {
            Picker("Exercice", selection: $exercise) {
              Text("Choisir un exercice").tag("")
              ForEach(exercises, id: \.id) { Text($0.title).tag($0.id) }
            }.accessibilityIdentifier("copy-exercise-picker")
            if let item = exercises.first(where: { $0.id == exercise }) {
              FormulaText(value: item.body)
              TextField("Votre réponse ou une précision", text: $text, axis: .vertical).lineLimit(
                2...8)
              if let bytes, let image = UIImage(data: bytes) {
                Image(uiImage: image).resizable().scaledToFit().frame(maxHeight: 160)
                  .accessibilityLabel("Aperçu de la copie")
              }
              PhotosPicker("Choisir une photo", selection: $photo, matching: .images)
              if UIImagePickerController.isSourceTypeAvailable(.camera) {
                Button("Prendre une photo") {
                  Task {
                    if await AVCaptureDevice.requestAccess(for: .video) {
                      camera = true
                    } else {
                      message = "Autorisez l’appareil photo dans les réglages."
                    }
                  }
                }
              }
              Button("Transmettre ma copie") {
                do {
                  try model.queueCopy(
                    exercise, text: text.trimmingCharacters(in: .whitespacesAndNewlines),
                    image: bytes)
                  self.bytes = nil
                  text = ""
                  photo = nil
                  exercise = ""
                  message = "Copie enregistrée."
                } catch { message = model.message(error) }
              }
              .disabled(
                converting
                  || (bytes == nil && text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                  || text.count > 12000)
            }
          }
          if !message.isEmpty { Text(message) }
        }
        Section("Suivi des copies") {
          if !model.trackingNotice.isEmpty { Text(model.trackingNotice).font(.footnote) }
          ForEach(model.copies, id: \.id) { row in
            Text(
              (exercises.first { $0.id == row.exerciseId }?.title ?? "Copie") + " · "
                + (row.rejection == nil ? "À synchroniser" : "Envoi refusé"))
          }
          ForEach(model.snapshot?.submissions ?? [], id: \.id) { item in
            VStack(alignment: .leading) {
              Text(exercises.first { $0.id == item.exerciseId }?.title ?? "Copie").bold()
              if let progress = item.correctionProgress {
                Text(CopyTracking.label(progress.stage)).font(.headline)
                if let updated = Practice.date(progress.updatedAt) {
                  Text(
                    "Dernière mise à jour · "
                      + updated.formatted(date: .abbreviated, time: .shortened)
                  ).font(.footnote)
                }
                if progress.stage == "retrying" {
                  Text("Essais de traitement · \(progress.attempts)").font(.footnote)
                }
              } else {
                Text("État détaillé indisponible")
              }
              if let correction = item.correction {
                Text(
                  correction.generatedByAI
                    ? "Généré par IA"
                    : correction.verdict == "uncertain"
                      ? "À relire par l’enseignante" : "Vérification numérique"
                ).font(
                  .caption.bold())
                FormulaText(value: correction.summary)
                ForEach(Array(correction.errors.enumerated()), id: \.offset) { _, fault in
                  FormulaText(value: fault.location + " · " + fault.message)
                }
                ForEach(Array(correction.writingFeedback.enumerated()), id: \.offset) {
                  _, feedback in FormulaText(value: feedback)
                }
                if !correction.referenceSolution.isEmpty {
                  Text("Corrigé de référence").bold()
                  FormulaText(value: correction.referenceSolution)
                }
                if let similar = correction.similarExerciseId,
                  exercises.contains(where: { $0.id == similar })
                {
                  Button("Ouvrir l’exercice conseillé") { exercise = similar }
                }
              }
              if !item.feedback.isEmpty { FormulaText(value: item.feedback) }
              if let value = item.reviewVideoUrl, let video = URL(string: value),
                video.scheme == "https"
              {
                Link("Voir le retour vidéo", destination: video)
              }
            }
          }
        }
      }.navigationTitle("Mes copies").refreshable { await model.sync() }
        .task(
          id: CopyTracking.shouldPoll(
            visible: isVisible, active: scenePhase == .active, signedIn: model.session != nil,
            stages: model.snapshot?.submissions.compactMap { $0.correctionProgress?.stage } ?? [])
        ) {
          guard
            CopyTracking.shouldPoll(
              visible: isVisible, active: scenePhase == .active, signedIn: model.session != nil,
              stages: model.snapshot?.submissions.compactMap { $0.correctionProgress?.stage } ?? [])
          else { return }
          while !Task.isCancelled {
            await model.refreshCopies()
            do { try await Task.sleep(for: .seconds(4)) } catch { return }
          }
        }
        .task(id: photo) {
          guard let photo else { return }
          converting = true
          defer { converting = false }
          do {
            if let data = try await photo.loadTransferable(type: Data.self) {
              bytes = try copyJPEG(data)
            }
          } catch { message = "Impossible de lire cette photo." }
        }
        .sheet(isPresented: $camera) {
          Camera { image in
            camera = false
            if let image, let original = image.jpegData(compressionQuality: 0.9) {
              do { bytes = try copyJPEG(original) } catch { message = model.message(error) }
            }
          }
        }
    }
  }
}
struct Account: View {
  @EnvironmentObject var model: MemberModel
  @State private var hour = 19
  @State private var confirmation = ""
  @State private var file: FileItem?
  var body: some View {
    NavigationStack {
      Form {
        Section("Mon espace") {
          Text(model.session?.user.email ?? "")
          if let date = model.snapshot?.updated { Text("Synchronisé le " + date.formatted()) }
          Button("Synchroniser maintenant") { Task { await model.sync() } }.disabled(model.syncing)
          if !model.notice.isEmpty { Text(model.notice) }
        }
        Section("Abonnement") { NavigationLink("Mon abonnement") { StoreScreen() } }
        Section("Rappel quotidien") {
          Text(model.reminder.map { "Rappel actif à \($0) h." } ?? "Le rappel est désactivé.")
          Stepper("Heure locale · \(hour) h", value: $hour, in: 0...23)
          Button(model.reminder == nil ? "Activer le rappel" : "Modifier le rappel") {
            Task { await model.setReminder(hour) }
          }
          if model.reminder != nil {
            Button("Désactiver") { Task { await model.setReminder(nil) } }
          }
        }
        Section("Mes données") {
          Button("Exporter mes données") {
            Task {
              do { file = FileItem(url: try await model.file()) } catch {
                model.notice = model.message(error)
              }
            }
          }
          Button("Me déconnecter") { confirmation = "Me déconnecter" }
          Button("Supprimer mon compte", role: .destructive) {
            confirmation = "Supprimer mon compte"
          }
        }
      }.navigationTitle("Mon compte").onAppear { hour = model.reminder ?? 19 }
        .confirmationDialog(
          confirmation,
          isPresented: Binding(
            get: { !confirmation.isEmpty }, set: { if !$0 { confirmation = "" } }),
          titleVisibility: .visible
        ) {
          let deleting = confirmation == "Supprimer mon compte"
          Button(confirmation, role: .destructive) { Task { await model.exit(delete: deleting) } }
          Button("Annuler", role: .cancel) {}
        } message: {
          Text(
            confirmation == "Supprimer mon compte"
              ? "Votre compte, vos copies et vos réponses seront effacés définitivement."
              : "Les données téléchargées et les envois en attente seront effacés de cet appareil.")
        }
        .sheet(item: $file) { ShareFile(url: $0.url) }
    }
  }
}
