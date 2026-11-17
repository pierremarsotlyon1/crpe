import { Portal } from "../../../components/Billing";
import { CurrentSubscriptions } from "../../../components/Membership";
import { Plans } from "../../../components/Plans";
import { Empty, Heading } from "../../../components/Shell";
import { memberAccess, memberApi } from "../../../lib/api";
import { dateLabel, result } from "../../../lib/model";
import { money } from "../../../lib/money";
export default async function Membership() {
  const access = await memberAccess();
  const providers = access.providers ?? [];
  const store = providers.some((p) => p.provider !== "stripe");
  const stripe = providers.some((p) => p.provider === "stripe");
  const { client } = await memberApi();
  const [plans, invoices] = await Promise.all([
    access.salesEnabled && !store ? client.GET("/v1/billing/plans") : null,
    client.GET("/v1/billing/invoices"),
  ]);
  return (
    <>
      <Heading kicker="Espace élève" title="Mon abonnement" />
      {providers.length > 0 ? (
        <CurrentSubscriptions providers={providers} />
      ) : (
        <p className="feedback neutral">
          Aucun abonnement actif sur ce compte.
        </p>
      )}
      {plans?.response.ok ? (
        <Plans
          plans={result(plans).items.filter(
            (p) =>
              !stripe || (access.tier === "autonomie" && p.tier === "suivi"),
          )}
          member
          hideEmpty={stripe}
        />
      ) : (
        plans && (
          <p role="status">Les formules sont momentanément indisponibles.</p>
        )
      )}
      {invoices && (
        <section className="section">
          <div className="row-title">
            <h2>Mes factures Stripe</h2>
            {!stripe && invoices.data && invoices.data.items.length > 0 && (
              <Portal />
            )}
          </div>
          {!invoices.response.ok ? (
            <p role="status">Vos factures sont momentanément indisponibles.</p>
          ) : invoices.data?.items.length ? (
            <ul className="plain-list">
              {invoices.data.items.map((i) => (
                <li key={i.id}>
                  <span>
                    {dateLabel(new Date(i.created * 1000).toISOString())} ·{" "}
                    {money(i.amount, i.currency)}
                  </span>
                  {i.url.startsWith("https://") && (
                    <a href={i.url} target="_blank" rel="noreferrer">
                      Consulter la facture ↗
                    </a>
                  )}
                </li>
              ))}
            </ul>
          ) : (
            <Empty>Aucune facture disponible.</Empty>
          )}
        </section>
      )}
    </>
  );
}
