import Link from "next/link";
import { EmailForm } from "../components/Identity";
import { PublicShell } from "../components/Shell";
import { publicConfig } from "../lib/public-site";

export const dynamic = "force-dynamic";
export default async function HomePage() {
  const { salesEnabled } = await publicConfig();
  return (
    <PublicShell>
      <div className="hero">
        <div className="hero-copy">
          <p className="overline">
            <span className="dot" />{" "}
            {salesEnabled ? "Préparation en ligne" : "Ouverture prochaine"}
          </p>
          <h1>
            Les maths du CRPE,
            <br />
            <em>pas à pas.</em>
          </h1>
          <p className="intro">
            Un espace pour préparer les mathématiques du concours de recrutement
            de professeurs des écoles, à votre rythme.
          </p>
          {salesEnabled ? (
            <section className="waitlist">
              <Link className="button" href="/offres">
                Découvrir les formules
              </Link>{" "}
              <Link className="text-link" href="/sujets">
                Consulter les sujets
              </Link>
            </section>
          ) : (
            <section className="waitlist">
              <h2>Être informé de l’ouverture</h2>
              <EmailForm waitlist />
            </section>
          )}
        </div>
        <div className="geometry" aria-hidden="true">
          <div className="graph-lines" />
          <div className="orbit" />
          <div className="triangle" />
          <div className="triangle inner" />
          <i className="point a" />
          <i className="point b" />
          <i className="point c" />
          <span className="coord x">x</span>
          <span className="coord y">y</span>
        </div>
      </div>
    </PublicShell>
  );
}
