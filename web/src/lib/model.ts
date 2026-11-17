import type { components } from "@crpe/api-client";
export type Content = components["schemas"]["Content"];
export type AdminContent = components["schemas"]["AdminContent"];
export type ContentInput = components["schemas"]["ContentInput"];
export type User = components["schemas"]["User"];
export type Submission = components["schemas"]["Submission"];
export const kindNames = {
  chapter: "Chapitre",
  lesson: "Leçon",
  exercise: "Exercice",
  mock: "Sujet blanc",
  oral: "Sujet d’oral",
} satisfies Record<Content["kind"], string>;
export const formatNames = {
  choice: "QCM",
  numeric: "Réponse numérique",
  text: "Réponse rédigée",
} as const;

export function dateLabel(date: string) {
  return new Intl.DateTimeFormat("fr-FR", {
    day: "numeric",
    month: "long",
    year: "numeric",
    timeZone: "Europe/Paris",
  }).format(new Date(date));
}
export function checked(response: Response, detail?: unknown) {
  if (response.ok) return;
  if (
    detail &&
    typeof detail === "object" &&
    "error" in detail &&
    typeof detail.error === "string" &&
    /[ .’éàè]/.test(detail.error) &&
    detail.error.length < 500
  )
    throw new Error(detail.error);
  const messages: Record<number, string> = {
    400: "Vérifiez les champs saisis. Le lien utilisé peut également avoir expiré.",
    401: "Votre session ou votre lien a expiré. Demandez un nouveau lien de connexion.",
    403: "Votre compte ne dispose pas des droits nécessaires.",
    404: "Cet élément est introuvable ou le lien a expiré.",
    409: "Cette modification est en conflit avec l’état actuel. Actualisez la page.",
    413: "Le fichier dépasse la taille autorisée.",
    415: "Ce type de fichier n’est pas pris en charge.",
    422: "Des champs sont incomplets ou incompatibles. Vérifiez votre saisie.",
    429: "Trop de demandes ont été envoyées. Réessayez dans quelques minutes.",
  };
  throw new Error(
    messages[response.status] ??
      "Le service est momentanément indisponible. Réessayez dans un instant.",
  );
}
export function result<T>(reply: {
  data?: T;
  error?: unknown;
  response: Response;
}): T {
  checked(reply.response, reply.error);
  if (reply.data === undefined)
    throw new Error("Le serveur a renvoyé une réponse incomplète.");
  return reply.data;
}
