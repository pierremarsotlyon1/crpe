import { parse } from "csv-parse/browser/esm/sync";
import type { ContentInput } from "./model";
export function parseContentFile(
  filename: string,
  source: string,
  parentId?: string,
): ContentInput[] {
  const parent = parentId ? { parentId } : {};
  if (/\.md$/i.test(filename)) {
    const title = /^# +(.+?)\s*$/m.exec(source);
    const body = title ? source.replace(title[0], "").trim() : source.trim();
    if (!body) throw new Error("Le fichier Markdown ne contient pas de fiche.");
    return [
      {
        kind: "lesson",
        title: title?.[1]?.trim() || filename.replace(/\.md$/i, ""),
        body,
        ...parent,
      },
    ];
  }
  if (!/\.csv$/i.test(filename))
    throw new Error("Choisissez un fichier .md ou .csv.");
  const header = source.split(/\r?\n/, 1)[0] ?? "";
  const rows = parse(source, {
    bom: true,
    columns: true,
    delimiter: header.includes(";") ? ";" : ",",
    skip_empty_lines: true,
    trim: true,
    max_record_size: 65000,
  }) as Record<string, string>[];
  if (!rows.length || rows.length > 100)
    throw new Error("Le CSV doit contenir entre 1 et 100 exercices.");
  return rows.map((row, index) => {
    if (!row.titre || !row.enonce)
      throw new Error(
        `Ligne ${index + 2} : renseignez les colonnes titre et enonce.`,
      );
    const formats = {
      qcm: "choice",
      numerique: "numeric",
      redaction: "text",
    } as const;
    const format = formats[(row.format ?? "redaction") as keyof typeof formats];
    if (!format)
      throw new Error(
        `Ligne ${index + 2} : format inconnu. Utilisez qcm, numerique ou redaction.`,
      );
    const tolerance = Number((row.tolerance || "0").replace(",", "."));
    if (!Number.isFinite(tolerance) || tolerance < 0)
      throw new Error(`Ligne ${index + 2} : tolérance invalide.`);
    return {
      kind: "exercise",
      format,
      title: row.titre,
      body: row.enonce,
      skill: row.notion ?? "",
      answer: row.reponse ?? "",
      solution: row.corrige ?? "",
      position: index + 1,
      tolerance,
      choices: (row.choix ?? "")
        .split("|")
        .map((s) => s.trim())
        .filter(Boolean),
      criteria: (row.criteres ?? "")
        .split("|")
        .map((s) => s.trim())
        .filter(Boolean),
      ...parent,
    };
  });
}
