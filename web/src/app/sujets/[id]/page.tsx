import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { Markdown } from "../../../components/Markdown";
import { PublicShell } from "../../../components/Shell";
import { publicSubjects, siteUrl } from "../../../lib/public-site";
export const dynamic = "force-dynamic";
type Props = { params: Promise<{ id: string }> };
async function subject({ params }: Props) {
  const [{ id }, items] = await Promise.all([params, publicSubjects()]);
  const item = items.find((row) => row.id === id);
  if (!item) notFound();
  return item;
}
export async function generateMetadata(props: Props): Promise<Metadata> {
  const item = await subject(props);
  const base = siteUrl();
  const canonical = base ? new URL(`/sujets/${item.id}`, base) : undefined;
  return {
    title: item.title,
    description: item.body.slice(0, 160),
    alternates: { canonical },
    openGraph: {
      type: "article",
      title: item.title,
      description: item.body.slice(0, 160),
      url: canonical,
    },
  };
}
export default async function PublicSubject(props: Props) {
  const item = await subject(props);
  return (
    <PublicShell>
      <article className="legal">
        <p className="overline">Sujet de mathématiques · CRPE</p>
        <h1>{item.title}</h1>
        <Markdown>{item.body}</Markdown>
      </article>
    </PublicShell>
  );
}
