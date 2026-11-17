import Link from "next/link";
import { Empty, Heading } from "../../../components/Shell";
import { lockedDestination } from "../../../lib/access";
import { availableCatalog, memberAccess, memberApi } from "../../../lib/api";
import { dateLabel, result } from "../../../lib/model";
export default async function Progress() {
  const [{ client }, access] = await Promise.all([memberApi(), memberAccess()]);
  const practice = access.features.find((f) => f.key === "practice");
  const locked = practice && !practice.allowed;
  const [p, plan, items] = await Promise.all([
    client.GET("/v1/progress").then(result),
    locked ? null : client.GET("/v1/revision-plan").then(result),
    availableCatalog(),
  ]);
  const titles = new Map(items.map((item) => [item.id, item.title]));
  return (
    <>
      <Heading kicker="Espace élève" title="Ma progression">
        Suivez vos résultats et retrouvez votre planning personnalisé.
      </Heading>
      <div className="metrics">
        {[
          [p.attempts, "Réponses données"],
          [p.correct, "Réponses correctes"],
          [p.streak, "Jours de série"],
        ].map(([n, label]) => (
          <div className="metric" key={label}>
            <strong>{n}</strong>
            <span>{label}</span>
          </div>
        ))}
      </div>
      <section className="section">
        <div className="row-title">
          <h2>Planning de révision</h2>
          <Link
            className="button secondary"
            href={
              locked
                ? lockedDestination(practice, access.salesEnabled)
                : "/espace/pratique/revision"
            }
          >
            {locked ? "Accès Autonomie requis" : "Commencer une révision"}
          </Link>
        </div>
        {!plan ? (
          <Empty>
            Le planning personnalisé nécessite un accès Autonomie ou Suivi.
          </Empty>
        ) : plan.weeks.length === 0 ? (
          <Empty>Aucune notion à planifier pour le moment.</Empty>
        ) : (
          plan.weeks.map((week) => (
            <section className="panel" key={week.week}>
              <h3>Semaine {week.week}</h3>
              <p>{week.skills.join(" · ")}</p>
              <ul>
                {week.exerciseIds.map((id) => (
                  <li key={id}>
                    <Link href={`/espace/chapitres/${id}`}>
                      {titles.get(id) || "Ouvrir l’exercice"}
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          ))
        )}
      </section>
      {p.skills.length > 0 && (
        <section className="section">
          <h2>Résultats par notion</h2>
          <ul className="plain-list">
            {p.skills.map((skill) => (
              <li key={skill.skill}>
                <strong>{skill.skill}</strong>
                <span>
                  {skill.correct} sur {skill.attempts} réussies · Revoir le{" "}
                  {dateLabel(skill.nextReviewAt)}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  );
}
