import { notFound } from "next/navigation";
import { PracticeSession } from "../../../../components/Practice";
import { Empty, Heading } from "../../../../components/Shell";
import { memberApi } from "../../../../lib/api";
import { result } from "../../../../lib/model";

const modes = {
  quotidien: {
    id: "daily",
    title: "Entraînement du jour",
    intro: "Retrouvez les notions à revoir dans une courte séance.",
  },
  diagnostic: {
    id: "diagnostic",
    title: "Test de niveau",
    intro: "Répondez aux 20 questions pour situer vos acquis.",
  },
  revision: {
    id: "revision",
    title: "Réviser mes notions",
    intro: "Reprenez les exercices proposés selon vos résultats.",
  },
} as const;
export default async function Practice({
  params,
}: {
  params: Promise<{ mode: string }>;
}) {
  const { mode } = await params;
  if (mode !== "quotidien" && mode !== "diagnostic" && mode !== "revision")
    notFound();
  const config = modes[mode];
  const { client } = await memberApi();
  const data = result(
    await client.GET("/v1/practice", {
      params: { query: { mode: config.id } },
    }),
  );
  const items = data.items.filter(
    (item) => item.kind === "exercise" && item.format !== "text",
  );
  return (
    <>
      <Heading kicker="Espace élève" title={config.title}>
        {config.intro}
      </Heading>
      {mode === "diagnostic" && items.length < 20 ? (
        <Empty>
          Le test sera disponible dès que 20 questions auront été validées par
          l’enseignante.
        </Empty>
      ) : items.length ? (
        <PracticeSession items={items} />
      ) : (
        <Empty>
          Aucun exercice à proposer pour cette séance pour le moment.
        </Empty>
      )}
    </>
  );
}
