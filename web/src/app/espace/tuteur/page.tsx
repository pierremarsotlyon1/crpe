import Link from "next/link";
import { Empty, Heading } from "../../../components/Shell";
import { catalog, requireFeature } from "../../../lib/api";
export default async function Tutor() {
  await requireFeature("tutor");
  const all = await catalog();
  const subjects = new Set(
    all.filter((i) => i.kind === "mock").map((i) => i.id),
  );
  const items = all.filter(
    (i) => i.kind === "exercise" && (!i.parentId || !subjects.has(i.parentId)),
  );
  return (
    <>
      <Heading kicker="Espace élève" title="Tuteur de correction">
        Choisissez un exercice pour envoyer votre démarche rédigée ou une photo
        de votre copie.
      </Heading>
      <Link className="text-link" href="/espace/copies">
        Retrouver mes corrections →
      </Link>
      {items.length ? (
        <ul className="item-list">
          {items.map((i) => (
            <li key={i.id}>
              <Link href={`/espace/chapitres/${i.id}`}>
                <span>
                  <small>{i.skill}</small>
                  {i.title}
                </span>
                <span aria-hidden="true">→</span>
              </Link>
            </li>
          ))}
        </ul>
      ) : (
        <Empty>Aucun exercice validé pour le moment.</Empty>
      )}
    </>
  );
}
