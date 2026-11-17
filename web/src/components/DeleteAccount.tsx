"use client";
import { useRouter } from "next/navigation";
import { browserApi } from "../lib/browser-api";
import { checked } from "../lib/model";
import { ActionForm, field } from "./Form";
export function DeleteAccount() {
  const router = useRouter();
  return (
    <details>
      <summary>Supprimer mon compte</summary>
      <p>
        La suppression de votre compte et de vos données pédagogiques est
        définitive. Vous pouvez d’abord télécharger votre export.
      </p>
      <ActionForm
        label="Supprimer définitivement mon compte"
        secondary
        submit={async (form) => {
          if (field(form, "confirm") !== "SUPPRIMER")
            throw new Error("Saisissez SUPPRIMER pour confirmer.");
          checked((await browserApi().DELETE("/v1/me")).response);
          router.replace("/");
          router.refresh();
        }}
      >
        <label>
          Saisissez SUPPRIMER pour confirmer
          <input
            type="text"
            required
            name="confirm"
            pattern="SUPPRIMER"
            autoComplete="off"
          />
        </label>
      </ActionForm>
    </details>
  );
}
