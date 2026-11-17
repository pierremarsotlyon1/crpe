import Link from "next/link";
import { notFound } from "next/navigation";
import { Empty, Heading, PublicShell } from "../../components/Shell";
import { publicConfig, publicSubjects } from "../../lib/public-site";
export const dynamic = "force-dynamic";
export const metadata = { title: "Sujets de mathématiques du CRPE" };
export default async function Subjects() {
  if (!(await publicConfig()).salesEnabled) notFound();
  const items = await publicSubjects();
  return (
    <PublicShell>
      <section className="legal">
        <Heading title="Sujets de mathématiques du CRPE" />
        {items.length === 0 ? (
          <Empty>Aucun sujet publié actuellement.</Empty>
        ) : (
          <ul className="item-list">
            {items.map((i) => (
              <li key={i.id}>
                <Link href={`/sujets/${i.id}`}>{i.title}</Link>
              </li>
            ))}
          </ul>
        )}
      </section>
    </PublicShell>
  );
}
