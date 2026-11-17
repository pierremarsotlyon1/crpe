"use client";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { browserApi } from "../lib/browser-api";
import { checked, result } from "../lib/model";
import { ActionForm, field } from "./Form";

export function EmailForm({ waitlist = false }: { waitlist?: boolean }) {
  return (
    <ActionForm
      label={waitlist ? "M’inscrire à la liste" : "Recevoir mon lien"}
      reset
      submit={async (form) => {
        const email = field(form, "email");
        const api = browserApi();
        const reply = waitlist
          ? await api.POST("/v1/waitlist", {
              body: { email, consent: form.get("consent") === "on" },
            })
          : await api.POST("/v1/auth/request", {
              body: { email, client: "web" },
            });
        checked(reply.response);
        return waitlist
          ? "Consultez votre boîte mail pour confirmer votre inscription."
          : "Consultez votre boîte mail pour ouvrir votre lien personnel.";
      }}
    >
      <label>
        Adresse email
        <input
          name="email"
          type="email"
          autoComplete="email"
          required
          maxLength={254}
          placeholder="vous@exemple.fr"
        />
      </label>
      {waitlist && (
        <label className="check">
          <input type="checkbox" name="consent" required />
          <span>
            J’accepte de recevoir par email les nouvelles de l’ouverture. Je
            pourrai me désinscrire à tout moment.{" "}
            <Link href="/informations/confidentialite">Confidentialité</Link>
          </span>
        </label>
      )}
    </ActionForm>
  );
}
export function LinkConfirmation({
  mode,
}: {
  mode: "login" | "confirm" | "unsubscribe";
}) {
  const router = useRouter();
  return (
    <ActionForm
      once
      label={
        mode === "login"
          ? "Ouvrir mon espace"
          : mode === "confirm"
            ? "Confirmer mon inscription"
            : "Me désinscrire"
      }
      submit={async () => {
        const token = new URLSearchParams(window.location.hash.slice(1)).get(
          "token",
        );
        if (!token) throw new Error("Ouvrez le lien complet reçu par email.");
        const api = browserApi();
        if (mode === "login") {
          const session = result(
            await api.POST("/v1/auth/verify", {
              body: { token, client: "web" },
            }),
          );
          window.history.replaceState(null, "", window.location.pathname);
          router.replace(
            session.user.role === "admin" ? "/atelier" : "/espace",
          );
          router.refresh();
        } else {
          const reply =
            mode === "confirm"
              ? await api.POST("/v1/waitlist/confirm", { body: { token } })
              : await api.POST("/v1/waitlist/unsubscribe", { body: { token } });
          checked(reply.response);
          window.history.replaceState(null, "", window.location.pathname);
          return mode === "confirm"
            ? "Votre inscription est confirmée."
            : "Votre adresse a été désinscrite.";
        }
      }}
    />
  );
}
export function Logout() {
  const router = useRouter();
  return (
    <ActionForm
      label="Se déconnecter"
      secondary
      submit={async () => {
        checked((await browserApi().POST("/v1/auth/logout")).response);
        router.replace("/");
        router.refresh();
      }}
    />
  );
}
