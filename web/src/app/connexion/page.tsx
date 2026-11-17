import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { EmailForm } from "../../components/Identity";
import { PublicShell } from "../../components/Shell";
import { createServerAPIClient } from "../../lib/api";
export const metadata = { title: "Connexion", robots: { index: false } };
export default async function Login() {
  const session = (await cookies()).get("crpe_session")?.value;
  if (session) {
    const current = await createServerAPIClient(session).GET("/v1/me");
    if (current.data)
      redirect(current.data.role === "admin" ? "/atelier" : "/espace");
  }
  return (
    <PublicShell>
      <section className="auth-panel">
        <p className="overline">Espace personnel</p>
        <h1>Se connecter</h1>
        <p className="lead">
          Recevez un lien de connexion personnel dans votre boîte mail.
        </p>
        <EmailForm />
      </section>
    </PublicShell>
  );
}
