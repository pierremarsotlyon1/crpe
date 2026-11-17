import Link from "next/link";
import { Empty, Heading } from "../../components/Shell";
import { adminApi, adminCatalog } from "../../lib/api";
import { kindNames, result } from "../../lib/model";
import { publicConfig } from "../../lib/public-site";

export default async function Dashboard() {
  const { client } = await adminApi();
  const [stats, items, config] = await Promise.all([
    client.GET("/v1/admin/stats").then(result),
    adminCatalog(),
    publicConfig(),
  ]);
  const drafts = items.filter((item) => item.status === "draft");
  return (
    <>
      <Heading kicker="Atelier enseignant" title="Vue d’ensemble">
        Suivez l’activité de la prépa et reprenez vos contenus en préparation.
      </Heading>
      <div className="metrics">
        {[
          [stats.users, "Comptes"],
          [stats.confirmedEmails, "Emails confirmés"],
          [stats.attempts, "Tentatives"],
          [stats.pendingReviews, "Copies à relire"],
          ...(config.salesEnabled
            ? [[stats.activeAccess, "Accès actifs"]]
            : []),
        ].map(([count, label]) => (
          <div key={label} className="metric">
            <strong>{count}</strong>
            <span>{label}</span>
          </div>
        ))}
      </div>
      <details className="panel">
        <summary>Usage du correcteur IA</summary>
        <dl>
          <dt>Corrections par fournisseur</dt>
          <dd>{stats.aiRuns}</dd>
          <dt>Jetons d’entrée et de sortie</dt>
          <dd>
            {stats.aiInputTokens.toLocaleString("fr-FR")} en entrée ·{" "}
            {stats.aiOutputTokens.toLocaleString("fr-FR")} en sortie
          </dd>
          <dt>Coût estimé</dt>
          <dd>
            {stats.estimatedCostUsd == null
              ? "Estimation indisponible"
              : new Intl.NumberFormat("fr-FR", {
                  style: "currency",
                  currency: "USD",
                  maximumFractionDigits: 4,
                }).format(stats.estimatedCostUsd)}
          </dd>
        </dl>
        {stats.aiUnpricedRuns > 0 && (
          <p>{stats.aiUnpricedRuns} correction(s) sans coût configuré.</p>
        )}
      </details>
      <section className="section">
        <div className="row-title">
          <h2>Brouillons en cours</h2>
          <Link className="button" href="/atelier/contenus/nouveau">
            Créer un contenu
          </Link>
        </div>
        {drafts.length === 0 ? (
          <Empty>
            Aucun brouillon en cours. Créez un chapitre pour commencer votre
            parcours.
          </Empty>
        ) : (
          <ul className="item-list">
            {drafts.slice(0, 8).map((item) => (
              <li key={item.id}>
                <Link href={`/atelier/contenus/${item.id}`}>
                  <span>
                    <small>{kindNames[item.kind]}</small>
                    {item.title}
                  </span>
                  <span aria-hidden="true">→</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>
      <Link className="tile" href="/atelier/copies">
        <span>
          <small>Suivi pédagogique</small>
          <strong>Relire les copies</strong>
        </span>
        <span aria-hidden="true">↗</span>
      </Link>
    </>
  );
}
