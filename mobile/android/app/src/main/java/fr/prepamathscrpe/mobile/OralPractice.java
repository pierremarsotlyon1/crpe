package fr.prepamathscrpe.mobile;

import android.Manifest;
import android.app.AlertDialog;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.Typeface;
import android.media.MediaPlayer;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.text.Editable;
import android.text.TextWatcher;
import android.view.View;
import android.widget.*;
import androidx.activity.ComponentActivity;
import androidx.activity.OnBackPressedCallback;
import java.io.*;
import java.time.Instant;
import java.util.*;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import org.json.*;

public final class OralPractice extends ComponentActivity {
  private final OnBackPressedCallback back =
      new OnBackPressedCallback(false) {
        @Override
        public void handleOnBackPressed() {
          stopRecording();
          stopPlayer();
          session = null;
          draft = null;
          message("");
          render();
        }
      };
  private final ExecutorService executor = Executors.newSingleThreadExecutor();
  private final Handler handler = new Handler(Looper.getMainLooper());
  private final OralRecording recording = new OralRecording();
  private MemberAPI api;
  private PracticeDB db;
  private MemberSession keychain;
  private String owner;
  private File folder;
  private JSONArray history = new JSONArray();
  private JSONObject session, draft;
  private LinearLayout column;
  private ScrollView scroll;
  private TextView notice, countdown, talkClock;
  private Button submitButton;
  private MediaPlayer player;
  private File playback;
  private boolean busy, resumed, permitted;
  private int requestedSlot = -2;
  private String status = "";
  private final Runnable tick =
      new Runnable() {
        @Override
        public void run() {
          clocks();
          if (resumed) handler.postDelayed(this, 1000);
        }
      };

  @Override
  protected void onCreate(Bundle saved) {
    super.onCreate(saved);
    getOnBackPressedDispatcher().addCallback(this, back);
    try {
      keychain = new MemberSession(this);
      JSONObject credentials = keychain.read();
      owner = credentials.getJSONObject("user").getString("id");
      folder = LocalFiles.folder(this, owner);
      api = new MemberAPI(credentials.getString("token"));
      db = new PracticeDB(this);
      scroll = new ScrollView(this);
      column = new LinearLayout(this);
      column.setOrientation(LinearLayout.VERTICAL);
      column.setPadding(dp(20), dp(32), dp(20), dp(28));
      scroll.addView(column);
      // Keep input state in the owner's SQLite records, never Android's shared instance bundle.
      scroll.setSaveFromParentEnabled(false);
      setContentView(scroll);
      fetch(saved == null ? "" : saved.getString("id", ""));
    } catch (Exception e) {
      finish();
    }
  }

  @Override
  protected void onSaveInstanceState(Bundle state) {
    super.onSaveInstanceState(state);
    if (session != null) state.putString("id", session.optString("id"));
  }

  @Override
  protected void onResume() {
    super.onResume();
    resumed = true;
    handler.post(tick);
    if (owner != null && !sameOwner()) finish();
  }

  @Override
  protected void onStop() {
    resumed = false;
    handler.removeCallbacks(tick);
    if (recording.slot != -2) stopRecording();
    stopPlayer();
    super.onStop();
  }

  @Override
  protected void onDestroy() {
    if (api != null) api.cancel();
    executor.shutdownNow();
    if (db != null) db.close();
    super.onDestroy();
  }

  private boolean sameOwner() {
    try {
      return owner.equals(keychain.read().getJSONObject("user").getString("id"));
    } catch (Exception e) {
      return false;
    }
  }

  private int dp(int x) {
    return (int) (getResources().getDisplayMetrics().density * x + 0.5f);
  }

  private interface Request {
    Object execute() throws Exception;
  }

  private interface Result {
    void apply(Object result) throws Exception;
  }

  private void request(Request job, Result result, boolean voice) {
    if (busy || isFinishing() || isDestroyed()) return;
    if (!sameOwner()) {
      finish();
      return;
    }
    busy = true;
    message("Chargement…");
    executor.execute(
        () -> {
          try {
            Object value = job.execute();
            runOnUiThread(
                () -> {
                  if (isDestroyed() || isFinishing()) return;
                  if (!sameOwner()) {
                    finish();
                    return;
                  }
                  busy = false;
                  message("");
                  try {
                    result.apply(value);
                  } catch (Exception e) {
                    message("Impossible d’enregistrer ou de lire ces données.");
                  }
                });
          } catch (Exception e) {
            runOnUiThread(
                () -> {
                  if (isDestroyed() || isFinishing()) return;
                  busy = false;
                  if (e instanceof MemberAPI.Failure f && (f.status == 401 || f.status == 403)) {
                    permitted = false;
                    history = new JSONArray();
                    session = null;
                    draft = null;
                    render();
                  }
                  if (e instanceof MemberAPI.Failure f && f.status == 503 && voice)
                    message(
                        "Le service vocal est indisponible. Vous pouvez saisir votre texte et lire"
                            + " les questions.");
                  else if (e instanceof MemberAPI.Failure f && f.status == 404)
                    message("Aucun sujet oral validé disponible pour cette demande.");
                  else if (e instanceof MemberAPI.Failure f && (f.status == 413 || f.status == 415))
                    message("L’enregistrement doit être un fichier M4A de moins de 20 Mio.");
                  else message(MemberAPI.message(e));
                });
          }
        });
  }

  private void fetch(String selected) {
    render();
    request(
        () -> {
          JSONObject access = api.json("GET", ApiPath.getMemberAccess, null);
          boolean allowed = false;
          for (JSONObject f : Repository.list(access.optJSONArray("features"))) {
            if ("oral".equals(f.optString("key"))) allowed = f.optBoolean("allowed");
          }
          if (!allowed) throw new MemberAPI.Failure(403);
          return api.json("GET", ApiPath.listOral, null).getJSONArray("items");
        },
        result -> {
          permitted = true;
          history = (JSONArray) result;
          session = null;
          draft = null;
          for (JSONObject item : Repository.list(history))
            if (item.optString("id").equals(selected)) open(item);
          render();
        },
        false);
  }

  private void open(JSONObject item) throws JSONException {
    session = item;
    draft =
        new JSONObject()
            .put("id", item.getString("id"))
            .put("transcript", item.optString("transcript"))
            .put("answers", new JSONArray());
    JSONArray answers = item.getJSONArray("answers");
    for (int i = 0; i < item.getJSONArray("questions").length(); i++)
      draft.getJSONArray("answers").put(answers.optString(i, ""));
    if ("active".equals(item.optString("status"))) {
      for (JSONObject saved : Repository.list(db.rows(owner, "oral-draft")))
        if (item.getString("id").equals(saved.getString("id"))) draft = saved;
    }
    scroll.scrollTo(0, 0);
  }

  private void save() {
    db.put(owner, "oral-draft", session.optString("id"), draft, null);
  }

  private int count() {
    return session.optJSONArray("questions").length();
  }

  private String answer(int slot) {
    return slot == -1
        ? draft.optString("transcript")
        : draft.optJSONArray("answers").optString(slot, "");
  }

  private boolean change(int slot, String text) {
    if (text.getBytes(java.nio.charset.StandardCharsets.UTF_8).length
        > (slot == -1 ? 120000 : 16000)) {
      message("Le texte dépasse la longueur acceptée.");
      return false;
    }
    try {
      if (slot == -1) draft.put("transcript", text);
      else draft.getJSONArray("answers").put(slot, text);
      save();
      if (submitButton != null) submitButton.setEnabled(complete() && recording.slot == -2);
      return true;
    } catch (Exception e) {
      message("Échec de l’enregistrement local.");
      return false;
    }
  }

  private boolean complete() {
    if (count() < 2 || count() > 3) return false;
    for (int i = -1; i < count(); i++) if (answer(i).trim().isEmpty()) return false;
    return true;
  }

  private TextView text(String value, int size) {
    TextView view = new TextView(this);
    view.setText(value);
    view.setTextSize(size);
    view.setTextColor(Color.rgb(29, 53, 49));
    view.setPadding(0, dp(3), 0, dp(3));
    column.addView(view);
    return view;
  }

  private void heading(String value) {
    TextView v = text(value, 21);
    v.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
    v.setPadding(0, dp(16), 0, dp(3));
  }

  private Button button(String value, Runnable action) {
    Button b = new Button(this);
    b.setText(value);
    b.setAllCaps(false);
    column.addView(b);
    b.setOnClickListener(
        view -> {
          if (!busy) action.run();
        });
    return b;
  }

  private void message(String s) {
    status = s;
    if (notice != null) {
      notice.setText(s);
      notice.setVisibility(s.isEmpty() ? View.GONE : View.VISIBLE);
    }
  }

  private static String label(JSONObject s) {
    return switch (s.optString("status")) {
      case "active" -> "En cours";
      case "reviewed" -> "Évaluée";
      default -> "En attente d’évaluation";
    };
  }

  private void grid() {
    Map<String, List<String>> progress = new TreeMap<>();
    for (int i = history.length() - 1; i >= 0; i--) {
      JSONObject s = history.optJSONObject(i);
      for (JSONObject f : Repository.list(s.optJSONArray("feedback")))
        progress.computeIfAbsent(f.optString("criterion"), k -> new ArrayList<>()).add(mark(f));
    }
    if (!progress.isEmpty()) {
      heading("Progression par critère");
      for (var row : progress.entrySet())
        text(row.getKey() + "\n" + String.join(" → ", row.getValue()), 16);
    }
  }

  private static String mark(JSONObject f) {
    return f.optString("status").equals("validated") ? "Validé" : "À travailler";
  }

  private void render() {
    back.setEnabled(!busy && session != null);
    if (column == null) return;
    column.removeAllViews();
    submitButton = null;
    countdown = null;
    talkClock = null;
    heading("Oral blanc");
    notice = text(status, 16);
    notice.setAccessibilityLiveRegion(View.ACCESSIBILITY_LIVE_REGION_POLITE);
    message(status);
    if (session == null) {
      if (permitted) {
        button(
            "Tirer un sujet",
            () ->
                request(
                    () -> api.json("POST", ApiPath.startOral, new JSONObject()),
                    result -> {
                      JSONObject item = (JSONObject) result;
                      JSONArray next = new JSONArray().put(item);
                      for (JSONObject saved : Repository.list(history)) next.put(saved);
                      history = next;
                      open(item);
                      render();
                    },
                    false));
        if (history.length() > 0) heading("Mes séances");
        for (JSONObject item : Repository.list(history))
          button(
              item.optString("title") + " · " + label(item),
              () -> {
                try {
                  open(item);
                  render();
                } catch (Exception e) {
                  message("Impossible d’ouvrir la séance.");
                }
              });
        grid();
      }
      button("Actualiser les séances", () -> fetch(""));
      button("Retour aux activités", this::finish);
      return;
    }
    heading(session.optString("title"));
    text(session.optString("body"), 17);
    text(label(session), 15);
    heading("Critères de l’oral");
    JSONArray criteria = session.optJSONArray("criteria");
    for (int i = 0; i < criteria.length(); i++) text(criteria.optString(i), 16);
    if ("active".equals(session.optString("status"))) {
      heading("Préparation · " + session.optInt("prepMinutes") + " min");
      countdown = text("", 17);
      heading("Exposé · " + session.optInt("durationMinutes") + " min");
      talkClock = text("", 17);
      if (!draft.has("speakingSince"))
        button(
            "Démarrer le chronomètre de l’exposé",
            () -> {
              try {
                startClock();
                render();
              } catch (Exception e) {
                message("Impossible de démarrer le chronomètre.");
              }
            });
      editor(-1);
      for (int i = 0; i < count(); i++) {
        int index = i;
        heading("Question " + (i + 1));
        text(session.optJSONArray("questions").optString(i), 17);
        button("Écouter la question", () -> listen(index)).setEnabled(recording.slot == -2);
        editor(i);
      }
      submitButton =
          button(
              "Envoyer mon oral",
              () ->
                  new AlertDialog.Builder(this)
                      .setMessage("Transmettre l’exposé et les réponses pour évaluation ?")
                      .setNegativeButton("Revenir", null)
                      .setPositiveButton("Transmettre", (d, w) -> submit())
                      .show());
      submitButton.setEnabled(complete() && recording.slot == -2);
      clocks();
    } else {
      heading("Mon exposé");
      text(session.optString("transcript"), 16);
      for (int i = 0; i < count(); i++) {
        heading(session.optJSONArray("questions").optString(i));
        text(session.optJSONArray("answers").optString(i), 16);
      }
      if (session.optJSONArray("feedback").length() > 0)
        heading(
            session.optString("feedbackSource").equals("ai")
                ? "Évaluation par IA"
                : "Évaluation enseignante");
      for (JSONObject f : Repository.list(session.optJSONArray("feedback")))
        text(f.optString("criterion") + " · " + mark(f) + "\n" + f.optString("comment"), 16);
    }
    button("Actualiser la séance", () -> fetch(session.optString("id")))
        .setEnabled(recording.slot == -2);
    button(
            "Toutes mes séances",
            () -> {
              stopPlayer();
              session = null;
              draft = null;
              message("");
              render();
            })
        .setEnabled(recording.slot == -2);
  }

  private void editor(int slot) {
    File file = OralRecording.clip(folder, session.optString("id"), slot);
    boolean editing = recording.slot == -2;
    if (recording.slot == slot) button("Arrêter l’enregistrement", this::stopRecording);
    else if (file.isFile()) {
      button("Transcrire l’enregistrement", () -> transcribe(slot, file)).setEnabled(editing);
      button(
              "Effacer l’enregistrement",
              () -> {
                if (!file.delete()) message("Impossible d’effacer l’enregistrement.");
                render();
              })
          .setEnabled(editing);
    } else
      button(
              slot == -1 ? "Enregistrer mon exposé" : "Enregistrer ma réponse",
              () -> microphone(slot))
          .setEnabled(editing);
    EditText field = new EditText(this);
    field.setMinLines(2);
    field.setMaxLines(8);
    field.setInputType(
        android.text.InputType.TYPE_CLASS_TEXT
            | android.text.InputType.TYPE_TEXT_FLAG_MULTI_LINE
            | android.text.InputType.TYPE_TEXT_FLAG_CAP_SENTENCES);
    field.setHint(slot == -1 ? "Texte de mon exposé" : "Texte de ma réponse");
    field.setText(answer(slot));
    column.addView(field);
    field.addTextChangedListener(
        new TextWatcher() {
          public void beforeTextChanged(CharSequence s, int st, int count, int after) {}

          public void onTextChanged(CharSequence s, int st, int before, int count) {}

          public void afterTextChanged(Editable e) {
            if (!change(slot, e.toString())) {
              field.removeTextChangedListener(this);
              field.setText(answer(slot));
              field.addTextChangedListener(this);
            }
          }
        });
  }

  private void startClock() throws JSONException {
    if (!draft.has("speakingSince")) {
      draft.put("speakingSince", System.currentTimeMillis());
      save();
    }
  }

  private static String remaining(long end) {
    long seconds = Math.max(0, (end - System.currentTimeMillis() + 999) / 1000);
    return String.format(Locale.FRANCE, "%02d:%02d", seconds / 60, seconds % 60);
  }

  private void clocks() {
    if (session == null || countdown == null) return;
    try {
      long start = Instant.parse(session.optString("createdAt")).toEpochMilli();
      countdown.setText(
          "Temps restant  " + remaining(start + session.optLong("prepMinutes") * 60000));
    } catch (RuntimeException ignored) {
      countdown.setText("");
    }
    talkClock.setText(
        draft.has("speakingSince")
            ? "Temps indicatif restant  "
                + remaining(
                    draft.optLong("speakingSince") + session.optLong("durationMinutes") * 60000)
            : "");
  }

  private void microphone(int slot) {
    if (checkSelfPermission(Manifest.permission.RECORD_AUDIO)
        != PackageManager.PERMISSION_GRANTED) {
      requestedSlot = slot;
      requestPermissions(new String[] {Manifest.permission.RECORD_AUDIO}, 491);
      return;
    }
    beginRecord(slot);
  }

  @Override
  public void onRequestPermissionsResult(int code, String[] permissions, int[] grants) {
    super.onRequestPermissionsResult(code, permissions, grants);
    if (code != 491 || session == null || requestedSlot == -2) return;
    int target = requestedSlot;
    requestedSlot = -2;
    if (grants.length > 0 && grants[0] == PackageManager.PERMISSION_GRANTED && sameOwner())
      beginRecord(target);
    else message("Autorisez le microphone dans les paramètres ou saisissez votre texte.");
  }

  private void beginRecord(int slot) {
    if (busy || !sameOwner() || session == null) return;
    try {
      stopPlayer();
      int seconds = session.getInt("durationMinutes") * 60;
      if (slot == -1) {
        long elapsed =
            (System.currentTimeMillis()
                    - draft.optLong("speakingSince", System.currentTimeMillis()))
                / 1000;
        if (elapsed >= 0 && elapsed < seconds) seconds -= (int) elapsed;
      }
      recording.start(
          this,
          OralRecording.clip(folder, session.getString("id"), slot),
          slot,
          seconds,
          this::stopRecording);
      if (slot == -1) startClock();
      message("Enregistrement en cours.");
      render();
    } catch (Exception e) {
      recording.stop();
      message("Microphone indisponible. Vous pouvez saisir votre texte.");
      render();
    }
  }

  private void stopRecording() {
    if (recording.slot == -2 || session == null) return;
    File file = OralRecording.clip(folder, session.optString("id"), recording.slot);
    recording.stop();
    try {
      OralRecording.validate(file);
      message("Enregistrement prêt à transcrire.");
    } catch (IOException e) {
      file.delete();
      message("Enregistrement trop court ou interrompu. Réessayez.");
    }
    render();
  }

  private void transcribe(int slot, File file) {
    String id = session.optString("id");
    request(
        () -> api.transcribe(id, file),
        result -> {
          if (session == null || !id.equals(session.optString("id"))) return;
          String current = answer(slot);
          if (change(slot, current.isEmpty() ? (String) result : current + "\n" + result)) {
            file.delete();
            message("Transcription ajoutée. Relisez-la avant d’envoyer.");
          }
          render();
        },
        true);
  }

  private void listen(int index) {
    String id = session.optString("id");
    request(
        () -> api.speak(id, index),
        result -> {
          if (!resumed || session == null || !id.equals(session.optString("id"))) return;
          stopPlayer();
          try {
            playback = File.createTempFile("oral-question-", ".mp3", folder);
            try (FileOutputStream out = new FileOutputStream(playback)) {
              out.write((byte[]) result);
            }
            player = new MediaPlayer();
            player.setDataSource(playback.getPath());
            player.setOnPreparedListener(MediaPlayer::start);
            player.setOnCompletionListener(p -> stopPlayer());
            player.setOnErrorListener(
                (p, w, e) -> {
                  stopPlayer();
                  message("Lecture vocale impossible. La question reste affichée.");
                  return true;
                });
            player.prepareAsync();
          } catch (Exception e) {
            stopPlayer();
            message("Lecture vocale impossible. La question reste affichée.");
          }
        },
        true);
  }

  private void stopPlayer() {
    if (player != null) {
      player.release();
      player = null;
    }
    if (playback != null) {
      playback.delete();
      playback = null;
    }
  }

  private void submit() {
    if (session == null || !complete()) return;
    try {
      String id = session.getString("id");
      JSONObject input =
          new JSONObject()
              .put("transcript", answer(-1))
              .put("answers", new JSONArray(draft.getJSONArray("answers").toString()));
      // Freeze this copy while the server accepts it.
      setFieldsEnabled(column, false);
      request(
          () -> {
            try {
              return api.json("POST", ApiPath.finishOral.replace("{id}", id), input);
            } finally {
              runOnUiThread(() -> setFieldsEnabled(column, true));
            }
          },
          result -> {
            JSONObject next = (JSONObject) result;
            for (int i = -1; i < count(); i++) OralRecording.clip(folder, id, i).delete();
            db.remove(owner, "oral-draft", id);
            for (int i = 0; i < history.length(); i++)
              if (history.optJSONObject(i).optString("id").equals(id)) history.put(i, next);
            open(next);
            render();
          },
          false);
    } catch (Exception e) {
      message("Impossible de transmettre la séance.");
    }
  }

  private void setFieldsEnabled(LinearLayout parent, boolean enabled) {
    for (int i = 0; i < parent.getChildCount(); i++)
      if (parent.getChildAt(i) instanceof EditText v) v.setEnabled(enabled);
  }
}
