"use client";
import Link from "next/link";
import { dateLabel, type Submission } from "../lib/model";
import { Review } from "./Copies";
import { CorrectionFeedback } from "./CorrectionFeedback";
import { useSubmissionProgress } from "./useSubmissionProgress";

const stages = {
  queued: "En file d’attente",
  processing: "Correction en cours",
  retrying: "Nouvelle tentative prévue",
  teacher_review: "En attente de l’enseignante",
  completed: "Correction terminée",
  failed: "Traitement interrompu",
} satisfies Record<Submission["correctionProgress"]["stage"], string>;
export function SubmissionList({
  items: initial,
  single = false,
  titles,
  teacher = false,
}: {
  items: Submission[];
  single?: boolean;
  titles: Map<string, string>;
  teacher?: boolean;
}) {
  const { items, stale } = useSubmissionProgress(initial, teacher, single);
  if (!items.length)
    return (
      <div className="empty">
        {teacher
          ? "Aucune copie à relire pour le moment."
          : "Vous n’avez pas encore envoyé de copie. Ouvrez un exercice de votre parcours pour déposer une réponse."}
      </div>
    );
  return (
    <div className="stack">
      {stale && (
        <p className="feedback neutral" role="status">
          Actualisation indisponible. Le dernier état reçu est affiché. La
          vérification reprendra automatiquement.
        </p>
      )}
      {items.map((item) => (
        <article key={item.id} className="panel">
          <div className="row-title">
            <div>
              <small>{dateLabel(item.createdAt)}</small>
              <h2>{titles.get(item.exerciseId) ?? "Exercice archivé"}</h2>
            </div>
            <span
              role="status"
              className={`status ${item.correctionProgress.stage === "completed" ? "validated" : ""}`}
            >
              {stages[item.correctionProgress.stage]}
            </span>
          </div>
          <small>
            Mis à jour le{" "}
            {new Intl.DateTimeFormat("fr-FR", {
              dateStyle: "short",
              timeStyle: "medium",
              timeZone: "Europe/Paris",
            }).format(new Date(item.correctionProgress.updatedAt))}
            {item.correctionProgress.attempts > 0 &&
              ` · ${item.correctionProgress.attempts} tentative${item.correctionProgress.attempts > 1 ? "s" : ""}`}
          </small>
          {item.correctionProgress.stage === "failed" && (
            <p>
              La correction automatique n’a pas abouti. La copie reste
              disponible pour relecture.
            </p>
          )}
          {!teacher && !single && (
            <Link className="text-link" href={`/espace/copies/${item.id}`}>
              Suivre cette copie →
            </Link>
          )}
          {item.text && <p className="preserve">{item.text}</p>}
          {item.hasImage && (
            <a
              className="text-link"
              href={`/api/v1/submissions/${item.id}/image`}
              target="_blank"
              rel="noreferrer"
            >
              Ouvrir la photo de la copie ↗
            </a>
          )}
          {item.reviewVideoUrl?.startsWith("https://") && (
            <a
              className="text-link"
              href={item.reviewVideoUrl}
              target="_blank"
              rel="noreferrer"
            >
              Regarder le retour vidéo de l’enseignante ↗
            </a>
          )}
          {item.correction && (
            <CorrectionFeedback
              correction={item.correction}
              teacher={teacher}
              similar={titles.get(item.correction.similarExerciseId ?? "")}
            />
          )}
          {teacher && item.status === "pending_review" ? (
            <Review submission={item} />
          ) : (
            item.status === "reviewed" &&
            item.feedback && (
              <div className="feedback">
                <strong>Retour de l’enseignante</strong>
                <p className="preserve">{item.feedback}</p>
              </div>
            )
          )}
        </article>
      ))}
    </div>
  );
}
