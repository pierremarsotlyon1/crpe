import type { components } from "@crpe/api-client";
import Link from "next/link";
import { Markdown } from "./Markdown";

type Correction = components["schemas"]["Correction"];
export function CorrectionFeedback({
  correction,
  similar,
  teacher = false,
}: {
  correction: Correction;
  similar?: string;
  teacher?: boolean;
}) {
  return (
    <section className="feedback">
      <p className="overline">
        {correction.generatedByAI
          ? "Généré par IA"
          : correction.verdict === "uncertain"
            ? "À relire par l’enseignante"
            : "Vérification numérique"}
      </p>
      <h3>
        {
          {
            correct: "Résultat correct",
            incorrect: "Réponse à reprendre",
            uncertain: "Réponse à vérifier",
          }[correction.verdict]
        }
      </h3>
      <p>{correction.summary}</p>
      {correction.errors.length > 0 && (
        <>
          <h4>Points à reprendre</h4>
          <ul>
            {correction.errors.map((error) => (
              <li key={error.location + error.message}>
                <strong>{error.location}</strong> {error.message}
              </li>
            ))}
          </ul>
        </>
      )}
      {correction.writingFeedback.length > 0 && (
        <>
          <h4>Rédaction</h4>
          <ul>
            {correction.writingFeedback.map((line) => (
              <li key={line}>{line}</li>
            ))}
          </ul>
        </>
      )}
      {correction.referenceSolution && (
        <details>
          <summary>Corrigé de référence</summary>
          <Markdown>{correction.referenceSolution}</Markdown>
        </details>
      )}
      {correction.needsTeacherReview && (
        <p>Cette copie a été signalée pour relecture par l’enseignante.</p>
      )}
      {correction.similarExerciseId && similar && !teacher && (
        <Link href={`/espace/chapitres/${correction.similarExerciseId}`}>
          Exercice à poursuivre · {similar} →
        </Link>
      )}
    </section>
  );
}
