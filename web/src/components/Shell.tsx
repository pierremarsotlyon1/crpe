import Link from "next/link";
import type { ReactNode } from "react";
import type { Access } from "../lib/access";
import type { User } from "../lib/model";
import { Logout } from "./Identity";
import { WorkspaceNavigation } from "./WorkspaceNavigation";

export function Brand() {
  return (
    <Link href="/" className="brand" title="Accueil">
      <span className="brand-mark" aria-hidden="true">
        <i className="brand-letter" />
      </span>
      <span>
        Prépa Maths <b>CRPE</b>
      </span>
    </Link>
  );
}
export function PublicShell({ children }: { children: ReactNode }) {
  return (
    <div className="public-shell">
      <header className="masthead">
        <Brand />
        <Link href="/connexion" className="login-link">
          Mon espace <span aria-hidden="true">↗</span>
        </Link>
      </header>
      <main id="main-content">{children}</main>
      <footer>
        <span>Prépa Maths CRPE</span>
        <nav aria-label="Informations">
          <Link href="/informations/confidentialite">Confidentialité</Link>
          <Link href="/informations/mentions-legales">Mentions légales</Link>
          <Link href="/informations/cookies">Cookies</Link>
        </nav>
      </footer>
    </div>
  );
}
export function Workspace({
  user,
  teacher = false,
  salesEnabled = false,
  access,
  children,
}: {
  user: User;
  teacher?: boolean;
  salesEnabled?: boolean;
  access?: Access;
  children: ReactNode;
}) {
  return (
    <div className="workspace">
      <aside className="sidebar">
        <Brand />
        <div className="side-nav">
          <p className="overline">
            {teacher ? "Atelier enseignant" : "Espace élève"}
          </p>
          <WorkspaceNavigation
            teacher={teacher}
            salesEnabled={salesEnabled}
            access={access}
          />
        </div>
        <div className="identity">
          <small>{user.email}</small>
          {user.role === "admin" && (
            <Link href={teacher ? "/espace" : "/atelier"}>
              {teacher ? "Voir l’espace élève" : "Ouvrir l’atelier"} ↗
            </Link>
          )}
          <Logout />
        </div>
      </aside>
      <main className="desk" id="main-content">
        {children}
      </main>
    </div>
  );
}
export function Heading({
  kicker,
  title,
  children,
  action,
}: {
  kicker?: string;
  title: string;
  children?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <header className="section-heading">
      <div>
        {kicker && <p className="overline">{kicker}</p>}
        <h1>{title}</h1>
        {children && <div className="lead">{children}</div>}
      </div>
      {action}
    </header>
  );
}
export function Empty({ children }: { children: ReactNode }) {
  return <div className="empty">{children}</div>;
}
