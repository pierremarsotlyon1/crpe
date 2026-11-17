import Link from "next/link";
import { Empty, Heading } from "../../../components/Shell";
import { adminCatalog } from "../../../lib/api";
import { kindNames } from "../../../lib/model";
export default async function ContentLibrary({
  searchParams,
}: {
  searchParams: Promise<{ etat?: string; recherche?: string }>;
}) {
  const [items, query] = await Promise.all([adminCatalog(), searchParams]);
  const needle = (query.recherche ?? "").trim().toLocaleLowerCase("fr");
  const filtered = items.filter(
    (item) =>
      (!query.etat || item.status === query.etat) &&
      item.title.toLocaleLowerCase("fr").includes(needle),
  );
  return (
    <>
      <Heading
        kicker="Atelier enseignant"
        title="Contenus"
        action={
          <Link href="/atelier/contenus/nouveau" className="button">
            Créer un contenu
          </Link>
        }
      >
        Organisez les chapitres, les fiches et les exercices du parcours.
      </Heading>
      <form className="filters">
        <label>
          Rechercher
          <input
            name="recherche"
            type="search"
            defaultValue={needle}
            placeholder="Titre du contenu"
          />
        </label>
        <label>
          Statut
          <select name="etat" defaultValue={query.etat ?? ""}>
            <option value="">Tous les statuts</option>
            <option value="draft">Brouillons</option>
            <option value="validated">Validés</option>
          </select>
        </label>
        <button className="button secondary" type="submit">
          Filtrer
        </button>
      </form>
      {filtered.length === 0 ? (
        <Empty>
          {items.length
            ? "Aucun contenu ne correspond à ces filtres."
            : "Votre bibliothèque est vide. Commencez par créer un chapitre."}
        </Empty>
      ) : (
        <ul className="item-list">
          {filtered.map((item) => (
            <li key={item.id}>
              <Link href={`/atelier/contenus/${item.id}`}>
                <span>
                  <small>
                    {kindNames[item.kind]}
                    {item.skill && ` · ${item.skill}`}
                  </small>
                  {item.title}
                </span>
                <span className={`status ${item.status}`}>
                  {item.demo
                    ? "Démo · Brouillon"
                    : item.status === "draft"
                      ? "Brouillon"
                      : "Validé"}
                </span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
