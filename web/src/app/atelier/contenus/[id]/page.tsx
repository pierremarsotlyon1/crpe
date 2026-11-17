import Link from "next/link";
import { notFound } from "next/navigation";
import { Editor } from "../../../../components/Editor";
import { Heading } from "../../../../components/Shell";
import { adminCatalog } from "../../../../lib/api";
import { kindNames } from "../../../../lib/model";
export default async function ContentPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ parent?: string }>;
}) {
  const [{ id }, items, query] = await Promise.all([
    params,
    adminCatalog(),
    searchParams,
  ]);
  const item = items.find((row) => row.id === id);
  if (id !== "nouveau" && !item) notFound();
  const parents = items
    .filter(
      (row) =>
        ["chapter", "lesson", "mock"].includes(row.kind) && row.id !== id,
    )
    .map(({ id, title, kind }) => ({ id, title, kind }));
  return (
    <>
      <Link className="back" href="/atelier/contenus">
        ← Bibliothèque
      </Link>
      <Heading
        kicker={item?.status === "validated" ? "Contenu validé" : "Brouillon"}
        title={item?.title ?? "Créer un contenu"}
      />
      {item && items.some((row) => row.parentId === id) && (
        <section className="panel">
          <h2>Contenus rattachés</h2>
          <ul className="item-list">
            {items
              .filter((row) => row.parentId === id)
              .map((row) => (
                <li key={row.id}>
                  <Link href={`/atelier/contenus/${row.id}`}>
                    <span>
                      <small>{kindNames[row.kind]}</small>
                      {row.title}
                    </span>
                    <span className="status">
                      {row.status === "draft" ? "Brouillon" : "Validé"}
                    </span>
                  </Link>
                </li>
              ))}
          </ul>
        </section>
      )}
      <Editor
        key={item?.id ?? "new"}
        item={item}
        parents={parents}
        initialParent={query.parent}
      />
    </>
  );
}
