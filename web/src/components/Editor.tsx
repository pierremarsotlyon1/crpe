"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { browserApi } from "../lib/browser-api";
import {
  type AdminContent,
  type Content,
  type ContentInput,
  checked,
  formatNames,
  kindNames,
  result,
} from "../lib/model";
import { Documents } from "./Documents";
import { ActionForm, field } from "./Form";
import { Markdown } from "./Markdown";

type Parent = Pick<Content, "id" | "title" | "kind">;
const kinds = Object.keys(kindNames) as Content["kind"][];
const formats = Object.keys(formatNames) as Content["format"][];
function lines(value: string) {
  return value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
}

export function Editor({
  item,
  parents,
  initialParent,
}: {
  item?: AdminContent;
  parents: Parent[];
  initialParent?: string;
}) {
  const router = useRouter();
  const [kind, setKind] = useState<Content["kind"]>(
    item?.kind ??
      (initialParent
        ? parents.find((p) => p.id === initialParent)?.kind === "chapter"
          ? "lesson"
          : "exercise"
        : "chapter"),
  );
  const [format, setFormat] = useState<Content["format"]>(
    item?.format ?? "numeric",
  );
  const [body, setBody] = useState(item?.body ?? "");
  const [dirty, setDirty] = useState(false);
  const [preview, setPreview] = useState(false);
  const [solution, setSolution] = useState(item?.solution ?? "");
  const isExercise = kind === "exercise";
  async function save(data: FormData) {
    const value: ContentInput = {
      examId: "crpe",
      kind,
      title: field(data, "title"),
      position: Number(field(data, "position")),
      body,
      skill: field(data, "skill"),
      format: isExercise ? format : "text",
      criteria: lines(field(data, "criteria")),
      choices:
        (isExercise && format === "choice") || kind === "oral"
          ? lines(field(data, "choices"))
          : [],
      ...(kind === "oral" || kind === "mock"
        ? { durationMinutes: Number(field(data, "durationMinutes")) }
        : {}),
      ...(kind === "oral"
        ? { prepMinutes: Number(field(data, "prepMinutes")) }
        : {}),
      ...(isExercise ? { points: Number(field(data, "points")) } : {}),
      publicCorrection: field(data, "publicCorrection"),
      videoDescription: field(data, "videoDescription"),
      sourceTitle: field(data, "sourceTitle"),
      sourceUrl: field(data, "sourceUrl"),
      sourceYear: Number(field(data, "sourceYear")),
      videoUrl: field(data, "videoUrl"),
      answer: field(data, "answer"),
      solution: isExercise ? solution : "",
      tolerance: Number(field(data, "tolerance")),
    };
    const parentId = field(data, "parentId");
    if (parentId) value.parentId = parentId;
    const api = browserApi();
    const saved = result(
      item
        ? await api.PUT("/v1/admin/content/{id}", {
            params: { path: { id: item.id } },
            body: value,
          })
        : await api.POST("/v1/admin/content", { body: value }),
    );
    setDirty(false);
    if (!item) router.replace(`/atelier/contenus/${saved.id}`);
    router.refresh();
    return "Brouillon enregistré.";
  }
  return (
    <>
      <div className="editor panel">
        <ActionForm label="Enregistrer le brouillon" submit={save}>
          <fieldset onChange={() => setDirty(true)}>
            <legend>Informations du contenu</legend>
            <div className="grid2">
              <label>
                Type de contenu
                <select
                  value={kind}
                  disabled={Boolean(item)}
                  onChange={(e) => setKind(e.target.value as Content["kind"])}
                >
                  {kinds.map((k) => (
                    <option key={k} value={k}>
                      {kindNames[k]}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                Ordre d’affichage
                <input
                  type="number"
                  min={0}
                  name="position"
                  defaultValue={item?.position ?? 0}
                  required
                />
              </label>
            </div>
            <label>
              Titre
              <input
                name="title"
                required
                maxLength={200}
                defaultValue={item?.title}
              />
            </label>
            {(kind === "lesson" || isExercise) && (
              <label>
                Rattacher à
                <select
                  name="parentId"
                  defaultValue={item?.parentId ?? initialParent ?? ""}
                >
                  <option value="">Aucun rattachement</option>
                  {parents
                    .filter((p) => isExercise || p.kind === "chapter")
                    .map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.title}
                      </option>
                    ))}
                </select>
              </label>
            )}
            <details open={Boolean(item?.sourceTitle || item?.sourceUrl)}>
              <summary>Renseigner la source du sujet</summary>
              <div className="stack">
                <a
                  className="text-link"
                  href="https://www.devenirenseignant.gouv.fr/les-sujets-des-epreuves-ecrites-et-rapports-des-jurys-des-crpe-1163"
                  target="_blank"
                  rel="noreferrer"
                >
                  Consulter les sujets officiels du CRPE ↗
                </a>
                <label>
                  Intitulé de la source
                  <input name="sourceTitle" defaultValue={item?.sourceTitle} />
                </label>
                <label>
                  Année de la session
                  <input
                    type="number"
                    min="1900"
                    max="2200"
                    name="sourceYear"
                    defaultValue={item?.sourceYear || ""}
                  />
                </label>
                <label>
                  Lien vers le sujet original
                  <input
                    type="url"
                    name="sourceUrl"
                    defaultValue={item?.sourceUrl}
                    placeholder="https://…"
                  />
                </label>
              </div>
            </details>
            <label>
              Notion travaillée
              <input name="skill" defaultValue={item?.skill} maxLength={120} />
            </label>
            <label>
              {kind === "chapter"
                ? "Présentation"
                : isExercise
                  ? "Énoncé"
                  : "Fiche et consignes"}
              <textarea
                name="body"
                rows={8}
                value={body}
                onChange={(e) => setBody(e.target.value)}
                aria-describedby="markdown-help"
              />
            </label>
            <small id="markdown-help">
              Rédigez en Markdown. Entourez une formule de signes $ pour la
              mettre en forme.
            </small>
            {kind === "lesson" && (
              <label>
                URL de la vidéo
                <input
                  type="url"
                  name="videoUrl"
                  defaultValue={item?.videoUrl}
                  placeholder="https://…"
                />
              </label>
            )}
            {kind === "lesson" && (
              <label>
                Description de la vidéo
                <textarea
                  name="videoDescription"
                  defaultValue={item?.videoDescription}
                  rows={3}
                />
              </label>
            )}
            {kind === "mock" && (
              <details>
                <summary>Préparer la page publique du sujet corrigé</summary>
                <p>
                  Un corrigé public renseigné pourra paraître sur le site après
                  validation du sujet et ouverture des ventes. Laissez ce champ
                  vide pour conserver le sujet dans l’espace membre.
                </p>
                <label>
                  Article public en Markdown
                  <textarea
                    name="publicCorrection"
                    defaultValue={item?.publicCorrection}
                    rows={7}
                  />
                </label>
              </details>
            )}
            {(kind === "oral" || kind === "mock") && (
              <label>
                Durée de l’épreuve en minutes
                <input
                  name="durationMinutes"
                  type="number"
                  min="1"
                  max="480"
                  defaultValue={
                    item?.durationMinutes || (kind === "oral" ? 15 : 180)
                  }
                  required
                />
              </label>
            )}
            {kind === "oral" && (
              <>
                <label>
                  Temps de préparation en minutes
                  <input
                    name="prepMinutes"
                    type="number"
                    min="1"
                    max="120"
                    defaultValue={item?.prepMinutes ?? 10}
                  />
                </label>
                <label>
                  Questions du jury, une par ligne
                  <textarea
                    name="choices"
                    rows={3}
                    defaultValue={item?.choices.join("\n")}
                  />
                  <small>Renseignez deux ou trois questions.</small>
                </label>
              </>
            )}
            {isExercise && (
              <>
                <label>
                  Points du barème
                  <input
                    name="points"
                    type="number"
                    step="any"
                    min="0.01"
                    defaultValue={item?.points || 1}
                    required
                  />
                </label>
                <h2>Correction</h2>
                <label>
                  Type de réponse
                  <select
                    value={format}
                    onChange={(e) =>
                      setFormat(e.target.value as Content["format"])
                    }
                  >
                    {formats.map((f) => (
                      <option key={f} value={f}>
                        {formatNames[f]}
                      </option>
                    ))}
                  </select>
                </label>
                {format === "choice" && (
                  <label>
                    Choix proposés, un par ligne
                    <textarea
                      name="choices"
                      rows={4}
                      defaultValue={item?.choices.join("\n")}
                    />
                  </label>
                )}
                <label>
                  {format === "choice"
                    ? "Texte exact du choix correct"
                    : format === "numeric"
                      ? "Résultat numérique attendu"
                      : "Réponse de référence"}
                  <input name="answer" defaultValue={item?.answer} />
                </label>
                {format === "numeric" && (
                  <label>
                    Tolérance absolue
                    <input
                      name="tolerance"
                      type="number"
                      step="any"
                      min={0}
                      defaultValue={item?.tolerance ?? 0}
                    />
                  </label>
                )}
                <label>
                  Corrigé détaillé
                  <textarea
                    name="solution"
                    rows={5}
                    value={solution}
                    onChange={(e) => setSolution(e.target.value)}
                  />
                </label>
              </>
            )}
            {(isExercise || kind === "oral" || kind === "mock") && (
              <label>
                Critères de correction, un par ligne
                <textarea
                  name="criteria"
                  rows={4}
                  defaultValue={item?.criteria.join("\n")}
                />
              </label>
            )}
          </fieldset>
          <button
            className="button secondary"
            type="button"
            onClick={() => setPreview(!preview)}
            aria-expanded={preview}
          >
            {preview ? "Fermer l’aperçu" : "Afficher l’aperçu"}
          </button>
          {preview && (
            <div className="preview">
              <h3>Aperçu de la fiche</h3>
              <Markdown>{body || "La fiche est vide."}</Markdown>
              {isExercise && solution && (
                <>
                  <h3>Corrigé de référence</h3>
                  <Markdown>{solution}</Markdown>
                </>
              )}
            </div>
          )}
        </ActionForm>
      </div>
      {item && <Documents item={item} />}
      {item && (
        <section className="panel publish">
          <h2>Publication</h2>
          {item.demo && (
            <p className="feedback">
              Ce contenu de démonstration reste en brouillon.
            </p>
          )}
          <p>
            {item.status === "validated"
              ? "Ce contenu a été validé. Une modification enregistrée le remet en brouillon."
              : "Relisez la fiche, les formules et le corrigé avant de valider ce contenu pour les élèves."}
          </p>
          {dirty ? (
            <p className="feedback">
              Enregistrez vos modifications avant de publier.
            </p>
          ) : (
            !item.demo &&
            item.status === "draft" && (
              <ActionForm
                label="Valider le contenu"
                refresh
                submit={async () => {
                  const reply = await browserApi().POST(
                    "/v1/admin/content/{id}/validate",
                    { params: { path: { id: item.id } } },
                  );
                  checked(reply.response, reply.error);
                  return "Contenu validé.";
                }}
              />
            )
          )}
          {(kind === "chapter" || kind === "mock" || kind === "lesson") && (
            <Link
              className="text-link"
              href={`/atelier/contenus/nouveau?parent=${item.id}`}
            >
              Ajouter un contenu à{" "}
              {kind === "mock"
                ? "ce sujet"
                : kind === "lesson"
                  ? "cette leçon"
                  : "ce chapitre"}{" "}
              →
            </Link>
          )}
          <details>
            <summary>Supprimer le contenu</summary>
            <ActionForm
              secondary
              label="Supprimer définitivement"
              submit={async (form) => {
                if (!form.get("ack"))
                  throw new Error("La confirmation est requise.");
                checked(
                  (
                    await browserApi().DELETE("/v1/admin/content/{id}", {
                      params: { path: { id: item.id } },
                    })
                  ).response,
                );
                router.replace("/atelier/contenus");
                router.refresh();
              }}
            >
              <label className="check">
                <input type="checkbox" name="ack" required />
                <span>
                  Je confirme la suppression définitive de ce contenu.
                </span>
              </label>
            </ActionForm>
          </details>
        </section>
      )}
    </>
  );
}
