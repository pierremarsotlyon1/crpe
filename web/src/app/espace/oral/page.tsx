import Link from "next/link";
import { StartOral } from "../../../components/Oral";
import { OralTrends } from "../../../components/OralTrends";
import { Empty, Heading } from "../../../components/Shell";
import { catalog, memberApi, requireFeature } from "../../../lib/api";
import { dateLabel, result } from "../../../lib/model";
export default async function Oral() {
  await requireFeature("oral");
  const { client } = await memberApi();
  const [items, sessions] = await Promise.all([
    catalog(),
    client.GET("/v1/oral-sessions").then(result),
  ]);
  return (
    <>
      <Heading kicker="Espace élève" title="Préparer l’oral">
        Préparez votre exposé et répondez aux questions du jury.
      </Heading>
      {items.some((i) => i.kind === "oral") ? (
        <section className="panel">
          <h2>Nouvelle séance</h2>
          <StartOral />
        </section>
      ) : (
        <Empty>Aucun sujet d’oral validé pour le moment.</Empty>
      )}
      <OralTrends
        sessions={sessions.items.filter((s) => s.status === "reviewed")}
      />
      {sessions.items.length > 0 && (
        <section className="section">
          <h2>Mes séances</h2>
          <ul className="item-list">
            {sessions.items.map((s) => (
              <li key={s.id}>
                <Link href={`/espace/oral/${s.id}`}>
                  <span>
                    <small>{dateLabel(s.createdAt)}</small>
                    {s.title}
                  </span>
                  <span className="status">
                    {s.status === "active"
                      ? "En cours"
                      : s.status === "reviewed"
                        ? "Évaluée"
                        : "En relecture"}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  );
}
