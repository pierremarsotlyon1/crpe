package fr.prepamathscrpe.mobile;

import android.Manifest;
import android.app.*;
import android.content.*;
import android.content.pm.PackageManager;
import android.graphics.*;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.*;
import android.provider.MediaStore;
import android.text.*;
import android.view.*;
import android.webkit.WebView;
import android.widget.*;
import java.io.*;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.time.*;
import java.time.format.DateTimeFormatter;
import java.util.*;
import java.util.concurrent.*;
import org.json.*;

public final class MainActivity extends androidx.activity.ComponentActivity {
  private final androidx.activity.OnBackPressedCallback back =
      new androidx.activity.OnBackPressedCallback(false) {
        @Override
        public void handleOnBackPressed() {
          backInMemberArea();
        }
      };
  private static final int INK = 0xff183e35, PAPER = 0xfff7f5ef, WHITE = 0xfffffef9;
  static final ExecutorService worker = Executors.newSingleThreadExecutor();
  private final Handler handler = new Handler(Looper.getMainLooper());
  Repository repo;
  final PlaySubscriptions billing = new PlaySubscriptions(this);
  private boolean accountPurchases;
  private final ActivityMenu menu = new ActivityMenu(this);
  private LinearLayout root, body;
  private String tab = "Journée",
      page = "",
      copyId = "",
      email = "",
      answer = "",
      typed = "",
      notice = "",
      photoFile = "",
      cameraFile = "";
  private int index = 0, count = 0, requestedHour = 19;
  private long end = 0;
  private boolean busy, saved;
  private ArrayList<String> sessionIds = new ArrayList<>();
  private TextView timer;
  private String mathTemplate;
  private boolean foreground;
  private boolean courseAllowed, tutorAllowed, liveAllowed;
  private MemberAPI pollRequest;
  private final Runnable poll = this::pollCopies;

  private boolean shouldPoll() {
    List<String> stages = new ArrayList<>();
    if (repo != null && repo.allows("tutor") && repo.snapshot != null)
      for (JSONObject row : Repository.list(repo.snapshot.optJSONArray("submissions"))) {
        JSONObject progress = row.optJSONObject("correctionProgress");
        if (progress != null) stages.add(progress.optString("stage"));
      }
    return Practice.shouldPoll(
        "Copies".equals(tab), foreground, repo != null && repo.session != null, stages);
  }

  private void schedulePoll() {
    handler.removeCallbacks(poll);
    if (!shouldPoll()) {
      if (pollRequest != null) {
        pollRequest.cancel();
        pollRequest = null;
      }
    } else if (!busy && pollRequest == null) {
      handler.postDelayed(poll, 4000);
    }
  }

  private void pollCopies() {
    if (!shouldPoll() || busy) {
      schedulePoll();
      return;
    }
    final MemberAPI net;
    try {
      net = repo.api();
    } catch (Exception e) {
      return;
    }
    pollRequest = net;
    worker.execute(
        () -> {
          JSONArray data = null;
          Exception error = null;
          try {
            data = net.json("GET", ApiPath.getSubmissions, null).getJSONArray("items");
          } catch (Exception e) {
            error = e;
          }
          final JSONArray fresh = data;
          final Exception failure = error;
          runOnUiThread(
              () -> {
                if (pollRequest != net || isDestroyed()) return;
                pollRequest = null;
                if (!shouldPoll() || busy) {
                  schedulePoll();
                  return;
                }
                if (failure instanceof MemberAPI.Failure f && f.status == 401) {
                  sync();
                  return;
                }
                if (failure != null) {
                  notice = "Actualisation indisponible. Le dernier état reçu reste affiché.";
                  render();
                  return;
                }
                try {
                  if (!fresh
                      .toString()
                      .equals(repo.snapshot.optJSONArray("submissions").toString())) {
                    repo.snapshot.put("submissions", fresh);
                    repo.db.put(repo.owner(), "snapshot", "current", repo.snapshot, null);
                    notice = "";
                    render();
                  } else schedulePoll();
                } catch (Exception e) {
                  notice = "Impossible d’enregistrer le suivi.";
                  render();
                }
              });
        });
  }

  private final Runnable tick =
      new Runnable() {
        public void run() {
          if (repo != null
              && repo.session != null
              && (courseAllowed != repo.allows("course")
                  || tutorAllowed != repo.allows("tutor")
                  || liveAllowed != repo.allows("live"))) {
            render();
          }
          if (end > 0 && timer != null) {
            long s = Math.max(0, (end - System.currentTimeMillis() + 999) / 1000);
            timer.setText(String.format(Locale.FRANCE, "Séance · %d min %02d s", s / 60, s % 60));
            if (s == 0) {
              timer = null;
              render();
            }
          }
          handler.postDelayed(this, 1000);
        }
      };
  private final Runnable retry =
      new Runnable() {
        public void run() {
          billing.retry();
          if (repo != null && repo.session != null && !busy && repo.hasPending()) sync();
          handler.postDelayed(this, 60000);
        }
      };

  @Override
  public void onCreate(Bundle state) {
    super.onCreate(state);
    getOnBackPressedDispatcher().addCallback(this, back);
    getWindow().setStatusBarColor(INK);
    getWindow().setNavigationBarColor(INK);
    if (state != null) {
      tab = state.getString("tab", tab);
      page = state.getString("page", "");
      copyId = state.getString("copy", "");
      typed = state.getString("text", "");
      answer = state.getString("answer", "");
      photoFile = state.getString("photo", "");
      cameraFile = state.getString("camera", "");
      email = state.getString("email", "");
      sessionIds = state.getStringArrayList("ids");
      if (sessionIds == null) sessionIds = new ArrayList<>();
      end = state.getLong("end");
      index = state.getInt("index");
      count = state.getInt("count");
      saved = state.getBoolean("saved");
    }
    try {
      repo = new Repository(getApplicationContext());
    } catch (Exception e) {
      notice = "Impossible d’ouvrir la mémoire sécurisée de cet appareil.";
    }
    render();
    handleLink(getIntent());
  }

  @Override
  protected void onSaveInstanceState(Bundle out) {
    super.onSaveInstanceState(out);
    out.putString("tab", tab);
    out.putString("page", page);
    out.putString("copy", copyId);
    out.putString("text", typed);
    out.putString("answer", answer);
    out.putString("photo", photoFile);
    out.putString("camera", cameraFile);
    out.putString("email", email);
    out.putStringArrayList("ids", sessionIds);
    out.putLong("end", end);
    out.putInt("index", index);
    out.putInt("count", count);
    out.putBoolean("saved", saved);
  }

  @Override
  protected void onResume() {
    super.onResume();
    foreground = true;
    billing.resume();
    menu.resume();
    schedulePoll();
    handler.post(tick);
    handler.postDelayed(retry, 60000);
    if (repo != null && repo.session != null && !busy) sync();
  }

  @Override
  protected void onDestroy() {
    billing.stop();
    super.onDestroy();
  }

  @Override
  protected void onPause() {
    foreground = false;
    schedulePoll();
    handler.removeCallbacks(tick);
    handler.removeCallbacks(retry);
    menu.pause();
    super.onPause();
  }

  @Override
  protected void onNewIntent(Intent intent) {
    super.onNewIntent(intent);
    setIntent(intent);
    handleLink(intent);
  }

  private void handleLink(Intent intent) {
    Uri uri = intent.getData();
    if (uri == null
        || repo == null
        || repo.session != null
        || !"crpemaths".equals(uri.getScheme())
        || !"auth".equals(uri.getEncodedAuthority())
        || !("".equals(uri.getPath()) || "/".equals(uri.getPath()))
        || uri.getEncodedFragment() == null) return;
    String token =
        Uri.parse("https://local.invalid/?" + uri.getEncodedFragment()).getQueryParameter("token");
    intent.setData(null);
    if (token != null && token.matches("[A-Za-z0-9_-]{16,256}"))
      task(
          () -> repo.login(token),
          () -> {
            tab = "Journée";
            billing.resume();
            sync();
          });
  }

  interface Work {
    void run() throws Exception;
  }

  void task(Work work, Runnable done) {
    if (busy) return;
    busy = true;
    notice = "";
    render();
    worker.execute(
        () -> {
          Exception failure = null;
          try {
            work.run();
          } catch (Exception e) {
            if (BuildConfig.DEBUG)
              android.util.Log.w(
                  "PrepaCRPE", "Request or storage error: " + e.getClass().getSimpleName());
            failure = e;
          }
          final Exception error = failure;
          runOnUiThread(
              () -> {
                if (isDestroyed() || isFinishing()) return;
                busy = false;
                if (error != null) notice = MemberAPI.message(error);
                else if (done != null) done.run();
                render();
              });
        });
  }

  void refreshMember() {
    if (!busy) sync();
  }

  private void sync() {
    task(() -> repo.sync(), null);
  }

  private int dp(int n) {
    return Math.round(n * getResources().getDisplayMetrics().density);
  }

  TextView label(String value, int sp) {
    TextView v = new TextView(this);
    v.setText(value);
    v.setTextSize(sp);
    v.setTextColor(INK);
    v.setPadding(0, dp(6), 0, dp(5));
    body.addView(v);
    return v;
  }

  void title(String value) {
    TextView v = label(value, 32);
    v.setTypeface(Typeface.SERIF, Typeface.BOLD);
    v.setAccessibilityHeading(true);
  }

  void heading(String value) {
    TextView v = label(value, 20);
    v.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
    v.setAccessibilityHeading(true);
  }

  Button button(String title, boolean primary, Runnable action) {
    Button b = new Button(this);
    b.setAllCaps(false);
    b.setText(title);
    b.setTextSize(16);
    b.setMinHeight(dp(48));
    b.setEnabled(!busy);
    b.setTextColor(primary ? WHITE : INK);
    GradientDrawable bg = new GradientDrawable();
    bg.setCornerRadius(dp(10));
    bg.setColor(primary ? INK : WHITE);
    bg.setStroke(dp(1), 0xffbfcabb);
    b.setBackground(bg);
    LinearLayout.LayoutParams p = new LinearLayout.LayoutParams(-1, -2);
    p.topMargin = dp(6);
    body.addView(b, p);
    b.setOnClickListener(v -> action.run());
    return b;
  }

  interface Change {
    void set(String s);
  }

  EditText field(String hint, String value, int type, Change change) {
    EditText input = new EditText(this);
    input.setHint(hint);
    input.setInputType(type);
    input.setText(value);
    input.setEnabled(!busy);
    input.setContentDescription(hint);
    input.setTextColor(INK);
    input.setHintTextColor(0xff59665c);
    input.setMinHeight(dp(48));
    body.addView(input);
    input.addTextChangedListener(
        new TextWatcher() {
          public void beforeTextChanged(CharSequence s, int a, int c, int f) {}

          public void onTextChanged(CharSequence s, int start, int before, int count) {
            change.set(s.toString());
          }

          public void afterTextChanged(Editable e) {}
        });
    return input;
  }

  void render() {
    back.setEnabled(repo != null && repo.session != null && !tab.equals("Journée"));
    if (repo != null) {
      courseAllowed = repo.allows("course");
      tutorAllowed = repo.allows("tutor");
      liveAllowed = repo.allows("live");
    }
    schedulePoll();
    timer = null;
    root = new LinearLayout(this);
    root.setOrientation(LinearLayout.VERTICAL);
    root.setBackgroundColor(PAPER);
    root.setFocusableInTouchMode(true);
    setContentView(root);
    root.setOnApplyWindowInsetsListener(
        (v, insets) -> {
          if (Build.VERSION.SDK_INT >= 30) {
            android.graphics.Insets s =
                insets.getInsets(
                    WindowInsets.Type.systemBars() | WindowInsets.Type.displayCutout());
            v.setPadding(s.left, s.top, s.right, s.bottom);
          }
          return insets;
        });
    ScrollView scroll = new ScrollView(this);
    root.addView(scroll, new LinearLayout.LayoutParams(-1, 0, 1));
    body = new LinearLayout(this);
    body.setOrientation(LinearLayout.VERTICAL);
    body.setPadding(dp(20), dp(22), dp(20), dp(24));
    scroll.addView(body);
    heading("Prépa Maths CRPE");
    if (!notice.isEmpty()) label(notice, 15);
    if (busy) {
      ProgressBar p = new ProgressBar(this);
      body.addView(p, new LinearLayout.LayoutParams(dp(28), dp(28)));
    }
    if (repo == null) return;
    if (repo.session == null) {
      title("Mon espace de révision");
      label("Recevez un lien par e-mail pour retrouver votre parcours et vos exercices.", 17);
      field(
          "Adresse e-mail",
          email,
          android.text.InputType.TYPE_CLASS_TEXT
              | android.text.InputType.TYPE_TEXT_VARIATION_EMAIL_ADDRESS,
          s -> email = s);
      button(
          "Recevoir mon lien",
          true,
          () -> {
            if (email.trim().isEmpty()) {
              toast("Saisissez votre adresse e-mail.");
              return;
            }
            task(
                () -> repo.request(email),
                () ->
                    notice =
                        "Consultez votre messagerie sur ce téléphone et ouvrez le lien de"
                            + " connexion.");
          });
      return;
    }
    String required = tabKey(tab);
    if (required != null && !repo.allows(required)) menu.locked(required);
    else
      switch (tab) {
        case "Activités" -> menu.show();
        case "Parcours" -> course();
        case "Copies" -> copies();
        case "Compte" -> account();
        default -> daily();
      }
    LinearLayout tabs = new LinearLayout(this);
    root.addView(tabs);
    for (String t : new String[] {"Journée", "Parcours", "Copies", "Activités", "Compte"}) {
      Button b = new Button(this);
      b.setAllCaps(false);
      b.setPadding(0, 0, 0, 0);
      b.setTextSize(13);
      b.setText(t);
      b.setSelected(t.equals(tab));
      b.setEnabled(!busy);
      b.setTextColor(
          t.equals(tab)
              ? WHITE
              : (tabKey(t) != null && repo.feature(tabKey(t)) != null && !repo.allows(tabKey(t)))
                  ? 0xff65736a
                  : INK);
      b.setBackgroundColor(t.equals(tab) ? INK : WHITE);
      tabs.addView(b, new LinearLayout.LayoutParams(0, dp(52), 1));
      b.setOnClickListener(
          v -> {
            if (!tab.equals(t)) menu.leave();
            if (!t.equals(tab)) {
              accountPurchases = false;
              billing.leave();
            }
            tab = t;
            notice = "";
            render();
          });
    }
  }

  private String tabKey(String name) {
    return switch (name) {
      case "Journée" -> "practice";
      case "Copies" -> "tutor";
      case "Parcours" -> "course";
      default -> null;
    };
  }

  void jump(String target) {
    menu.leave();
    billing.leave();
    accountPurchases = false;
    tab = target;
    render();
  }

  void toast(String v) {
    Toast.makeText(this, v, Toast.LENGTH_LONG).show();
  }

  private JSONObject content(String id) {
    for (JSONObject i : repo.catalog()) if (i.optString("id").equals(id)) return i;
    return null;
  }

  void math(String value) {
    if (value == null || value.isEmpty()) return;
    if (!value.contains("$")) {
      label(value, 17);
      return;
    }
    try {
      if (mathTemplate == null) {
        try (InputStream a = getAssets().open("math.html");
            InputStream b = getAssets().open("katex.min.txt")) {
          mathTemplate = readText(a).replace("/*KATEX*/", readText(b));
        }
      }
      WebView w = new WebView(this);
      w.setBackgroundColor(PAPER);
      w.getSettings().setAllowFileAccess(false);
      w.getSettings().setAllowContentAccess(false);
      w.getSettings().setBlockNetworkLoads(true);
      w.getSettings().setJavaScriptEnabled(true);
      w.setVerticalScrollBarEnabled(false);
      String html =
          mathTemplate.replace(
              "PAYLOAD_BASE64",
              android.util.Base64.encodeToString(
                  value.getBytes(StandardCharsets.UTF_8), android.util.Base64.NO_WRAP));
      body.addView(w, new LinearLayout.LayoutParams(-1, -2));
      w.loadDataWithBaseURL(null, html, "text/html", "UTF-8", null);
    } catch (Exception e) {
      label(value, 17);
    }
  }

  private static String readText(InputStream stream) throws IOException {
    try (ByteArrayOutputStream bytes = new ByteArrayOutputStream()) {
      byte[] b = new byte[8192];
      int n;
      while ((n = stream.read(b)) != -1) bytes.write(b, 0, n);
      return bytes.toString("UTF-8");
    }
  }

  private void daily() {
    title("Ma journée");
    if (end > 0) {
      JSONObject item = index < sessionIds.size() ? content(sessionIds.get(index)) : null;
      if (item == null || System.currentTimeMillis() >= end) {
        heading("Séance terminée");
        label("Réponses enregistrées · " + count, 18);
        button(
            "Revenir à ma journée",
            true,
            () -> {
              end = 0;
              render();
            });
        return;
      }
      timer = label("Entraînement", 16);
      heading(item.optString("title"));
      math(item.optString("body"));
      if (saved) {
        JSONObject result = null;
        for (JSONObject row : Repository.list(repo.answers))
          if (item.optString("id").equals(row.optJSONObject("input").optString("exerciseId")))
            result = row.optJSONObject("receipt");
        if (result == null)
          label("Réponse enregistrée. La correction apparaîtra après synchronisation.", 16);
        else {
          label(result.optBoolean("correct") ? "Réponse juste." : "À retravailler.", 17);
          math(result.optString("feedback"));
        }
        button(
            "Continuer",
            true,
            () -> {
              index++;
              answer = "";
              saved = false;
              render();
            });
      } else {
        if ("choice".equals(item.optString("format"))) {
          JSONArray choices = item.optJSONArray("choices");
          if (choices != null)
            for (int i = 0; i < choices.length(); i++) {
              String value = choices.optString(i);
              button(
                  value,
                  value.equals(answer),
                  () -> {
                    answer = value;
                    render();
                  });
            }
        } else
          field(
              "Réponse numérique", answer, android.text.InputType.TYPE_CLASS_TEXT, s -> answer = s);
        button(
            "Valider ma réponse",
            true,
            () -> {
              if (answer.trim().isEmpty()) {
                toast("Saisissez votre réponse.");
                return;
              }
              if (System.currentTimeMillis() >= end) {
                render();
                return;
              }
              try {
                repo.queueAnswer(item.optString("id"), answer);
                saved = true;
                count++;
                sync();
              } catch (Exception e) {
                toast("Impossible d’enregistrer la réponse sur cet appareil.");
              }
            });
      }
      button(
          "Terminer la séance",
          false,
          () -> {
            end = 1;
            render();
          });
      return;
    }
    heading("Cinq minutes de maths");
    if (repo.snapshot == null) {
      label("Connectez-vous au service pour télécharger votre première séance.", 17);
      button("Télécharger le parcours", true, this::sync);
      return;
    }
    List<JSONObject> due = repo.due();
    if (due.isEmpty())
      label(
          repo.catalog().stream()
                  .anyMatch(
                      i ->
                          "exercise".equals(i.optString("kind"))
                              && !"text".equals(i.optString("format")))
              ? "Vos exercices reviendront à leur prochaine échéance."
              : "Les exercices apparaîtront après validation par votre enseignante.",
          17);
    else {
      label("Exercices à découvrir ou à revoir · " + due.size(), 17);
      button(
          "Commencer ma séance",
          true,
          () -> {
            sessionIds.clear();
            for (JSONObject item : due) sessionIds.add(item.optString("id"));
            index = 0;
            count = 0;
            saved = false;
            answer = "";
            end = System.currentTimeMillis() + 300000;
            render();
          });
    }
    JSONObject p = repo.snapshot == null ? null : repo.snapshot.optJSONObject("progress");
    if (p != null) {
      heading("Ma progression");
      label(
          p.optInt("streak")
              + " jours de suite · "
              + p.optInt("correct")
              + " réponses justes sur "
              + p.optInt("attempts"),
          17);
      for (JSONObject s : Repository.list(p.optJSONArray("skills")))
        label(s.optString("skill") + " · " + date(s.optString("nextReviewAt")), 15);
    }
    List<JSONObject> rows = Repository.list(repo.answers);
    if (!rows.isEmpty()) heading("Dernières réponses");
    for (int i = rows.size() - 1; i >= Math.max(0, rows.size() - 8); i--) {
      JSONObject a = rows.get(i),
          input = a.optJSONObject("input"),
          q = content(input.optString("exerciseId")),
          r = a.optJSONObject("receipt");
      label(
          (q == null ? "Exercice" : q.optString("title"))
              + " · "
              + input.optString("answer")
              + " · "
              + (r != null
                  ? (r.optBoolean("correct") ? "Juste" : "À retravailler")
                  : a.has("rejection") ? "Réponse refusée" : "À synchroniser"),
          15);
      if (r != null) math(r.optString("feedback"));
    }
  }

  private String date(String s) {
    try {
      return Instant.parse(s)
          .atZone(ZoneId.systemDefault())
          .format(DateTimeFormatter.ofPattern("dd/MM/yyyy", Locale.FRANCE));
    } catch (Exception e) {
      return "";
    }
  }

  void external(String value) {
    Uri url = Uri.parse(value);
    if (!FeatureAccess.https(value)) return;
    try {
      startActivity(new Intent(Intent.ACTION_VIEW, url));
    } catch (ActivityNotFoundException ex) {
      toast("Aucune application ne peut ouvrir ce lien.");
    }
  }

  private void course() {
    JSONObject item = content(page);
    title(item == null ? "Mon parcours" : item.optString("title"));
    if (item != null) {
      button(
          "Retour au parcours",
          false,
          () -> {
            page = item.optString("parentId", "");
            render();
          });
      math(item.optString("body"));
      if (!item.optString("videoDescription").isEmpty()) math(item.optString("videoDescription"));
      if (!item.optString("videoUrl").isEmpty())
        button("Voir la vidéo du cours", false, () -> external(item.optString("videoUrl")));
      if (!item.optString("sourceTitle").isEmpty()) label(item.optString("sourceTitle"), 15);
      if (!item.optString("sourceUrl").isEmpty())
        button("Consulter la source", false, () -> external(item.optString("sourceUrl")));
      for (JSONObject doc : Repository.list(item.optJSONArray("documents")))
        button(doc.optString("title"), false, () -> shareDocument(doc));
      if (repo.exercises().stream().anyMatch(i -> i.optString("id").equals(item.optString("id"))))
        button(
            "Envoyer ma copie",
            true,
            () -> {
              copyId = page;
              clearDraft();
              tab = "Copies";
              render();
            });
    }
    int n = 0;
    for (JSONObject child : repo.catalog())
      if (item == null
          ? "chapter".equals(child.optString("kind"))
          : page.equals(child.optString("parentId"))) {
        n++;
        button(
            child.optString("title"),
            false,
            () -> {
              page = child.optString("id");
              render();
            });
      }
    if (n == 0 && item == null)
      label("Les chapitres validés par votre enseignante apparaîtront ici.", 17);
  }

  private void shareDocument(JSONObject doc) {
    final Uri[] uri = new Uri[1];
    String mime = doc == null ? "application/json" : "application/pdf";
    task(
        () -> {
          String owner = repo.owner(),
              name =
                  doc == null
                      ? "mes-donnees-crpe.json"
                      : UUID.fromString(doc.getString("id")) + ".pdf";
          File file = new File(LocalFiles.folder(this, owner), name);
          if (doc == null || !file.exists()) {
            byte[] bytes =
                repo.api()
                    .file(
                        doc == null
                            ? ApiPath.exportMember
                            : ApiPath.downloadDocument.replace("{id}", doc.getString("id")));
            File tmp = File.createTempFile("download-", ".tmp", file.getParentFile());
            try {
              Files.write(tmp.toPath(), bytes);
              Files.move(
                  tmp.toPath(), file.toPath(), java.nio.file.StandardCopyOption.REPLACE_EXISTING);
            } finally {
              tmp.delete();
            }
          }
          uri[0] = LocalFiles.uri(this, owner, name);
        },
        () -> {
          Intent intent =
              doc == null
                  ? new Intent(Intent.ACTION_SEND)
                      .setType(mime)
                      .putExtra(Intent.EXTRA_STREAM, uri[0])
                  : new Intent(Intent.ACTION_VIEW).setDataAndType(uri[0], mime);
          intent.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
          intent.setClipData(ClipData.newRawUri("Fichier CRPE", uri[0]));
          try {
            startActivity(Intent.createChooser(intent, "Ouvrir le fichier"));
          } catch (Exception e) {
            toast("Aucune application compatible.");
          }
        });
  }

  private void clearDraft() {
    typed = "";
    if (!photoFile.isEmpty()) new File(photoFile).delete();
    photoFile = "";
  }

  private void copies() {
    title("Mes copies");
    heading("Déposer une copie");
    JSONObject exercise = content(copyId);
    if (exercise == null) {
      boolean any = false;
      for (JSONObject i : repo.exercises())
        if ("exercise".equals(i.optString("kind"))) {
          any = true;
          button(
              i.optString("title"),
              false,
              () -> {
                copyId = i.optString("id");
                render();
              });
        }
      if (!any) label("Vous pourrez déposer une copie dès la publication d’un exercice.", 17);
    } else {
      heading(exercise.optString("title"));
      math(exercise.optString("body"));
      EditText text =
          field(
              "Votre réponse ou une précision",
              typed,
              android.text.InputType.TYPE_CLASS_TEXT
                  | android.text.InputType.TYPE_TEXT_FLAG_MULTI_LINE,
              s -> typed = s);
      text.setFilters(new InputFilter[] {new InputFilter.LengthFilter(12000)});
      if (!photoFile.isEmpty()) {
        ImageView img = new ImageView(this);
        img.setImageBitmap(BitmapFactory.decodeFile(photoFile));
        img.setContentDescription("Aperçu de la copie");
        img.setScaleType(ImageView.ScaleType.FIT_CENTER);
        body.addView(img, new LinearLayout.LayoutParams(-1, dp(155)));
      }
      button(
          "Choisir une photo",
          false,
          () -> {
            Intent pick =
                new Intent(Intent.ACTION_OPEN_DOCUMENT)
                    .addCategory(Intent.CATEGORY_OPENABLE)
                    .setType("image/*");
            try {
              startActivityForResult(pick, 31);
            } catch (ActivityNotFoundException e) {
              toast("Aucun sélecteur de photos disponible.");
            }
          });
      button(
          "Prendre une photo",
          false,
          () -> {
            try {
              String name = UUID.randomUUID() + ".jpg";
              File f = new File(LocalFiles.folder(this, repo.owner()), name);
              if (!f.createNewFile()) throw new IOException();
              cameraFile = f.getPath();
              Uri target = LocalFiles.uri(this, repo.owner(), name);
              Intent capture =
                  new Intent(MediaStore.ACTION_IMAGE_CAPTURE)
                      .putExtra(MediaStore.EXTRA_OUTPUT, target)
                      .addFlags(
                          Intent.FLAG_GRANT_WRITE_URI_PERMISSION
                              | Intent.FLAG_GRANT_READ_URI_PERMISSION);
              capture.setClipData(ClipData.newRawUri("Copie", target));
              startActivityForResult(capture, 32);
            } catch (Exception e) {
              if (!cameraFile.isEmpty()) new File(cameraFile).delete();
              cameraFile = "";
              toast("Aucun appareil photo disponible. Choisissez une image.");
            }
          });
      button(
          "Transmettre ma copie",
          true,
          () -> {
            if (typed.trim().isEmpty() && photoFile.isEmpty()) {
              toast("Ajoutez une réponse ou une photo.");
              return;
            }
            try {
              repo.queueCopy(
                  copyId,
                  typed,
                  photoFile.isEmpty() ? null : Files.readAllBytes(new File(photoFile).toPath()));
              clearDraft();
              copyId = "";
              sync();
            } catch (Exception e) {
              toast("Impossible d’enregistrer la copie sur cet appareil.");
            }
          });
      button(
          "Changer d’exercice",
          false,
          () -> {
            clearDraft();
            copyId = "";
            render();
          });
    }
    heading("Suivi des copies");
    for (JSONObject c : Repository.list(repo.copies)) {
      JSONObject q = content(c.optString("exerciseId"));
      label(
          (q == null ? "Copie" : q.optString("title"))
              + " · "
              + (c.has("rejection") ? "Envoi refusé" : "À synchroniser"),
          15);
    }
    for (JSONObject s :
        Repository.list(repo.snapshot == null ? null : repo.snapshot.optJSONArray("submissions"))) {
      JSONObject q = content(s.optString("exerciseId"));
      heading(q == null ? "Copie" : q.optString("title"));
      JSONObject progress = s.optJSONObject("correctionProgress");
      if (progress != null) {
        label(Practice.correctionLabel(progress.optString("stage")), 16);
        try {
          label(
              "Dernière mise à jour · "
                  + Instant.parse(progress.getString("updatedAt"))
                      .atZone(ZoneId.systemDefault())
                      .format(DateTimeFormatter.ofPattern("dd/MM/yyyy HH:mm", Locale.FRANCE)),
              14);
        } catch (Exception ignored) {
        }
        if ("retrying".equals(progress.optString("stage")))
          label("Essais de traitement · " + progress.optInt("attempts"), 14);
      } else label("État de correction indisponible", 16);
      JSONObject correction = s.optJSONObject("correction");
      if (correction != null) {
        label(
            correction.optBoolean("generatedByAI")
                ? "Généré par IA"
                : "uncertain".equals(correction.optString("verdict"))
                    ? "À relire par l’enseignante"
                    : "Vérification numérique",
            15);
        math(correction.optString("summary"));
        for (JSONObject err : Repository.list(correction.optJSONArray("errors")))
          math(err.optString("location") + " · " + err.optString("message"));
        JSONArray writing = correction.optJSONArray("writingFeedback");
        if (writing != null) for (int i = 0; i < writing.length(); i++) math(writing.optString(i));
        if (!correction.optString("referenceSolution").isEmpty()) {
          heading("Corrigé de référence");
          math(correction.optString("referenceSolution"));
        }
        String similar = correction.optString("similarExerciseId", "");
        if (content(similar) != null)
          button(
              "Ouvrir l’exercice conseillé",
              false,
              () -> {
                tab = "Parcours";
                page = similar;
                render();
              });
      }
      math(s.optString("feedback"));
      if (!s.optString("reviewVideoUrl").isEmpty())
        button("Voir le retour vidéo", false, () -> external(s.optString("reviewVideoUrl")));
    }
  }

  @Override
  protected void onActivityResult(int request, int result, Intent data) {
    super.onActivityResult(request, result, data);
    if (request != 31 && request != 32) return;
    String camera = cameraFile;
    cameraFile = "";
    if (result != RESULT_OK) {
      if (request == 32 && !camera.isEmpty()) new File(camera).delete();
      return;
    }
    Uri uri =
        request == 31 && data != null
            ? data.getData()
            : request == 32 && !camera.isEmpty() ? Uri.fromFile(new File(camera)) : null;
    if (uri == null) return;
    task(
        () -> {
          try {
            ImageDecoder.Source source = ImageDecoder.createSource(getContentResolver(), uri);
            Bitmap bitmap =
                ImageDecoder.decodeBitmap(
                    source,
                    (decoder, info, src) -> {
                      int width = info.getSize().getWidth(), height = info.getSize().getHeight();
                      float scale = Math.min(1f, 2400f / Math.max(width, height));
                      decoder.setTargetSize(
                          Math.max(1, Math.round(width * scale)),
                          Math.max(1, Math.round(height * scale)));
                      decoder.setAllocator(ImageDecoder.ALLOCATOR_SOFTWARE);
                    });
            File file = new File(LocalFiles.folder(this, repo.owner()), UUID.randomUUID() + ".jpg");
            try (FileOutputStream out = new FileOutputStream(file)) {
              if (!bitmap.compress(Bitmap.CompressFormat.JPEG, 85, out))
                throw new IOException("Encoding");
            } finally {
              bitmap.recycle();
            }
            if (file.length() == 0 || file.length() > 8 * 1024 * 1024) {
              file.delete();
              throw new MemberAPI.Failure(413);
            }
            if (!photoFile.isEmpty()) new File(photoFile).delete();
            photoFile = file.getPath();
          } finally {
            if (request == 32 && !camera.isEmpty()) new File(camera).delete();
          }
        },
        null);
  }

  private void account() {
    if (accountPurchases) {
      button(
          "Revenir à mon compte",
          false,
          () -> {
            accountPurchases = false;
            billing.leave();
            render();
          });
      billing.show("");
      return;
    }
    title("Mon compte");
    label(repo.session.optJSONObject("user").optString("email"), 17);
    if (repo.snapshot != null)
      label("Dernière synchronisation · " + date(repo.snapshot.optString("updated")), 15);
    button("Synchroniser maintenant", true, this::sync);
    button(
        "Mon abonnement",
        false,
        () -> {
          accountPurchases = true;
          render();
        });
    heading("Rappel quotidien");
    int active = Reminder.hour(this);
    label(
        active < 0 ? "Le rappel est désactivé." : "Rappel actif à " + active + " h, heure locale.",
        16);
    button(
        "Choisir une heure",
        false,
        () ->
            new TimePickerDialog(
                    this,
                    (picker, h, m) -> {
                      requestedHour = h;
                      if (Build.VERSION.SDK_INT >= 33
                          && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS)
                              != PackageManager.PERMISSION_GRANTED) {
                        requestPermissions(
                            new String[] {Manifest.permission.POST_NOTIFICATIONS}, 33);
                      } else {
                        Reminder.change(this, h);
                        render();
                      }
                    },
                    active < 0 ? 19 : active,
                    0,
                    true)
                .show());
    if (active >= 0)
      button(
          "Désactiver le rappel",
          false,
          () -> {
            Reminder.change(this, -1);
            render();
          });
    heading("Mes données");
    button("Exporter mes données", false, () -> shareDocument(null));
    button("Me déconnecter", false, () -> confirmExit(false));
    button("Supprimer mon compte", false, () -> confirmExit(true));
  }

  private void confirmExit(boolean deleting) {
    new AlertDialog.Builder(this)
        .setTitle(deleting ? "Supprimer définitivement mon compte ?" : "Me déconnecter ?")
        .setMessage(
            deleting
                ? "Votre compte, vos réponses et vos copies seront supprimés."
                : "Les envois en attente et les données téléchargées seront effacés de cet"
                    + " appareil.")
        .setNegativeButton("Annuler", null)
        .setPositiveButton(
            deleting ? "Supprimer" : "Me déconnecter",
            (d, w) ->
                task(
                    () -> {
                      String old = repo.owner();
                      repo.logout(deleting);
                      File directory = LocalFiles.folder(this, old);
                      File[] files = directory.listFiles();
                      if (files != null) for (File file : files) file.delete();
                      directory.delete();
                      Reminder.change(this, -1);
                    },
                    () -> {
                      billing.stop();
                      accountPurchases = false;
                      clearDraft();
                      tab = "Journée";
                      copyId = "";
                      page = "";
                      end = 0;
                      email = "";
                    }))
        .show();
  }

  @Override
  public void onRequestPermissionsResult(int req, String[] permissions, int[] results) {
    super.onRequestPermissionsResult(req, permissions, results);
    if (req == 33) {
      if (results.length > 0 && results[0] == PackageManager.PERMISSION_GRANTED)
        Reminder.change(this, requestedHour);
      else notice = "Autorisez les notifications dans les réglages du téléphone.";
      render();
    }
  }

  private void backInMemberArea() {
    if (tab.equals("Compte") && accountPurchases) {
      accountPurchases = false;
      billing.leave();
      render();
      return;
    }
    if (tab.equals("Activités") && menu.back()) {
      render();
    } else if (tab.equals("Parcours") && !page.isEmpty()) {
      JSONObject i = content(page);
      page = i == null ? "" : i.optString("parentId", "");
      render();
    } else if (!tab.equals("Journée")) {
      tab = "Journée";
      render();
    }
  }
}
