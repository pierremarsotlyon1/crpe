import { notFound } from "next/navigation";
import { LinkConfirmation } from "../../../components/Identity";
import { PublicShell } from "../../../components/Shell";
export const metadata = { title: "Liste d’attente", robots: { index: false } };
export default async function Waitlist({
  params,
}: {
  params: Promise<{ action: string }>;
}) {
  const { action } = await params;
  if (action !== "confirmer" && action !== "desinscrire") notFound();
  return (
    <PublicShell>
      <section className="auth-panel">
        <p className="overline">Liste d’attente</p>
        <h1>
          {action === "confirmer"
            ? "Confirmer l’inscription"
            : "Se désinscrire"}
        </h1>
        <p className="lead">
          {action === "confirmer"
            ? "Confirmez que vous souhaitez recevoir les nouvelles de l’ouverture."
            : "Confirmez le retrait de votre adresse de la liste d’attente."}
        </p>
        <LinkConfirmation
          mode={action === "confirmer" ? "confirm" : "unsubscribe"}
        />
      </section>
    </PublicShell>
  );
}
