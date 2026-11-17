"use client";
import type { components } from "@crpe/api-client";
import { browserApi } from "../lib/browser-api";
import { result } from "../lib/model";
import { ActionForm, field } from "./Form";

type Settings = Pick<
  components["schemas"]["SupportSettings"],
  "liveUrl" | "groupUrl" | "liveSchedule"
>;
export function SupportSettings({ settings }: { settings: Settings }) {
  return (
    <section className="panel">
      <h2>Rendez-vous et groupe</h2>
      <p>
        Ces informations sont visibles par les élèves disposant de
        l’accompagnement Suivi.
      </p>
      <ActionForm
        label="Enregistrer les informations"
        refresh
        submit={async (form) => {
          const body = {
            liveUrl: field(form, "liveUrl"),
            liveSchedule: field(form, "liveSchedule"),
            groupUrl: field(form, "groupUrl"),
          };
          result(await browserApi().PUT("/v1/admin/support", { body }));
          return "Informations de la formule Suivi enregistrées.";
        }}
      >
        <label>
          Horaire hebdomadaire et fuseau
          <textarea
            name="liveSchedule"
            defaultValue={settings.liveSchedule}
            rows={2}
            placeholder="Par exemple, le mercredi à 18 h, heure de Paris."
          />
        </label>
        <label>
          Lien de la visioconférence
          <input
            type="url"
            name="liveUrl"
            defaultValue={settings.liveUrl}
            placeholder="https://…"
          />
        </label>
        <label>
          Invitation du groupe WhatsApp
          <input
            type="url"
            name="groupUrl"
            defaultValue={settings.groupUrl}
            placeholder="https://chat.whatsapp.com/…"
          />
        </label>
      </ActionForm>
    </section>
  );
}
type Session = components["schemas"]["MockSession"];
export function TeacherMockReview({
  session,
  supportAccess,
}: {
  session: Session;
  supportAccess: boolean;
}) {
  const written = session.items.filter((i) => i.format === "text");
  return (
    <section className="panel">
      <h2>Évaluer la copie</h2>
      <ActionForm
        label="Transmettre mon retour"
        refresh
        submit={async (form) => {
          const assessments = written.flatMap((item) => {
            const value = field(form, `points-${item.id}`);
            const feedback = field(form, `comment-${item.id}`);
            if (!value && feedback)
              throw new Error(
                "Indiquez les points associés au commentaire pour " +
                  item.title +
                  ".",
              );
            return value
              ? [{ exerciseId: item.id, points: Number(value), feedback }]
              : [];
          });
          const feedback = field(form, "feedback");
          const reviewVideoUrl = supportAccess
            ? field(form, "reviewVideoUrl")
            : undefined;
          if (!assessments.length && !feedback && !reviewVideoUrl)
            throw new Error(
              "Renseignez une appréciation, des points ou un retour vidéo.",
            );
          result(
            await browserApi().POST("/v1/admin/mock-sessions/{id}/review", {
              params: { path: { id: session.id } },
              body: { feedback, assessments, reviewVideoUrl },
            }),
          );
          return "Le retour est disponible dans l’espace de l’élève.";
        }}
      >
        {written.length > 0 && (
          <p>
            Pour une correction partielle, laissez les points des exercices à
            reprendre vides.
          </p>
        )}
        {written.map((item) => {
          const grade = session.results.find(
            (row) => row.exerciseId === item.id,
          );
          return (
            <fieldset key={item.id}>
              <legend>{item.title}</legend>
              {item.criteria.length > 0 && (
                <ul>
                  {item.criteria.map((c) => (
                    <li key={c}>{c}</li>
                  ))}
                </ul>
              )}
              <label>
                Points pour {item.title}
                <input
                  type="number"
                  min={0}
                  max={grade?.maxPoints ?? item.points}
                  step="any"
                  name={`points-${item.id}`}
                  defaultValue={grade?.points}
                />
                <small>Sur {grade?.maxPoints ?? item.points} points</small>
              </label>
              <label>
                Appréciation pour {item.title}
                <textarea
                  name={`comment-${item.id}`}
                  rows={2}
                  defaultValue={grade?.teacherFeedback}
                />
              </label>
            </fieldset>
          );
        })}
        <label>
          Commentaires sur l’épreuve
          <textarea
            name="feedback"
            defaultValue={session.teacherFeedback}
            rows={4}
          />
        </label>
        {supportAccess && (
          <label>
            Lien de votre correction vidéo
            <input
              type="url"
              name="reviewVideoUrl"
              defaultValue={session.reviewVideoUrl}
              placeholder="https://…"
            />
          </label>
        )}
      </ActionForm>
    </section>
  );
}
