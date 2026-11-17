"use client";
import type { components } from "@crpe/api-client";
import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { browserApi } from "../lib/browser-api";
import { checked, result } from "../lib/model";
import { ActionForm, field } from "./Form";
import { Markdown } from "./Markdown";
import { Recorder } from "./Recorder";

type Session = components["schemas"]["OralSession"];
export function StartOral() {
  const router = useRouter();
  return (
    <ActionForm
      label="Tirer un sujet"
      submit={async () => {
        const session = result(
          await browserApi().POST("/v1/oral-sessions", { body: {} }),
        );
        router.push(`/espace/oral/${session.id}`);
      }}
    />
  );
}
function Listen({
  session,
  index,
  question,
}: {
  session: string;
  index: number;
  question: string;
}) {
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const caption = question
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replace(/\s+/g, " ");
  const track = `data:text/vtt;charset=utf-8,${encodeURIComponent(`WEBVTT\n\n00:00:00.000 --> 00:30:00.000\n${caption}\n`)}`;
  useEffect(
    () => () => {
      if (url) URL.revokeObjectURL(url);
    },
    [url],
  );
  async function load() {
    setBusy(true);
    setError("");
    try {
      const reply = await fetch(`/api/v1/oral-sessions/${session}/speech`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ index }),
      });
      checked(reply);
      setUrl(URL.createObjectURL(await reply.blob()));
    } catch {
      setError(
        "La lecture audio est indisponible. La question est affichée ci-dessus.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="audio-controls">
      {url ? (
        <audio controls src={url} aria-label="Écouter la question">
          <track
            kind="captions"
            srcLang="fr"
            label="Question du jury"
            src={track}
          />
        </audio>
      ) : (
        <button
          type="button"
          className="button secondary"
          disabled={busy}
          onClick={load}
        >
          {busy ? "Chargement…" : "Écouter la question"}
        </button>
      )}
      {error && <p role="alert">{error}</p>}
    </div>
  );
}

export function OralFeedback({
  items,
  source,
}: {
  items: Session["feedback"];
  source: Session["feedbackSource"];
}) {
  return (
    <>
      <p className="overline">
        {source === "ai" ? "Généré par IA" : "Évaluation de l’enseignante"}
      </p>
      <ul className="plain-list">
        {items.map((i) => (
          <li key={i.criterion}>
            <div>
              <strong>{i.criterion}</strong>
              <p>{i.comment}</p>
            </div>
            <span
              className={`status ${i.status === "validated" ? "validated" : ""}`}
            >
              {i.status === "validated" ? "Validé" : "À travailler"}
            </span>
          </li>
        ))}
      </ul>
    </>
  );
}
export function OralPractice({ initial }: { initial: Session }) {
  const [session, setSession] = useState(initial);
  const [transcript, setTranscript] = useState(initial.transcript);
  const [answers, setAnswers] = useState(
    initial.questions.map((_, i) => initial.answers[i] ?? ""),
  );
  const [preparing, setPreparing] = useState(true);
  const [now, setNow] = useState<number>();
  useEffect(() => {
    if (!preparing || session.status !== "active") return;
    const tick = () => setNow(Date.now());
    tick();
    const id = window.setInterval(tick, 1000);
    return () => clearInterval(id);
  }, [preparing, session.status]);
  const secs = Math.max(
    0,
    Math.ceil(
      (Date.parse(session.createdAt) +
        session.prepMinutes * 60000 -
        (now ?? Date.parse(session.createdAt))) /
        1000,
    ),
  );
  if (session.status !== "active")
    return (
      <section className="panel">
        <h2>
          {session.status === "reviewed"
            ? "Retour par critère"
            : "Oral remis pour relecture"}
        </h2>
        {session.status === "reviewed" ? (
          <OralFeedback
            items={session.feedback}
            source={session.feedbackSource}
          />
        ) : (
          <p>Le retour par critère sera disponible après relecture.</p>
        )}
        <details>
          <summary>Relire ma transcription</summary>
          <p className="preserve">{session.transcript}</p>
        </details>
      </section>
    );
  return (
    <>
      <article className="panel">
        <Markdown>{session.body}</Markdown>
        <h3>Critères travaillés</h3>
        <ul>
          {session.criteria.map((c) => (
            <li key={c}>{c}</li>
          ))}
        </ul>
      </article>
      {preparing && secs > 0 ? (
        <section className="panel">
          <h2>Temps de préparation</h2>
          <p>
            {Math.floor(secs / 60)} min {secs % 60} s restantes
          </p>
          <button
            className="button"
            type="button"
            onClick={() => setPreparing(false)}
          >
            Commencer mon exposé
          </button>
        </section>
      ) : (
        <section className="panel">
          <h2>Mon exposé</h2>
          <p>
            Durée prévue : {session.durationMinutes} minutes. Relisez la
            transcription avant l’envoi.
          </p>
          <ActionForm
            label="Remettre ma prestation"
            submit={async () => {
              setSession(
                result(
                  await browserApi().POST("/v1/oral-sessions/{id}/finish", {
                    params: { path: { id: session.id } },
                    body: { transcript, answers },
                  }),
                ),
              );
            }}
          >
            <Recorder
              sessionId={session.id}
              onText={setTranscript}
              label="Enregistrer l’exposé"
              maxMinutes={session.durationMinutes}
            />
            <label>
              Transcription de l’exposé
              <textarea
                rows={6}
                value={transcript}
                onChange={(e) => setTranscript(e.target.value)}
                required
              />
            </label>
            {session.questions.map((question, i) => (
              <fieldset key={question}>
                <legend>{question}</legend>
                <Listen session={session.id} index={i} question={question} />
                <Recorder
                  sessionId={session.id}
                  onText={(text) =>
                    setAnswers((prev) =>
                      prev.map((value, j) => (j === i ? text : value)),
                    )
                  }
                  maxMinutes={5}
                  label="Enregistrer ma réponse"
                />
                <label>
                  Réponse à la question {i + 1}
                  <textarea
                    rows={3}
                    value={answers[i] ?? ""}
                    onChange={(e) =>
                      setAnswers((prev) =>
                        prev.map((value, j) =>
                          j === i ? e.target.value : value,
                        ),
                      )
                    }
                    required
                  />
                </label>
              </fieldset>
            ))}
          </ActionForm>
        </section>
      )}
    </>
  );
}
export function OralReview({ session }: { session: Session }) {
  return (
    <ActionForm
      label="Transmettre l’évaluation"
      refresh
      submit={async (form) => {
        const feedback = session.criteria.map((criterion, i) => ({
          criterion,
          status:
            field(form, `status-${i}`) === "validated"
              ? ("validated" as const)
              : ("to_work" as const),
          comment: field(form, `comment-${i}`),
        }));
        result(
          await browserApi().POST("/v1/admin/oral-sessions/{id}/review", {
            params: { path: { id: session.id } },
            body: { feedback },
          }),
        );
        return "Évaluation transmise.";
      }}
    >
      {session.criteria.map((criterion, i) => (
        <fieldset key={criterion}>
          <legend>{criterion}</legend>
          <label>
            Appréciation
            <select required name={`status-${i}`} defaultValue="">
              <option value="" disabled>
                Choisir
              </option>
              <option value="validated">Validé</option>
              <option value="to_work">À travailler</option>
            </select>
          </label>
          <label>
            Commentaire
            <textarea name={`comment-${i}`} required rows={2} />
          </label>
        </fieldset>
      ))}
    </ActionForm>
  );
}
