"use client";
import { useState } from "react";
import { browserApi } from "../lib/browser-api";
import { result } from "../lib/model";
import { ActionForm, field } from "./Form";

function openStripe(url: string) {
  const dest = new URL(url);
  if (
    dest.protocol !== "https:" ||
    dest.username ||
    dest.password ||
    !(dest.hostname === "stripe.com" || dest.hostname.endsWith(".stripe.com"))
  )
    throw new Error("L’adresse de paiement reçue est invalide.");
  window.location.assign(dest.href);
}
export function Checkout({ planKey }: { planKey: string }) {
  const [existing, setExisting] = useState(false);
  if (existing)
    return (
      <div>
        <p role="status">
          Votre abonnement est déjà actif ou en cours de confirmation.
        </p>
        <a className="text-link" href="/espace/abonnement">
          Consulter mon abonnement →
        </a>
      </div>
    );
  return (
    <ActionForm
      label="Choisir cette formule"
      submit={async () => {
        const reply = await browserApi().POST("/v1/billing/checkout", {
          body: { planKey },
        });
        if (reply.response.status === 409) {
          setExisting(true);
          return;
        }
        openStripe(result(reply).url);
      }}
    />
  );
}
export function Portal() {
  return (
    <ActionForm
      label="Gérer mon abonnement"
      secondary
      submit={async () => {
        openStripe(result(await browserApi().POST("/v1/billing/portal")).url);
      }}
    />
  );
}
export function Refund({ paymentId }: { paymentId: string }) {
  return (
    <details>
      <summary>Rembourser le paiement</summary>
      <ActionForm
        label="Confirmer le remboursement"
        secondary
        refresh
        submit={async (form) => {
          if (!field(form, "confirm"))
            throw new Error("Cochez la confirmation.");
          const refund = result(
            await browserApi().POST("/v1/admin/refunds", {
              body: { paymentIntentId: paymentId },
            }),
          );
          return `Demande de remboursement transmise. Référence ${refund.refundId}.`;
        }}
      >
        <label className="check">
          <input type="checkbox" name="confirm" required />
          <span>Je confirme le remboursement intégral de ce paiement.</span>
        </label>
      </ActionForm>
    </details>
  );
}
