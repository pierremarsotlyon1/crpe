import Link from "next/link";
import { notFound, redirect } from "next/navigation";
import { Heading } from "../../../../components/Shell";
import { featurePages, lockedDestination } from "../../../../lib/access";
import { memberAccess } from "../../../../lib/api";
export default async function FeatureNotice({
  params,
}: {
  params: Promise<{ feature: string }>;
}) {
  const [{ feature }, access] = await Promise.all([params, memberAccess()]);
  const item = access.features.find((f) => f.key === feature);
  if (!item) notFound();
  if (item.allowed) redirect(featurePages[item.key]);
  const destination = lockedDestination(item, access.salesEnabled);
  if (destination.startsWith("/offres?")) redirect(destination);
  return (
    <>
      <Heading kicker="Espace élève" title={item.label} />
      <section className="panel">
        <h2>
          Accès {item.requiredTier === "suivi" ? "Suivi" : "Autonomie"} requis
        </h2>
        <p>
          Cette activité est réservée aux élèves disposant de cet
          accompagnement. L’ouverture de nouveaux accès est actuellement fermée.
        </p>
        <Link className="text-link" href="/espace/compte">
          Retrouver mon compte →
        </Link>
      </section>
    </>
  );
}
