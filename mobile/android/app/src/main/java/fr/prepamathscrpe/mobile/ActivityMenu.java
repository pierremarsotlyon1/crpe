package fr.prepamathscrpe.mobile;

import android.content.Intent;
import android.widget.Button;
import org.json.*;

final class ActivityMenu {
  private final MainActivity view;
  private final MockPractice mock;
  private String key = "";
  private JSONObject support;

  ActivityMenu(MainActivity view) {
    this.view = view;
    this.mock = new MockPractice(view);
  }

  void pause() {
    mock.pause();
  }

  void resume() {
    mock.resume();
  }

  void leave() {
    key = "";
    view.billing.leave();
    support = null;
    mock.close();
  }

  boolean back() {
    if (key.isEmpty()) return false;
    if (key.equals("mock") && mock.back()) return true;
    leave();
    return true;
  }

  void locked(String key) {
    JSONObject feature = view.repo.feature(key);
    if (feature == null) {
      view.heading("Vérification des accès");
      view.label("Connectez-vous à Internet pour actualiser vos accès.", 17);
      view.button("Actualiser", false, () -> view.task(view.repo::sync, null));
      return;
    }
    String tier = FeatureAccess.tier(feature.optString("requiredTier"));
    view.title(feature.optString("label"));
    view.label("Cette activité est incluse dans la formule " + tier + ".", 17);
    if (view.repo.access != null && view.repo.access.optBoolean("salesEnabled"))
      view.billing.show(feature.optString("requiredTier"));
  }

  void show() {
    if (key.isEmpty()) {
      view.title("Activités");
      if (view.repo.access == null) {
        locked("");
        return;
      }
      for (JSONObject f : Repository.list(view.repo.access.optJSONArray("features"))) {
        String id = f.optString("key"), title = f.optString("label");
        boolean allowed = view.repo.allows(id);
        Button control =
            view.button(
                allowed
                    ? title
                    : title + " · " + FeatureAccess.tier(f.optString("requiredTier")) + " 🔒",
                false,
                () -> enter(id));
        if (!allowed) control.setTextColor(0xff65736a);
      }
      return;
    }
    view.button(
        "Toutes les activités",
        false,
        () -> {
          leave();
          view.render();
        });
    if (!view.repo.allows(key)) {
      support = null;
      locked(key);
      return;
    }
    if (key.equals("mock")) {
      mock.show();
      return;
    }
    JSONObject capability = view.repo.feature(key);
    view.title(capability.optString("label"));
    if (support == null) {
      view.button("Actualiser", false, this::loadSupport);
      return;
    }
    if (key.equals("group")) {
      link("Rejoindre le groupe", "groupUrl", "Le lien du groupe sera publié par l’enseignante.");
    } else if (key.equals("live")) {
      view.label(support.optString("liveSchedule"), 17);
      link(
          "Rejoindre le direct",
          "liveUrl",
          "Le prochain lien de direct sera publié par l’enseignante.");
    } else {
      boolean found = false;
      for (JSONObject item : Repository.list(support.optJSONArray("mockReviews"))) {
        String url = item.optString("reviewVideoUrl");
        if (!FeatureAccess.https(url)) continue;
        found = true;
        view.heading(item.optString("title"));
        view.math(item.optString("feedback"));
        view.button("Voir mon retour vidéo", false, () -> view.external(url));
      }
      if (!found) view.label("Vous retrouverez ici les retours vidéo de vos concours blancs.", 17);
    }
  }

  private void link(String title, String field, String empty) {
    String url = support.optString(field);
    if (FeatureAccess.https(url)) view.button(title, true, () -> view.external(url));
    else view.label(empty, 17);
  }

  private void loadSupport() {
    String requested = key;
    JSONObject[] result = new JSONObject[1];
    view.task(
        () -> result[0] = view.repo.api().json("GET", ApiPath.getSupport, null),
        () -> {
          if (requested.equals(key) && view.repo.allows(key)) support = result[0];
        });
  }

  private void enter(String id) {
    if (!view.repo.allows(id)) {
      key = id;
      view.render();
      return;
    }
    switch (id) {
      case "practice" -> view.jump("Journée");
      case "course" -> view.jump("Parcours");
      case "tutor" -> view.jump("Copies");
      case "oral" -> view.startActivity(new Intent(view, OralPractice.class));
      case "mock" -> {
        key = id;
        mock.loadHistory();
      }
      default -> {
        key = id;
        loadSupport();
      }
    }
  }
}
