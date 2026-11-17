import Link from "next/link";
import { notFound } from "next/navigation";
import { MockExam } from "../../../../components/MockExam";
import { Heading } from "../../../../components/Shell";
import { memberApi } from "../../../../lib/api";
import { result } from "../../../../lib/model";
export default async function Mock({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const [{ client }, { id }] = await Promise.all([memberApi(), params]);
  const reply = await client.GET("/v1/mock-sessions/{id}", {
    params: { path: { id } },
  });
  if (reply.response.status === 404) notFound();
  return (
    <>
      <Link className="back" href="/espace/epreuves">
        ← Sujets blancs
      </Link>
      <Heading title="Épreuve chronométrée" />
      <MockExam initial={result(reply)} />
    </>
  );
}
