"use client";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { browserApi } from "../lib/browser-api";
import { result } from "../lib/model";
import { ActionForm, field } from "./Form";
export function MockDraft({
  bank,
}: {
  bank: { id: string; title: string; source: string }[];
}) {
  const [fromBank, setFromBank] = useState(false);
  const router = useRouter();
  return (
    <details className="panel">
      <summary>Préparer un sujet blanc avec l’IA</summary>
      <p>
        Le générateur crée un sujet et ses exercices en brouillon. Relisez
        chaque énoncé, le corrigé et le barème avant de les valider.
      </p>
      <ActionForm
        label="Générer les brouillons"
        pendingLabel="Génération en cours…"
        submit={async (form) => {
          const count = Number(field(form, "count"));
          const ids = form.getAll("sourceExerciseIds").map(String);
          if (
            fromBank &&
            (ids.length < 2 || ids.length < count || ids.length > 100)
          )
            throw new Error(
              "Sélectionnez de 2 à 100 exercices, avec au moins le nombre demandé pour le sujet.",
            );
          const reply = result(
            await browserApi().POST("/v1/admin/mock-drafts", {
              body: {
                title: field(form, "title"),
                topic: field(form, "topic"),
                count,
                sourceExerciseIds: fromBank ? ids : undefined,
                durationMinutes: Number(field(form, "durationMinutes")),
              },
            }),
          );
          const mock = reply.items.find((i) => i.kind === "mock");
          if (!mock)
            throw new Error(
              "La réponse ne contient pas de sujet. Vérifiez votre bibliothèque avant de réessayer.",
            );
          router.push(`/atelier/contenus/${mock.id}`);
          router.refresh();
        }}
      >
        <label>
          Mode de composition
          <select
            value={fromBank ? "bank" : "arithmetic"}
            onChange={(e) => setFromBank(e.target.value === "bank")}
          >
            <option value="arithmetic">
              Créer des exercices de calcul vérifiables
            </option>
            <option value="bank">Composer depuis la banque de sujets</option>
          </select>
        </label>
        {fromBank && (
          <fieldset>
            <legend>Exercices validés avec une source</legend>
            <p>
              Choisissez les exercices que l’IA peut assembler. Les énoncés et
              leurs corrigés proviennent de la banque.
            </p>
            {bank.length ? (
              bank.map((i) => (
                <label className="check" key={i.id}>
                  <input
                    type="checkbox"
                    name="sourceExerciseIds"
                    value={i.id}
                  />
                  <span>
                    {i.title}
                    <small>{i.source}</small>
                  </span>
                </label>
              ))
            ) : (
              <p>
                Aucun exercice validé avec source. Renseignez la source ou
                joignez un PDF dans la bibliothèque.
              </p>
            )}
          </fieldset>
        )}
        <label>
          Titre du sujet proposé
          <input name="title" required maxLength={160} />
        </label>
        <label>
          Notion ou consigne de génération
          <textarea name="topic" rows={3} required maxLength={1500} />
        </label>
        <div className="grid2">
          <label>
            Nombre d’exercices
            <input
              type="number"
              min="1"
              max="10"
              defaultValue={3}
              name="count"
              required
            />
          </label>
          <label>
            Durée visée en minutes
            <input
              type="number"
              min="1"
              max="480"
              defaultValue={60}
              name="durationMinutes"
              required
            />
          </label>
        </div>
      </ActionForm>
    </details>
  );
}
