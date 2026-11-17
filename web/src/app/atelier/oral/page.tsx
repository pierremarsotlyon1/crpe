import { OralFeedback, OralReview } from "../../../components/Oral";
import { Empty, Heading } from "../../../components/Shell";
import { adminApi } from "../../../lib/api";
import { dateLabel, result } from "../../../lib/model";
export default async function Reviews() {
  const { client } = await adminApi();
  const items = result(
    await client.GET("/v1/admin/oral-sessions"),
  ).items.filter((s) => s.status !== "active");
  return (
    <>
      <Heading kicker="Atelier enseignant" title="Prestations orales" />
      {!items.length ? (
        <Empty>Aucune prestation orale à relire pour le moment.</Empty>
      ) : (
        items.map((s) => (
          <article className="panel" key={s.id}>
            <small>{dateLabel(s.createdAt)}</small>
            <h2>{s.title}</h2>
            <h3>Exposé</h3>
            <p className="preserve">{s.transcript}</p>
            {s.questions.map((q, i) => (
              <div key={q}>
                <h3>{q}</h3>
                <p>{s.answers[i]}</p>
              </div>
            ))}
            {s.status === "reviewed" ? (
              <OralFeedback items={s.feedback} source={s.feedbackSource} />
            ) : (
              <OralReview session={s} />
            )}
          </article>
        ))
      )}
    </>
  );
}
