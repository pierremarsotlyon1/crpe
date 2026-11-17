import { notFound } from "next/navigation";
import { PublicShell } from "../../../components/Shell";

const pages: Record<string, { title: string; sections: [string, string][] }> = {
  "mentions-legales": {
    title: "Mentions légales",
    sections: [
      [
        "Éditeur",
        "À compléter avant publication avec l’identité de l’éditeur, ses coordonnées et les mentions d’immatriculation applicables.",
      ],
      [
        "Publication et hébergement",
        "À compléter avec le responsable de publication et les coordonnées de l’hébergeur retenu.",
      ],
    ],
  },
  confidentialite: {
    title: "Confidentialité",
    sections: [
      [
        "Liste d’attente",
        "Votre adresse email sert à vous informer de l’ouverture après confirmation par email. Le message contient un lien de désinscription.",
      ],
      [
        "Espace personnel",
        "L’espace personnel utilise votre adresse email, vos contenus et vos réponses pour assurer la connexion et le suivi pédagogique. Vous pouvez exporter vos données et demander la suppression de votre compte dans Mon compte.",
      ],
      [
        "Informations à compléter",
        "L’éditeur doit préciser son identité, les coordonnées de contact, les bases de traitement, les destinataires, les durées de conservation et les modalités d’exercice des droits avant la publication du service.",
      ],
    ],
  },
  conditions: {
    title: "Conditions du service et des abonnements",
    sections: [
      [
        "Accès au service",
        "À compléter par l’éditeur avec son identité, les conditions d’accès, les formules, le contenu fourni et les canaux de contact.",
      ],
      [
        "Abonnements",
        "Le projet de conditions doit préciser, pour chaque canal proposé, la durée, le renouvellement, le prix affiché avant engagement et les modalités de résiliation, de rétractation et de remboursement applicables.",
      ],
      [
        "Publication",
        "L’éditeur doit relire et publier les conditions définitives avant l’ouverture des souscriptions.",
      ],
    ],
  },
  cookies: {
    title: "Cookies",
    sections: [
      [
        "Cookie de session",
        "Lorsque vous vous connectez, le cookie crpe_session permet au serveur de reconnaître votre session. Vous pouvez fermer cette session avec Se déconnecter.",
      ],
      [
        "Informations à compléter",
        "L’éditeur doit vérifier la liste et les durées des cookies réellement utilisés sur son hébergement avant publication.",
      ],
    ],
  },
};
export const metadata = { robots: { index: false } };
export default async function Info({
  params,
}: {
  params: Promise<{ page: string }>;
}) {
  const info = pages[(await params).page];
  if (!info) notFound();
  return (
    <PublicShell>
      <article className="legal">
        <p className="overline">Gabarit à compléter avant publication</p>
        <h1>{info.title}</h1>
        {info.sections.map(([name, description]) => (
          <section key={name}>
            <h2>{name}</h2>
            <p>{description}</p>
          </section>
        ))}
      </article>
    </PublicShell>
  );
}
