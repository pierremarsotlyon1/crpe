import { cookies } from "next/headers";
import Link from "next/link";
import { notFound } from "next/navigation";
import { CurrentSubscriptions } from "../../components/Membership";
import { Plans } from "../../components/Plans";
import { Heading, PublicShell } from "../../components/Shell";
import { createServerAPIClient } from "../../lib/api";
import { result } from "../../lib/model";
import { publicConfig } from "../../lib/public-site";
export const dynamic = "force-dynamic";
export const metadata = { title: "Formules de préparation" };
export default async function Offers({
  searchParams,
}: {
  searchParams: Promise<{ tier?: string; feature?: string }>;
}) {
  if (!(await publicConfig()).salesEnabled) notFound();
  const [query, jar] = await Promise.all([searchParams, cookies()]);
  const client = createServerAPIClient(jar.get("crpe_session")?.value);
  const identity = jar.has("crpe_session")
    ? await client.GET("/v1/me/access")
    : null;
  // Avoid presenting another purchase when an authenticated member's current access cannot be checked.
  const unavailable = Boolean(
    identity && !identity.response.ok && identity.response.status !== 401,
  );
  const access = identity?.data;
  const providers = access?.providers ?? [];
  const store = providers.some((p) => p.provider !== "stripe");
  const currentStripe = providers.some((p) => p.provider === "stripe");
  const reply =
    !unavailable && !store ? await client.GET("/v1/billing/plans") : null;
  const tier =
    query.tier === "suivi" || query.tier === "autonomie"
      ? query.tier
      : undefined;
  const plans = reply?.response.ok
    ? result(reply).items.filter(
        (p) =>
          (!tier || (p.tier ?? "autonomie") === tier) &&
          (!currentStripe ||
            (access?.tier === "autonomie" && p.tier === "suivi")),
      )
    : undefined;
  const heading = store
    ? "Votre abonnement"
    : tier
      ? `Formule ${tier === "suivi" ? "Suivi" : "Autonomie"}`
      : "Formules de préparation";
  return (
    <PublicShell>
      <section className="legal">
        <Heading title={heading}>
          {tier && !store
            ? "Cette formule donne accès à l’activité choisie."
            : undefined}
        </Heading>
        {store && tier === "suivi" && access?.tier !== "suivi" && (
          <p className="feedback">
            Cette activité nécessite la formule Suivi. Pour changer de formule,
            ouvrez la gestion de votre abonnement dans votre boutique mobile.
          </p>
        )}
        {providers.length > 0 && <CurrentSubscriptions providers={providers} />}
        {!store &&
          (plans ? (
            <Plans
              plans={plans}
              member={Boolean(access)}
              hideEmpty={currentStripe}
            />
          ) : (
            <p role="status">
              Les formules sont momentanément indisponibles. Réessayez
              ultérieurement.
            </p>
          ))}
        {tier && !store && (
          <Link className="text-link" href="/offres">
            Voir toutes les formules →
          </Link>
        )}
        <p>
          <Link href="/informations/conditions">
            Conditions du service et des abonnements
          </Link>
        </p>
      </section>
    </PublicShell>
  );
}
