import Link from "next/link";
import { Markdown } from "../../../components/Markdown";
import { StartMock } from "../../../components/MockExam";
import { Empty, Heading } from "../../../components/Shell";
import { catalog, memberApi, requireFeature } from "../../../lib/api";
import { dateLabel, result } from "../../../lib/model";
export default async function Mocks() {
  await requireFeature("mock");
  const { client } = await memberApi();
  const [all, history] = await Promise.all([
    catalog(),
    client.GET("/v1/mock-sessions").then(result),
  ]);
  const items = all.filter((i) => i.kind === "mock");
  return (
    <>
      <Heading kicker="Espace élève" title="Sujets blancs">
        Passez une épreuve chronométrée et consultez sa correction selon le
        barème de l’enseignante.
      </Heading>
      {items.length === 0 ? (
        <Empty>Aucun sujet blanc validé pour le moment.</Empty>
      ) : (
        items.map((item) => (
          <article className="panel" key={item.id}>
            <small>{item.durationMinutes} minutes</small>
            <h2>{item.title}</h2>
            <Markdown>{item.body}</Markdown>
            <StartMock subjectId={item.id} />
          </article>
        ))
      )}
      {history.items.length > 0 && (
        <section className="section">
          <h2>Mes épreuves remises</h2>
          <ul className="item-list">
            {history.items.map((s) => (
              <li key={s.id}>
                <Link href={`/espace/epreuves/${s.id}`}>
                  <span>
                    <small>{dateLabel(s.startedAt)}</small>
                    {s.title}
                  </span>
                  <span className="status">
                    {s.gradingStatus === "pending_review"
                      ? "Correction en cours"
                      : "Corrigée"}
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
