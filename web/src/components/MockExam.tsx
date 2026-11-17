"use client";
import type { components } from "@crpe/api-client";
import { useRouter } from "next/navigation";
import { useEffect, useRef, useState } from "react";
import { browserApi } from "../lib/browser-api";
import { result } from "../lib/model";
import { ActionForm, field } from "./Form";
import { Markdown } from "./Markdown";

type Session = components["schemas"]["MockSession"];
export function StartMock({ subjectId }: { subjectId: string }) {
  const router = useRouter();
  return (
    <ActionForm
      label="Commencer l’épreuve"
      submit={async () => {
        const session = result(
          await browserApi().POST("/v1/mock-sessions", { body: { subjectId } }),
        );
        router.push(`/espace/epreuves/${session.id}`);
      }}
    />
  );
}
function TimeLeft({ deadline }: { deadline: string }) {
  const [now, setNow] = useState<number>();
  useEffect(() => {
    setNow(Date.now());
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, []);
  const seconds = Math.max(
    0,
    Math.ceil(
      (new Date(deadline).getTime() - (now ?? new Date(deadline).getTime())) /
        1000,
    ),
  );
  return (
    <p className="feedback">
      <strong>
        {now === undefined
          ? "Épreuve en cours"
          : seconds === 0
            ? "Temps écoulé"
            : "Temps restant"}
      </strong>{" "}
      {now !== undefined &&
        seconds > 0 &&
        `${Math.floor(seconds / 60)} min ${seconds % 60} s`}
    </p>
  );
}
export function MockExam({ initial }: { initial: Session }) {
  const [session, setSession] = useState(initial);
  const [saveStatus, setSaveStatus] = useState("");
  const pending = useRef(new Map<string, string>());
  const saving = useRef(false);
  const timer = useRef<number | undefined>(undefined);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
      window.clearTimeout(timer.current);
    };
  }, []);
  async function saveAnswers() {
    if (saving.current || !pending.current.size) return;
    saving.current = true;
    try {
      while (pending.current.size && active.current) {
        const snapshot = new Map(pending.current);
        pending.current.clear();
        try {
          const updated = result(
            await browserApi().PUT("/v1/mock-sessions/{id}/answers", {
              params: { path: { id: session.id } },
              body: {
                answers: Array.from(snapshot, ([exerciseId, answer]) => ({
                  exerciseId,
                  answer,
                })),
              },
            }),
          );
          if (active.current && updated.status === "completed")
            setSession(updated);
        } catch {
          for (const [id, answer] of snapshot) {
            if (!pending.current.has(id)) pending.current.set(id, answer);
          }
          if (active.current)
            setSaveStatus(
              "Enregistrement interrompu. Vérifiez votre connexion puis réessayez.",
            );
          return;
        }
      }
      if (active.current) setSaveStatus("Réponses enregistrées.");
    } finally {
      saving.current = false;
    }
  }
  useEffect(() => {
    if (session.status !== "active") return;
    let cancelled = false;
    let timeout: number;
    async function check() {
      try {
        const updated = result(
          await browserApi().GET("/v1/mock-sessions/{id}", {
            params: { path: { id: session.id } },
          }),
        );
        if (cancelled) return;
        if (updated.status === "completed") {
          setSession(updated);
          return;
        }
      } catch {
        if (cancelled) return;
        setSaveStatus(
          "Temps écoulé. Le résultat sera disponible lorsque la connexion sera rétablie.",
        );
      }
      timeout = window.setTimeout(check, 5000);
    }
    timeout = window.setTimeout(
      check,
      Math.max(100, Date.parse(session.deadline) - Date.now() + 150),
    );
    return () => {
      cancelled = true;
      window.clearTimeout(timeout);
    };
  }, [session.status, session.id, session.deadline]);
  function queue(id: string, answer: string) {
    pending.current.set(id, answer);
    setSaveStatus("Enregistrement…");
    window.clearTimeout(timer.current);
    timer.current = window.setTimeout(() => {
      void saveAnswers();
    }, 300);
  }
  if (session.status === "completed")
    return (
      <section className="panel">
        <h2>Résultat de l’épreuve</h2>
        {session.gradingStatus === "pending_review" ? (
          <div className="feedback">
            <h3>Correction écrite en cours</h3>
            <p>
              {session.automaticPoints} points attribués automatiquement. La
              note finale sera disponible après la relecture des réponses
              rédigées.
            </p>
          </div>
        ) : (
          <p className="score">
            {session.score} / {session.maxScore} points
          </p>
        )}
        {(session.teacherFeedback || session.reviewVideoUrl) && (
          <div className="feedback">
            <h3>Retour de l’enseignante</h3>
            <p>{session.teacherFeedback}</p>
            {session.reviewVideoUrl?.startsWith("https://") && (
              <a
                className="text-link"
                href={session.reviewVideoUrl}
                target="_blank"
                rel="noreferrer"
              >
                Regarder ma correction vidéo ↗
              </a>
            )}
          </div>
        )}
        {session.results.map((row) => (
          <div key={row.exerciseId} className="review-item">
            <h3>
              {session.items.find((i) => i.id === row.exerciseId)?.title ??
                "Exercice"}
            </h3>
            <strong>
              {row.gradingStatus === "pending_review"
                ? "À relire"
                : `${row.points} / ${row.maxPoints} points`}
            </strong>
            <p>
              Votre réponse ·{" "}
              {session.answers[row.exerciseId] || "Aucune réponse remise."}
            </p>
            <Markdown>{row.feedback}</Markdown>
            {row.teacherFeedback && (
              <p className="feedback">{row.teacherFeedback}</p>
            )}
          </div>
        ))}
      </section>
    );
  return (
    <>
      <TimeLeft deadline={session.deadline} />
      <p role="status">{saveStatus}</p>
      {saveStatus.startsWith("Enregistrement interrompu") && (
        <button
          type="button"
          className="button secondary"
          onClick={saveAnswers}
        >
          Réessayer l’enregistrement
        </button>
      )}
      <ActionForm
        label="Remettre ma copie"
        submit={async (form) => {
          const answers = session.items.map((item) => ({
            exerciseId: item.id,
            answer: field(form, item.id),
          }));
          setSession(
            result(
              await browserApi().POST("/v1/mock-sessions/{id}/finish", {
                params: { path: { id: session.id } },
                body: { answers },
              }),
            ),
          );
        }}
      >
        {session.items.map((item) => (
          <section key={item.id} className="panel">
            <small>
              {item.points} point{item.points > 1 ? "s" : ""}
            </small>
            <h2>{item.title}</h2>
            <Markdown>{item.body}</Markdown>
            {item.format === "choice" ? (
              <fieldset>
                <legend>Votre choix pour {item.title}</legend>
                {item.choices.map((choice) => (
                  <label className="option" key={choice}>
                    <input
                      type="radio"
                      name={item.id}
                      value={choice}
                      defaultChecked={initial.answers[item.id] === choice}
                      onChange={() => queue(item.id, choice)}
                    />
                    {choice}
                  </label>
                ))}
              </fieldset>
            ) : (
              <label htmlFor={item.id}>
                Votre réponse pour {item.title}
                {item.format === "text" ? (
                  <textarea
                    id={item.id}
                    name={item.id}
                    rows={5}
                    defaultValue={initial.answers[item.id] ?? ""}
                    onChange={(event) => queue(item.id, event.target.value)}
                  />
                ) : (
                  <input
                    id={item.id}
                    name={item.id}
                    defaultValue={initial.answers[item.id] ?? ""}
                    onChange={(event) => queue(item.id, event.target.value)}
                    inputMode="decimal"
                  />
                )}
              </label>
            )}
          </section>
        ))}
      </ActionForm>
    </>
  );
}
