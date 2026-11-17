import Link from "next/link";
import { Empty, Heading } from "../../components/Shell";
import { lockedDestination } from "../../lib/access";
import { availableCatalog, memberAccess, memberApi } from "../../lib/api";
import { kindNames, result } from "../../lib/model";
export default async function Member() {
  const { client } = await memberApi();
  const [items, progress, access] = await Promise.all([
    availableCatalog(),
    client.GET("/v1/progress").then(result),
    memberAccess(),
  ]);
  const course = access.features.find((f) => f.key === "course");
  const practice = access.features.find((f) => f.key === "practice");
  const topLevel = items.filter(
    (item) =>
      !item.parentId && ["chapter", "lesson", "exercise"].includes(item.kind),
  );
  return (
    <>
      <Heading kicker="Espace élève" title="Mon parcours">
        Retrouvez vos chapitres et entraînez-vous à votre rythme.
      </Heading>
      <div className="tiles">
        <Link
          className="tile dark"
          href={
            practice && !practice.allowed
              ? lockedDestination(practice, access.salesEnabled)
              : "/espace/pratique/quotidien"
          }
        >
          <span>
            <small>
              {practice && !practice.allowed
                ? "Accès Autonomie requis"
                : "Pratique régulière"}
            </small>
            <strong>Entraînement du jour</strong>
          </span>
          <span aria-hidden="true">↗</span>
        </Link>
        <Link className="tile" href="/espace/progression">
          <span>
            <small>
              {progress.attempts} tentative{progress.attempts > 1 ? "s" : ""}{" "}
              enregistrée{progress.attempts > 1 ? "s" : ""}
            </small>
            <strong>Ma progression</strong>
          </span>
          <span aria-hidden="true">→</span>
        </Link>
      </div>
      <section id="cours" className="section">
        <h2>Parcours de mathématiques</h2>
        {course && !course.allowed ? (
          <div className="panel">
            <p>
              Les chapitres et les fiches de cours nécessitent un accès
              Autonomie ou Suivi.
            </p>
            <Link
              className="text-link"
              href={lockedDestination(course, access.salesEnabled)}
            >
              Consulter les conditions d’accès →
            </Link>
          </div>
        ) : topLevel.length === 0 ? (
          <Empty>
            Le premier chapitre apparaîtra ici après validation par
            l’enseignante.
          </Empty>
        ) : (
          <ul className="item-list">
            {topLevel.map((item) => (
              <li key={item.id}>
                <Link href={`/espace/chapitres/${item.id}`}>
                  <span>
                    <small>
                      {kindNames[item.kind]}
                      {item.skill && ` · ${item.skill}`}
                    </small>
                    {item.title}
                  </span>
                  <span aria-hidden="true">→</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>
    </>
  );
}
