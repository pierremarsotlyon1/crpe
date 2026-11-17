package fr.prepamathscrpe.mobile;

import android.app.AlertDialog;
import android.os.*;
import android.text.*;
import android.widget.*;
import java.nio.charset.StandardCharsets;
import java.time.*;
import java.util.*;
import org.json.*;

final class MockPractice {
  private final MainActivity view;
  private final Handler handler = new Handler(Looper.getMainLooper());
  private JSONObject session, answers;
  private JSONArray history = new JSONArray();
  private TextView timer, state;
  private boolean foreground;
  private int edits;
  private final Runnable saveLater = () -> send(false);
  private final Runnable tick =
      new Runnable() {
        public void run() {
          if (!foreground || timer == null || !active()) return;
          long remaining = remaining();
          timer.setText(
              String.format(Locale.FRANCE, "%02d min %02d s", remaining / 60, remaining % 60));
          if (remaining > 0) handler.postDelayed(this, 1000);
          else {
            timer = null;
            view.render();
          }
        }
      };

  MockPractice(MainActivity view) {
    this.view = view;
  }

  private boolean active() {
    return session != null && "active".equals(session.optString("status"));
  }

  private long remaining() {
    try {
      return Math.max(
          0,
          Duration.between(Instant.now(), Instant.parse(session.getString("deadline")))
              .getSeconds());
    } catch (Exception e) {
      return 0;
    }
  }

  void resume() {
    foreground = true;
    handler.removeCallbacks(tick);
    if (timer != null) tick.run();
  }

  void pause() {
    foreground = false;
    handler.removeCallbacks(tick);
  }

  void close() {
    edits++;
    handler.removeCallbacks(saveLater);
    handler.removeCallbacks(tick);
    timer = null;
    state = null;
    session = null;
    answers = null;
    history = new JSONArray();
  }

  boolean back() {
    if (session == null) return false;
    close();
    loadHistory();
    return true;
  }

  void loadHistory() {
    JSONObject[] data = new JSONObject[1];
    view.task(
        () -> data[0] = view.repo.api().json("GET", ApiPath.listCompletedMocks, null),
        () -> history = data[0].optJSONArray("items"));
  }

  private JSONObject cached(String id, boolean bySubject) throws Exception {
    for (JSONObject entry : Repository.list(view.repo.db.rows(view.repo.owner(), "mock"))) {
      JSONObject saved = entry.getJSONObject("session");
      if (bySubject
          ? id.equals(saved.optString("subjectId")) && "active".equals(saved.optString("status"))
          : id.equals(saved.optString("id"))) return entry;
    }
    return null;
  }

  private void install(JSONObject data, JSONObject local) throws Exception {
    JSONObject merged = new JSONObject(data.getJSONObject("answers").toString());
    if (local != null && "active".equals(data.optString("status"))) {
      JSONObject previous = local.getJSONObject("session").getJSONObject("answers"),
          draft = local.getJSONObject("answers");
      Iterator<String> keys = draft.keys();
      while (keys.hasNext()) {
        String q = keys.next();
        if (!draft.getString(q).equals(previous.optString(q))) merged.put(q, draft.getString(q));
      }
    }
    session = data;
    answers = merged;
    persist();
  }

  private void persist() throws Exception {
    JSONObject row = new JSONObject().put("session", session).put("answers", answers);
    view.repo.db.put(view.repo.owner(), "mock", session.getString("id"), row, null);
  }

  private void open(String id, boolean bySubject) {
    JSONObject[] remote = new JSONObject[1], saved = new JSONObject[1];
    try {
      saved[0] = cached(id, bySubject);
      if (saved[0] != null) install(saved[0].getJSONObject("session"), saved[0]);
    } catch (Exception error) {
      view.toast(MemberAPI.message(error));
      return;
    }
    view.task(
        () -> {
          if (bySubject && saved[0] == null)
            remote[0] =
                view.repo.api().json("POST", ApiPath.startMock, Repository.object("subjectId", id));
          else {
            String sessionId = bySubject ? saved[0].getJSONObject("session").getString("id") : id;
            remote[0] =
                view.repo
                    .api()
                    .json(
                        "GET",
                        ApiPath.getMock.replace("{id}", UUID.fromString(sessionId).toString()),
                        null);
          }
        },
        () -> {
          try {
            install(remote[0], saved[0]);
            if (active() && saved[0] != null) handler.postDelayed(saveLater, 1000);
          } catch (Exception error) {
            view.toast(MemberAPI.message(error));
          }
        });
  }

  private void edit(JSONObject question, String text) {
    if (!active() || remaining() == 0) return;
    int limit = "text".equals(question.optString("format")) ? 48000 : 4096;
    if (text.getBytes(StandardCharsets.UTF_8).length > limit) {
      view.toast("Votre réponse dépasse la taille autorisée.");
      return;
    }
    try {
      answers.put(question.getString("id"), text);
      persist();
      edits++;
      if (state != null) state.setText("Réponse enregistrée sur cet appareil.");
      handler.removeCallbacks(saveLater);
      handler.postDelayed(saveLater, 1000);
    } catch (Exception error) {
      view.toast(MemberAPI.message(error));
    }
  }

  private void send(boolean finish) {
    if (!active() || !view.repo.allows("mock")) return;
    try {
      String id = session.getString("id"), owner = view.repo.owner();
      int revision = edits;
      JSONArray list = new JSONArray();
      for (Iterator<String> keys = answers.keys(); keys.hasNext(); ) {
        String key = keys.next();
        list.put(Repository.object("exerciseId", key, "answer", answers.getString(key)));
      }
      JSONObject payload = new JSONObject().put("answers", list);
      MemberAPI api = view.repo.api();
      String path = (finish ? ApiPath.finishMock : ApiPath.saveMockAnswers).replace("{id}", id);
      if (finish) {
        handler.removeCallbacks(saveLater);
        JSONObject[] done = new JSONObject[1];
        view.task(
            () -> done[0] = api.json("POST", path, payload),
            () -> {
              try {
                install(done[0], null);
              } catch (Exception e) {
                view.toast(MemberAPI.message(e));
              }
            });
      } else {
        // Serialize edits with final submission, without rebuilding the view while the user types.
        MainActivity.worker.execute(
            () -> {
              try {
                JSONObject synced = api.json("PUT", path, payload);
                view.runOnUiThread(
                    () -> {
                      if (view.isDestroyed() || !sameDraft(owner, id) || revision != edits) return;
                      try {
                        install(synced, null);
                        if (!active()) view.render();
                        else if (state != null) state.setText("Réponses synchronisées.");
                      } catch (Exception e) {
                        view.toast(MemberAPI.message(e));
                      }
                    });
              } catch (Exception e) {
                view.runOnUiThread(
                    () -> {
                      if (!view.isDestroyed() && sameDraft(owner, id) && state != null)
                        state.setText(
                            "Réponses conservées sur cet appareil. Synchronisez avant la fin du"
                                + " sujet.");
                    });
              }
            });
      }
    } catch (Exception e) {
      view.toast(MemberAPI.message(e));
    }
  }

  private boolean sameDraft(String owner, String id) {
    try {
      return session != null
          && view.repo.session != null
          && owner.equals(view.repo.owner())
          && id.equals(session.optString("id"));
    } catch (JSONException e) {
      return false;
    }
  }

  void show() {
    timer = null;
    state = null;
    handler.removeCallbacks(tick);
    view.title("Sujets blancs");
    if (session == null) {
      boolean found = false;
      for (JSONObject item : view.repo.catalog())
        if ("mock".equals(item.optString("kind"))) {
          found = true;
          view.heading(item.optString("title"));
          view.math(item.optString("body"));
          view.label("Durée · " + item.optInt("durationMinutes") + " min", 15);
          view.button("Ouvrir le sujet", true, () -> open(item.optString("id"), true))
              .setContentDescription("Ouvrir le sujet " + item.optString("title"));
        }
      if (!found) view.label("Aucun sujet blanc validé n’est disponible.", 17);
      if (history != null && history.length() > 0) {
        view.heading("Mes résultats");
        for (JSONObject result : Repository.list(history))
          view.button(
              result.optString("title") + " · " + result.optString("startedAt").split("T")[0],
              false,
              () -> open(result.optString("id"), false));
      }
      return;
    }
    if (active()) {
      long seconds = remaining();
      view.heading("Temps restant");
      timer =
          view.label(
              String.format(Locale.FRANCE, "%02d min %02d s", seconds / 60, seconds % 60), 22);
      if (foreground && seconds > 0) handler.postDelayed(tick, 1000);
      state =
          view.label(
              seconds == 0 ? "Temps écoulé" : "Vos réponses sont enregistrées sur cet appareil.",
              15);
      for (JSONObject q : Repository.list(session.optJSONArray("items"))) {
        view.heading(q.optString("title"));
        view.math(q.optString("body"));
        String id = q.optString("id");
        if ("choice".equals(q.optString("format"))) {
          JSONArray choices = q.optJSONArray("choices");
          if (choices != null)
            for (int i = 0; i < choices.length(); i++) {
              String value = choices.optString(i);
              Button b =
                  view.button(
                      value,
                      value.equals(answers.optString(id)),
                      () -> {
                        edit(q, value);
                        view.render();
                      });
              if (seconds == 0) b.setEnabled(false);
            }
        } else {
          boolean written = "text".equals(q.optString("format"));
          EditText input =
              view.field(
                  written ? "Votre raisonnement" : "Votre réponse",
                  answers.optString(id),
                  InputType.TYPE_CLASS_TEXT | (written ? InputType.TYPE_TEXT_FLAG_MULTI_LINE : 0),
                  v -> edit(q, v));
          if (seconds == 0) input.setEnabled(false);
        }
      }
      if (seconds > 0) view.button("Enregistrer mes réponses", false, () -> send(false));
      view.button(
          seconds > 0 ? "Rendre ma copie" : "Obtenir mon résultat",
          true,
          () ->
              new AlertDialog.Builder(view)
                  .setTitle("Rendre définitivement ma copie ?")
                  .setNegativeButton("Annuler", null)
                  .setPositiveButton("Rendre ma copie", (d, w) -> send(true))
                  .show());
    } else {
      view.heading(
          "pending_review".equals(session.optString("gradingStatus"))
              ? "Résultat provisoire"
              : "Résultat");
      view.label(
          session.optDouble("score") + " / " + session.optDouble("maxScore") + " points", 22);
      for (JSONObject r : Repository.list(session.optJSONArray("results"))) {
        for (JSONObject q : Repository.list(session.optJSONArray("items")))
          if (r.optString("exerciseId").equals(q.optString("id"))) {
            view.heading(q.optString("title"));
            break;
          }
        view.label(
            r.isNull("points")
                ? "Réponse écrite en attente de relecture"
                : r.optDouble("points") + " / " + r.optDouble("maxPoints"),
            16);
        view.math(r.optString("feedback"));
        view.math(r.optString("teacherFeedback"));
      }
      view.math(session.optString("teacherFeedback"));
      String url = session.optString("reviewVideoUrl");
      if (view.repo.allows("mock_video") && FeatureAccess.https(url))
        view.button("Voir le retour vidéo", false, () -> view.external(url));
    }
    view.button(
        "Revenir aux sujets",
        false,
        () -> {
          close();
          loadHistory();
        });
  }
}
