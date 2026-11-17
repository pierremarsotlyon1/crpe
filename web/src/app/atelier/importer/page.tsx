import { Importer } from "../../../components/Importer";
import { MockDraft } from "../../../components/MockDraft";
import { Heading } from "../../../components/Shell";
import { adminCatalog } from "../../../lib/api";
export default async function Import() {
  const records = await adminCatalog();
  const byId = new Map(records.map((row) => [row.id, row]));
  const bank = records.flatMap((item) => {
    if (item.kind !== "exercise") return [];
    let source = "";
    const seen = new Set<string>();
    for (
      let row: typeof item | undefined = item;
      row;
      row = row.parentId ? byId.get(row.parentId) : undefined
    ) {
      if (seen.has(row.id) || row.status !== "validated") return [];
      seen.add(row.id);
      source ||=
        row.sourceTitle || row.sourceUrl || row.documents[0]?.title || "";
      if (row.parentId && !byId.has(row.parentId)) return [];
    }
    return source ? [{ id: item.id, title: item.title, source }] : [];
  });
  const chapters = records
    .filter((i) => i.kind === "chapter")
    .map(({ id, title }) => ({ id, title }));
  return (
    <>
      <Heading kicker="Atelier enseignant" title="Importer des contenus" />
      <Importer parents={chapters} />
      <MockDraft bank={bank} />
    </>
  );
}
