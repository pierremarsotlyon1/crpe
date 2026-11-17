import { notFound } from "next/navigation";
import { Refund } from "../../../components/Billing";
import { Empty, Heading } from "../../../components/Shell";
import { adminApi } from "../../../lib/api";
import { dateLabel, result } from "../../../lib/model";
import { money } from "../../../lib/money";
import { publicConfig } from "../../../lib/public-site";
export default async function Payments() {
  if (!(await publicConfig()).salesEnabled) notFound();
  const { client } = await adminApi();
  const payments = result(await client.GET("/v1/admin/payments")).items;
  return (
    <>
      <Heading kicker="Atelier enseignant" title="Paiements" />
      {payments.length === 0 ? (
        <Empty>Aucun paiement enregistré.</Empty>
      ) : (
        payments.map((p) => (
          <article key={p.paymentIntentId} className="panel">
            <small>{dateLabel(p.createdAt)}</small>
            <h2>{money(p.amount, p.currency)}</h2>
            <p>{p.email}</p>
            {p.refunded || p.refundStatus === "succeeded" ? (
              <span className="status validated">Remboursé</span>
            ) : p.refundStatus === "pending" ? (
              <span className="status">Remboursement en cours</span>
            ) : p.refundStatus === "requires_action" ? (
              <p className="feedback">
                Le prestataire demande une action pour finaliser le
                remboursement. Consultez le paiement dans Stripe.
              </p>
            ) : p.refundStatus === "failed" ? (
              <p className="feedback error">
                Le remboursement a échoué. Vérifiez son motif dans Stripe avant
                toute nouvelle demande.
              </p>
            ) : (
              <Refund paymentId={p.paymentIntentId} />
            )}
          </article>
        ))
      )}
    </>
  );
}
