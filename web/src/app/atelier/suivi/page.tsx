import Link from "next/link";
import { Empty, Heading } from "../../../components/Shell";
import { SupportSettings } from "../../../components/Support";
import { adminApi } from "../../../lib/api";
import { dateLabel, result } from "../../../lib/model";
export default async function Support() {
  const { client } = await adminApi();
  const [settings, reviews] = await Promise.all([
    client.GET("/v1/admin/support").then(result),
    client.GET("/v1/admin/mock-sessions").then(result),
  ]);
  return (
    <>
      <Heading kicker="Atelier enseignant" title="Épreuves et suivi" />
      <SupportSettings settings={settings} />
      <section className="section">
        <h2>Épreuves blanches à relire</h2>
        {!reviews.items.length ? (
          <Empty>Aucune épreuve à relire pour le moment.</Empty>
        ) : (
          <ul className="item-list">
            {reviews.items.map((s) => (
              <li key={s.id}>
                <Link href={`/atelier/suivi/${s.id}`}>
                  <span>
                    <small>
                      {s.email} · {dateLabel(s.startedAt)}
                    </small>
                    {s.title}
                  </span>
                  <span className="status">
                    {s.gradingStatus === "pending_review"
                      ? "Réponse écrite à relire"
                      : s.feedback || s.reviewVideoUrl
                        ? "Retour transmis"
                        : "Accompagnement Suivi"}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  );
}
