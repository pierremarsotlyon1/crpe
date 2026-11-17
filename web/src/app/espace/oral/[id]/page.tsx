import Link from "next/link";
import { notFound } from "next/navigation";
import { OralPractice } from "../../../../components/Oral";
import { Heading } from "../../../../components/Shell";
import { memberApi } from "../../../../lib/api";
import { result } from "../../../../lib/model";
export default async function Session({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const [{ client }, { id }] = await Promise.all([memberApi(), params]);
  const item = result(await client.GET("/v1/oral-sessions")).items.find(
    (s) => s.id === id,
  );
  if (!item) notFound();
  return (
    <>
      <Link className="back" href="/espace/oral">
        ← Mes séances d’oral
      </Link>
      <Heading kicker="Oral" title={item.title} />
      <OralPractice initial={item} />
    </>
  );
}
