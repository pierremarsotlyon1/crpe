import { LinkConfirmation } from "../../../components/Identity";
import { PublicShell } from "../../../components/Shell";
export const metadata = {
  title: "Confirmer la connexion",
  robots: { index: false },
};
export default function Verify() {
  return (
    <PublicShell>
      <section className="auth-panel">
        <p className="overline">Connexion</p>
        <h1>Bienvenue.</h1>
        <p className="lead">
          Confirmez l’ouverture de votre session sur cet appareil.
        </p>
        <LinkConfirmation mode="login" />
      </section>
    </PublicShell>
  );
}
