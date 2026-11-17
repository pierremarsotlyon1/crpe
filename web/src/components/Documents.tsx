"use client";
import { browserApi } from "../lib/browser-api";
import { type Content, checked } from "../lib/model";
import { ActionForm, field } from "./Form";
export function Documents({ item }: { item: Content }) {
  return (
    <section className="panel">
      <h2>Documents du sujet</h2>
      <p>
        Joignez le PDF du sujet ou une fiche. Toute modification des pièces
        jointes nécessite une nouvelle validation du contenu.
      </p>
      {item.documents.map((doc) => (
        <details key={doc.id}>
          <summary>{doc.title}</summary>
          <a
            className="text-link"
            href={`/api/v1/documents/${doc.id}/file`}
            target="_blank"
            rel="noreferrer"
          >
            Consulter le PDF ↗
          </a>
          <ActionForm
            secondary
            label="Retirer le document"
            refresh
            submit={async () => {
              checked(
                (
                  await browserApi().DELETE("/v1/admin/documents/{id}", {
                    params: { path: { id: doc.id } },
                  })
                ).response,
              );
              return "Document retiré.";
            }}
          >
            <label className="check">
              <input type="checkbox" required />
              Je confirme le retrait de ce PDF.
            </label>
          </ActionForm>
        </details>
      ))}
      <ActionForm
        label="Joindre le PDF"
        reset
        refresh
        submit={async (form) => {
          const file = form.get("file");
          if (
            !(file instanceof File) ||
            file.size === 0 ||
            file.size > 20 * 1024 * 1024
          )
            throw new Error("Choisissez un PDF de 20 Mo maximum.");
          const data = new FormData();
          data.append("file", file);
          data.append("title", field(form, "title"));
          checked(
            await fetch(`/api/v1/admin/content/${item.id}/documents`, {
              method: "POST",
              body: data,
              credentials: "same-origin",
            }),
          );
          return "PDF joint au brouillon.";
        }}
      >
        <label>
          Titre du document
          <input name="title" maxLength={200} required />
        </label>
        <label>
          Document PDF
          <input
            name="file"
            type="file"
            accept="application/pdf,.pdf"
            required
          />
          <small>20 Mo maximum.</small>
        </label>
      </ActionForm>
    </section>
  );
}
