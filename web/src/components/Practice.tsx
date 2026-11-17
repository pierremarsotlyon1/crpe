"use client";
import Link from "next/link";
import { useState } from "react";
import { browserApi } from "../lib/browser-api";
import { type Content, dateLabel, result } from "../lib/model";
import { ActionForm, field } from "./Form";
import { Markdown } from "./Markdown";

export function Exercise({
  item,
  onGraded,
}: {
  item: Content;
  onGraded?: () => void;
}) {
  const [feedback, setFeedback] = useState<{
    correct: boolean;
    feedback: string;
    nextReviewAt: string;
  }>();
  const [retryKey, setRetryKey] = useState<{ answer: string; id: string }>();
  return (
    <div className="exercise">
      <Markdown>{item.body}</Markdown>
      {feedback ? (
        <section
          className={`feedback ${feedback.correct ? "" : "neutral"}`}
          aria-live="polite"
        >
          <h3>{feedback.correct ? "Réponse correcte" : "À retravailler"}</h3>
          <Markdown>{feedback.feedback}</Markdown>
          <small>
            Prochaine révision prévue le {dateLabel(feedback.nextReviewAt)}.
          </small>
        </section>
      ) : (
        <ActionForm
          label="Vérifier ma réponse"
          submit={async (form) => {
            const answer = field(form, "answer");
            const clientId =
              retryKey?.answer === answer ? retryKey.id : crypto.randomUUID();
            setRetryKey({ answer, id: clientId });
            const response = result(
              await browserApi().POST("/v1/attempts", {
                body: { exerciseId: item.id, clientId, answer },
              }),
            );
            setFeedback(response);
            onGraded?.();
          }}
        >
          {item.format === "choice" ? (
            <fieldset>
              <legend>Choisissez une réponse</legend>
              {item.choices.map((choice) => (
                <label className="option" key={choice}>
                  <input type="radio" name="answer" value={choice} required />
                  {choice}
                </label>
              ))}
            </fieldset>
          ) : (
            <label>
              Votre réponse numérique
              <input
                type="text"
                inputMode="decimal"
                name="answer"
                autoComplete="off"
                required
                maxLength={100}
              />
            </label>
          )}
        </ActionForm>
      )}
    </div>
  );
}

export function PracticeSession({ items }: { items: Content[] }) {
  const [index, setIndex] = useState(0);
  const [graded, setGraded] = useState(false);
  const item = items[index];
  if (!item)
    return (
      <div className="panel">
        <h2>Séance terminée</h2>
        <p>Vos {items.length} réponses ont été enregistrées.</p>
        <Link className="button" href="/espace/progression">
          Voir ma progression
        </Link>
      </div>
    );
  return (
    <section className="panel">
      <div className="row-title">
        <p className="overline">
          Question {index + 1} sur {items.length}
        </p>
        <span className="status">{item.skill || "Mathématiques"}</span>
      </div>
      <progress
        aria-label="Questions traitées"
        value={index + (graded ? 1 : 0)}
        max={items.length}
      />
      <h2>{item.title}</h2>
      <Exercise key={item.id} item={item} onGraded={() => setGraded(true)} />
      {graded && (
        <button
          className="button next"
          type="button"
          onClick={() => {
            setIndex(index + 1);
            setGraded(false);
          }}
        >
          {index + 1 === items.length
            ? "Terminer la séance"
            : "Question suivante"}{" "}
          →
        </button>
      )}
    </section>
  );
}
