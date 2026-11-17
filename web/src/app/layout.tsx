import type { Metadata } from "next";
import type { ReactNode } from "react";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "Prépa Maths CRPE — Liste d’attente",
    template: "%s · Prépa Maths CRPE",
  },
  description:
    "Préparation aux mathématiques du CRPE. Inscrivez-vous pour être informé de l’ouverture.",
  openGraph: {
    type: "website",
    locale: "fr_FR",
    siteName: "Prépa Maths CRPE",
    title: "Prépa Maths CRPE",
    description: "Préparer les mathématiques du CRPE, pas à pas.",
  },
  referrer: "no-referrer",
};
export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="fr">
      <body>
        <a href="#main-content" className="skip">
          Aller au contenu
        </a>
        {children}
      </body>
    </html>
  );
}
