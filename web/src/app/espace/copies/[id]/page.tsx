import Link from "next/link";
import { notFound } from "next/navigation";
import { Heading } from "../../../../components/Shell";
import { SubmissionList } from "../../../../components/SubmissionList";
import { availableCatalog, memberApi } from "../../../../lib/api";
import { result } from "../../../../lib/model";
export default async function Copy({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ envoi?: string }>;
}) {
  const [{ client }, { id }, query] = await Promise.all([
    memberApi(),
    params,
    searchParams,
  ]);
  const [reply, items] = await Promise.all([
    client.GET("/v1/submissions/{id}", { params: { path: { id } } }),
    availableCatalog(),
  ]);
  if (reply.response.status === 404) notFound();
  return (
    <>
      <Link href="/espace/copies" className="back">
        ← Mes copies
      </Link>
      <Heading kicker="Espace élève" title="Suivi de ma copie" />
      {query.envoi === "1" && (
        <p className="feedback">Votre copie a été envoyée.</p>
      )}
      <SubmissionList
        single
        items={[result(reply)]}
        titles={new Map(items.map((i) => [i.id, i.title]))}
      />
    </>
  );
}
