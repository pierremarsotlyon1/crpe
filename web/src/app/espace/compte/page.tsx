import Link from "next/link";
import { DeleteAccount } from "../../../components/DeleteAccount";
import { Heading } from "../../../components/Shell";
import { memberAccess, memberApi } from "../../../lib/api";
export default async function Account() {
  const [{ user }, access] = await Promise.all([memberApi(), memberAccess()]);
  const hasStore = access.providers?.some(
    (p) => p.provider === "apple" || p.provider === "google",
  );
  return (
    <>
      <Heading kicker="Espace élève" title="Mon compte" />
      <section className="panel">
        <h2>Mes informations</h2>
        <dl>
          <dt>Adresse email</dt>
          <dd>{user.email}</dd>
          <dt>Rôle</dt>
          <dd>{user.role === "admin" ? "Administration" : "Élève"}</dd>
        </dl>
        <a
          className="button secondary"
          download="mes-donnees-crpe.json"
          href="/api/v1/me/export"
        >
          Exporter mes données
        </a>
        {hasStore && (
          <div className="feedback">
            <p>
              Votre abonnement est géré par votre boutique mobile. Avant de
              supprimer votre compte, vous pouvez y désactiver le
              renouvellement.
            </p>
            <Link href="/espace/abonnement">Gérer mon abonnement →</Link>
          </div>
        )}
        <DeleteAccount />
      </section>
    </>
  );
}
