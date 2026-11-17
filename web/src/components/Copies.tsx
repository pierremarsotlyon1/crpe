"use client";
import { useRouter } from "next/navigation";
import { useRef } from "react";
import { browserApi } from "../lib/browser-api";
import { checked, type Submission } from "../lib/model";
import { ActionForm, field } from "./Form";

export function SubmitCopy({ exerciseId }: { exerciseId: string }) {
  const router = useRouter();
  const previous = useRef<{ text: string; image?: File; clientId: string }>(
    undefined,
  );
  return (
    <ActionForm
      label="Envoyer ma copie"
      reset
      submit={async (form) => {
        const text = field(form, "text");
        const file = form.get("image");
        const image = file instanceof File && file.size > 0 ? file : undefined;
        if (!image && !text)
          throw new Error("Rédigez une réponse ou choisissez une photo.");
        if (
          image &&
          (image.size > 8 * 1024 * 1024 ||
            !["image/png", "image/jpeg"].includes(image.type))
        )
          throw new Error("Choisissez une photo JPEG ou PNG de 8 Mo maximum.");
        const data = new FormData();
        if (
          !previous.current ||
          previous.current.text !== text ||
          previous.current.image !== image
        ) {
          previous.current = { text, image, clientId: crypto.randomUUID() };
        }
        data.set("clientId", previous.current.clientId);
        data.set("exerciseId", exerciseId);
        if (text) data.set("text", text);
        if (image) data.set("image", image);
        const reply = await fetch("/api/v1/submissions", {
          method: "POST",
          body: data,
          credentials: "same-origin",
        });
        checked(reply);
        const submission = (await reply.json()) as Submission;
        previous.current = undefined;
        router.push(`/espace/copies/${submission.id}?envoi=1`);
        return "Votre copie a été envoyée. Retrouvez son suivi dans Mes copies.";
      }}
    >
      <label>
        Réponse rédigée
        <textarea name="text" rows={4} maxLength={16000} />
      </label>
      <label>
        Photo de votre copie
        <input name="image" type="file" accept="image/jpeg,image/png" />
        <small>JPEG ou PNG, 8 Mo maximum.</small>
      </label>
    </ActionForm>
  );
}
export function Review({ submission }: { submission: Submission }) {
  return (
    <ActionForm
      label="Envoyer mon retour"
      refresh
      submit={async (form) => {
        checked(
          (
            await browserApi().POST("/v1/admin/submissions/{id}/review", {
              params: { path: { id: submission.id } },
              body: {
                feedback: field(form, "feedback"),
                reviewVideoUrl: submission.supportAccess
                  ? field(form, "reviewVideoUrl")
                  : undefined,
              },
            })
          ).response,
        );
        return "Le retour a été transmis à l’élève.";
      }}
    >
      <label>
        Retour de l’enseignante
        <textarea
          name="feedback"
          rows={5}
          required
          defaultValue={submission.feedback}
        />
      </label>
      {submission.supportAccess && (
        <label>
          Lien de votre correction vidéo
          <input
            type="url"
            name="reviewVideoUrl"
            defaultValue={submission.reviewVideoUrl}
          />
        </label>
      )}
    </ActionForm>
  );
}
