"use client";
import Link from "next/link";
import { useState } from "react";
import { browserApi } from "../lib/browser-api";
import { type Content, type ContentInput, result } from "../lib/model";
import { ActionForm, field } from "./Form";
export function Importer({
  parents,
}: {
  parents: Pick<Content, "id" | "title">[];
}) {
  const [preview, setPreview] = useState<{
    items: ContentInput[];
    clientId: string;
  }>();
  const [done, setDone] = useState(false);
  return (
    <>
      <section className="panel">
        <h2>Importer un fichier</h2>
        <p>
          Importez une fiche Markdown ou un tableau CSV d’exercices. Vous
          pourrez relire chaque brouillon avant publication.
        </p>
        <ActionForm
          label="Prévisualiser le fichier"
          submit={async (form) => {
            const file = form.get("file");
            if (!(file instanceof File) || !file.size || file.size > 524288)
              throw new Error("Choisissez un fichier texte de 512 Ko maximum.");
            setPreview(undefined);
            setDone(false);
            const { parseContentFile } = await import("../lib/import-content");
            const items = parseContentFile(
              file.name,
              await file.text(),
              field(form, "parentId"),
            );
            setPreview({ items, clientId: crypto.randomUUID() });
          }}
        >
          <label>
            Fichier Markdown ou CSV
            <input type="file" required name="file" accept=".md,.csv" />
          </label>
          <label>
            Chapitre de destination
            <select name="parentId" defaultValue="">
              <option value="">Sans chapitre</option>
              {parents.map((p) => (
                <option value={p.id} key={p.id}>
                  {p.title}
                </option>
              ))}
            </select>
          </label>
        </ActionForm>
        <details>
          <summary>Format du fichier</summary>
          <p>
            Dans une fiche Markdown, le premier titre de niveau 1 devient le
            titre de la leçon. Pour un CSV, utilisez la première ligne comme
            en-tête et une ligne par exercice.
          </p>
          <p>
            Les colonnes obligatoires sont <code>titre</code> et{" "}
            <code>enonce</code>. Les formats sont <code>qcm</code>,{" "}
            <code>numerique</code> et <code>redaction</code>. Séparez les choix
            et les critères par une barre verticale.
          </p>
          <a download href="/modele-exercices.csv">
            Télécharger le modèle CSV vierge
          </a>
        </details>
      </section>
      {preview && !done && (
        <section className="panel">
          <h2>
            {preview.items.length} brouillon
            {preview.items.length > 1 ? "s" : ""} à importer
          </h2>
          <ul>
            {preview.items.map((item) => (
              <li key={`${item.position ?? 0}-${item.title}`}>{item.title}</li>
            ))}
          </ul>
          <ActionForm
            label="Importer les brouillons"
            submit={async () => {
              result(
                await browserApi().POST("/v1/admin/import", { body: preview }),
              );
              setDone(true);
            }}
          />
        </section>
      )}
      {done && (
        <div className="feedback" role="status">
          Import terminé.{" "}
          <Link href="/atelier/contenus">Ouvrir la bibliothèque</Link>
        </div>
      )}
    </>
  );
}
