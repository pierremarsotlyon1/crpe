import Link from "next/link";
import { notFound } from "next/navigation";
import { SubmitCopy } from "../../../../components/Copies";
import { LessonVideo } from "../../../../components/LessonVideo";
import { Markdown } from "../../../../components/Markdown";
import { Exercise } from "../../../../components/Practice";
import { Empty, Heading } from "../../../../components/Shell";
import { catalog } from "../../../../lib/api";
import { type Content, formatNames, kindNames } from "../../../../lib/model";

function Lesson({ item }: { item: Content }) {
  return (
    <>
      <Markdown>{item.body}</Markdown>
      {item.videoUrl && (
        <>
          <LessonVideo source={item.videoUrl} title={item.title} />
          {item.videoDescription && (
            <Markdown>{item.videoDescription}</Markdown>
          )}
        </>
      )}
    </>
  );
}
export default async function Chapter({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const [{ id }, items] = await Promise.all([params, catalog()]);
  const item = items.find((row) => row.id === id);
  if (!item || item.kind === "mock" || item.kind === "oral") notFound();
  const children = items.filter((row) => row.parentId === id);
  const parent = items.find((row) => row.id === item.parentId);
  if (parent?.kind === "mock") notFound();
  return (
    <>
      <Link
        href={parent ? `/espace/chapitres/${parent.id}` : "/espace"}
        className="back"
      >
        ← {parent?.title ?? "Mon parcours"}
      </Link>
      <Heading kicker={kindNames[item.kind]} title={item.title}>
        {item.skill}
      </Heading>
      {item.kind === "exercise" ? (
        <div className="panel">
          <p className="overline">{formatNames[item.format]}</p>
          {item.format === "text" ? (
            <>
              <Markdown>{item.body}</Markdown>
              <SubmitCopy exerciseId={id} />
            </>
          ) : (
            <>
              <Exercise item={item} />
              <details>
                <summary>Envoyer une copie pour relecture</summary>
                <SubmitCopy exerciseId={id} />
              </details>
            </>
          )}
        </div>
      ) : (
        <div className="panel">
          <Lesson item={item} />
        </div>
      )}
      {(item.documents.length > 0 || item.sourceUrl) && (
        <section className="panel">
          <h2>Documents du cours</h2>
          <ul>
            {item.documents.map((doc) => (
              <li key={doc.id}>
                <a
                  href={`/api/v1/documents/${doc.id}/file`}
                  target="_blank"
                  rel="noreferrer"
                >
                  {doc.title} (PDF) ↗
                </a>
              </li>
            ))}
            {item.sourceUrl?.startsWith("https://") && (
              <li>
                <a href={item.sourceUrl} target="_blank" rel="noreferrer">
                  {item.sourceTitle || "Sujet original"} {item.sourceYear || ""}{" "}
                  ↗
                </a>
              </li>
            )}
          </ul>
        </section>
      )}
      {item.kind !== "exercise" && (
        <section className="section">
          <h2>{item.kind === "chapter" ? "Dans ce chapitre" : "Exercices"}</h2>
          {children.length === 0 ? (
            <Empty>Aucun contenu complémentaire publié pour le moment.</Empty>
          ) : (
            <ul className="item-list">
              {children.map((child) => (
                <li key={child.id}>
                  <Link href={`/espace/chapitres/${child.id}`}>
                    <span>
                      <small>{kindNames[child.kind]}</small>
                      {child.title}
                    </span>
                    <span aria-hidden="true">→</span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
    </>
  );
}
