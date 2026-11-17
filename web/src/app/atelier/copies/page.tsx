import { Heading } from "../../../components/Shell";
import { SubmissionList } from "../../../components/SubmissionList";
import { adminApi, adminCatalog } from "../../../lib/api";
import { result } from "../../../lib/model";
export default async function Reviews() {
  const { client } = await adminApi();
  const [copies, items] = await Promise.all([
    client.GET("/v1/admin/submissions").then(result),
    adminCatalog(),
  ]);
  return (
    <>
      <Heading kicker="Atelier enseignant" title="Copies à relire">
        Consultez les réponses des élèves et transmettez un retour individuel.
      </Heading>
      <SubmissionList
        teacher
        items={copies.items}
        titles={new Map(items.map((item) => [item.id, item.title]))}
      />
    </>
  );
}
