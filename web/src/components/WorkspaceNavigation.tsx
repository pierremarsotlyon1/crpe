"use client";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  type Access,
  type Feature,
  featurePages,
  lockedDestination,
} from "../lib/access";

type Entry = { href: string; label: string; feature?: Feature["key"] };
const teacherLinks: Entry[] = [
  { href: "/atelier", label: "Vue d’ensemble" },
  { href: "/atelier/contenus", label: "Contenus" },
  { href: "/atelier/importer", label: "Importer" },
  { href: "/atelier/copies", label: "Copies à relire" },
  { href: "/atelier/oral", label: "Prestations orales" },
  { href: "/atelier/suivi", label: "Épreuves et suivi" },
];
const memberLinks: Entry[] = [
  { href: "/espace", label: "Mon parcours" },
  { href: featurePages.course, label: "Mes cours", feature: "course" },
  { href: featurePages.practice, label: "Entraînement", feature: "practice" },
  {
    href: "/espace/pratique/diagnostic",
    label: "Test de niveau",
    feature: "practice",
  },
  { href: "/espace/progression", label: "Ma progression" },
  { href: featurePages.tutor, label: "Tuteur de correction", feature: "tutor" },
  { href: "/espace/copies", label: "Mes copies" },
  { href: featurePages.mock, label: "Sujets blancs", feature: "mock" },
  { href: featurePages.oral, label: "Préparer l’oral", feature: "oral" },
  {
    href: featurePages.live,
    label: "Rendez-vous hebdomadaire",
    feature: "live",
  },
  {
    href: featurePages.mock_video,
    label: "Corrections vidéo",
    feature: "mock_video",
  },
  { href: featurePages.group, label: "Groupe WhatsApp", feature: "group" },
  { href: "/espace/compte", label: "Mon compte" },
];
export function WorkspaceNavigation({
  teacher,
  salesEnabled,
  access,
}: {
  teacher: boolean;
  salesEnabled: boolean;
  access?: Access;
}) {
  const path = usePathname();
  return (
    <nav aria-label={teacher ? "Administration" : "Espace membre"}>
      {(teacher ? teacherLinks : memberLinks).map((entry) => {
        const permission = access?.features.find(
          (f) => f.key === entry.feature,
        );
        const locked = permission && !permission.allowed;
        const href = locked
          ? lockedDestination(permission, salesEnabled)
          : entry.href;
        const active =
          path === href ||
          (!locked &&
            entry.href.split("/").length > 2 &&
            path.startsWith(`${entry.href}/`));
        return (
          <Link
            key={entry.href}
            href={href}
            className={locked ? "locked-link" : undefined}
            aria-current={active ? "page" : undefined}
          >
            <span>{entry.label}</span>
            {locked && (
              <>
                <svg
                  width="14"
                  height="16"
                  viewBox="0 0 16 18"
                  fill="none"
                  aria-hidden="true"
                >
                  <rect
                    x="2.5"
                    y="8"
                    width="11"
                    height="8"
                    rx="2"
                    stroke="currentColor"
                    strokeWidth="1.5"
                  />
                  <path
                    d="M5 8V5a3 3 0 0 1 6 0v3"
                    stroke="currentColor"
                    strokeWidth="1.5"
                  />
                </svg>
                <span className="sr-only">
                  {" "}
                  · accès{" "}
                  {permission.requiredTier === "suivi" ? "Suivi" : "Autonomie"}{" "}
                  requis
                </span>
              </>
            )}
          </Link>
        );
      })}
      {(salesEnabled || !teacher) && (
        <Link href={teacher ? "/atelier/paiements" : "/espace/abonnement"}>
          {teacher ? "Paiements" : "Mon abonnement"}
        </Link>
      )}
    </nav>
  );
}
