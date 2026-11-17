import Link from "next/link";
import { notFound } from "next/navigation";
import { Markdown } from "../../../../components/Markdown";
import { Heading } from "../../../../components/Shell";
import { TeacherMockReview } from "../../../../components/Support";
import { adminApi } from "../../../../lib/api";
import { result } from "../../../../lib/model";
export default async function Review({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const [{ id }, { client }] = await Promise.all([params, adminApi()]);
  const [reply, queue] = await Promise.all([
    client.GET("/v1/admin/mock-sessions/{id}", { params: { path: { id } } }),
    client.GET("/v1/admin/mock-sessions").then(result),
  ]);
  if (reply.response.status === 404) notFound();
  const session = result(reply);
  return (
    <>
      <Link href="/atelier/suivi" className="back">
        ← Épreuves et suivi
      </Link>
      <Heading kicker="Atelier enseignant" title="Relire l’épreuve blanche" />
      <section className="panel">
        <h2>Copie remise</h2>
        <p>
          {session.gradingStatus === "pending_review"
            ? "Correction partielle"
            : "Note finale"}{" "}
          · {session.score} / {session.maxScore} points
        </p>
        {session.items.map((item) => {
          const grade = session.results.find(
            (row) => row.exerciseId === item.id,
          );
          return (
            <details key={item.id}>
              <summary>{item.title}</summary>
              <Markdown>{item.body}</Markdown>
              <h3>Réponse de l’élève</h3>
              <p className="preserve">
                {session.answers[item.id] || "Aucune réponse remise."}
              </p>
              {grade && (
                <>
                  <h3>Corrigé de référence</h3>
                  <Markdown>{grade.feedback}</Markdown>
                  <p>
                    {grade.points === undefined
                      ? "À relire"
                      : `${grade.points} / ${grade.maxPoints} points`}
                  </p>
                </>
              )}
            </details>
          );
        })}
      </section>
      <TeacherMockReview
        session={session}
        supportAccess={queue.items.some(
          (item) => item.id === id && item.supportAccess,
        )}
      />
    </>
  );
}
