import { Heading } from "../../../components/Shell";
import { SubmissionList } from "../../../components/SubmissionList";
import { availableCatalog, memberApi } from "../../../lib/api";
import { result } from "../../../lib/model";
export default async function Copies() {
  const { client } = await memberApi();
  const [copies, items] = await Promise.all([
    client.GET("/v1/submissions").then(result),
    availableCatalog(),
  ]);
  return (
    <>
      <Heading kicker="Espace élève" title="Mes copies">
        Retrouvez vos réponses et les retours de l’enseignante.
      </Heading>
      <SubmissionList
        items={copies.items}
        titles={new Map(items.map((item) => [item.id, item.title]))}
      />
    </>
  );
}
