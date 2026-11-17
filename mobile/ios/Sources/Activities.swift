import SwiftUI

enum FeatureAccess {
  static func eligible(
    _ feature: Capability, access: MemberAccess, at now: Date, administrator: Bool = false
  ) -> Bool {
    guard feature.allowed else { return false }
    if administrator && access.tier == "suivi" && access.validUntil == nil { return true }
    if !access.salesEnabled && feature.requiredTier == "autonomie" { return true }
    if let until = access.validUntil.flatMap(Practice.date), until > now { return true }
    return (access.providers ?? []).contains { provider in
      (provider.tier == "suivi" || provider.tier == feature.requiredTier)
        && (Practice.date(provider.expiresAt) ?? .distantPast) > now
    }
  }
  static func tier(_ value: String) -> String { value == "suivi" ? "Suivi" : "Autonomie" }
  static func https(_ value: String?) -> URL? {
    guard let value, let url = URL(string: value), url.scheme == "https",
      let host = url.host, !host.isEmpty, url.user == nil, url.password == nil
    else { return nil }
    return url
  }

}
struct FeatureGate: View {
  @EnvironmentObject var model: MemberModel
  let key: String
  var body: some View {
    if model.access?.salesEnabled == true, let feature = model.feature(key) {
      StoreScreen(tier: feature.requiredTier)
    } else {
      VStack(spacing: 16) {
        if let feature = model.feature(key) {
          Image(systemName: "lock.fill").font(.title).foregroundStyle(.secondary)
          Text(feature.label).font(.title2.bold())
          Text(
            "Cette activité est incluse dans la formule " + FeatureAccess.tier(feature.requiredTier)
              + "."
          )
          .multilineTextAlignment(.center)
        } else if model.syncing {
          ProgressView("Vérification des accès…")
        } else {
          Text("Connectez-vous à Internet pour actualiser vos accès.")
          Button("Actualiser") { Task { await model.sync() } }
        }
      }.padding()
    }
  }
}
struct Activities: View {
  @EnvironmentObject var model: MemberModel
  let jump: (String) -> Void
  var body: some View {
    NavigationStack {
      List {
        if let access = model.access {
          ForEach(access.features.compactMap { model.feature($0.key) }, id: \.key) { feature in
            if feature.allowed && ["course", "practice", "tutor"].contains(feature.key) {
              Button(feature.label) { jump(feature.key) }
            } else {
              NavigationLink {
                if !model.allows(feature.key) {
                  FeatureGate(key: feature.key)
                } else if feature.key == "mock" {
                  MockScreen()
                } else if feature.key == "oral" {
                  OralPracticeView()
                } else {
                  SupportScreen(key: feature.key)
                }
              } label: {
                HStack {
                  Text(feature.label)
                  if !feature.allowed {
                    Spacer()
                    Label(FeatureAccess.tier(feature.requiredTier), systemImage: "lock.fill").font(
                      .caption)
                  }
                }.foregroundStyle(feature.allowed ? Color.primary : Color.secondary)
              }
            }
          }
        } else {
          FeatureGate(key: "")
        }
      }.navigationTitle("Activités")
    }
  }
}
private struct SupportScreen: View {
  @EnvironmentObject var model: MemberModel
  let key: String
  @State private var support: Support?
  @State private var error = ""
  private func refresh() async {
    do {
      guard model.allows(key) else { return }
      let result = try await model.authorizedAPI().read(APIPath.getSupport, Support.self)
      try Task.checkCancellation()
      guard model.allows(key) else { return }
      support = result
      error = ""
    } catch { if !Task.isCancelled { self.error = model.message(error) } }
  }
  var body: some View {
    List {
      if !model.allows(key) {
        FeatureGate(key: key)
      } else if let support {
        if key == "live" {
          if !support.liveSchedule.isEmpty { Text(support.liveSchedule) }
          if let url = FeatureAccess.https(support.liveUrl) {
            Link("Rejoindre le direct", destination: url)
          } else {
            Text("Le prochain lien de direct sera publié par l’enseignante.")
          }
        } else if key == "group" {
          if let url = FeatureAccess.https(support.groupUrl) {
            Link("Rejoindre le groupe", destination: url)
          } else {
            Text("Le lien du groupe sera publié par l’enseignante.")
          }
        } else {
          let videos = support.mockReviews.filter { FeatureAccess.https($0.reviewVideoUrl) != nil }
          if videos.isEmpty {
            Text("Vous retrouverez ici les retours vidéo de vos concours blancs.")
          }
          ForEach(videos, id: \.id) { review in
            Section(review.title) {
              if !review.feedback.isEmpty { FormulaText(value: review.feedback) }
              if let url = FeatureAccess.https(review.reviewVideoUrl) {
                Link("Voir mon retour vidéo", destination: url)
              }
            }
          }
        }
      } else if !error.isEmpty {
        Text(error)
        Button("Réessayer") { Task { await refresh() } }
      } else {
        ProgressView()
      }
    }.navigationTitle(model.feature(key)?.label ?? "Suivi").task { await refresh() }
  }
}
