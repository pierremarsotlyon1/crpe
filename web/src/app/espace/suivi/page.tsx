import Link from "next/link";

import { Empty, Heading } from "../../../components/Shell";
import { memberApi, requireFeature } from "../../../lib/api";
import { result } from "../../../lib/model";
export default async function Support() {
  await requireFeature("live");
  const { client } = await memberApi();
  const support = result(await client.GET("/v1/support"));
  return (
    <>
      <Heading kicker="Espace élève" title="Mon accompagnement Suivi" />
      <div className="tiles">
        <section id="live" className="panel">
          <h2>Rendez-vous hebdomadaire</h2>
          <p>
            {support.liveSchedule ||
              "Le prochain horaire sera communiqué par l’enseignante."}
          </p>
          {support.liveUrl.startsWith("https://") && (
            <a
              href={support.liveUrl}
              className="button"
              target="_blank"
              rel="noreferrer"
            >
              Rejoindre la visioconférence ↗
            </a>
          )}
        </section>
        <section id="groupe" className="panel">
          <h2>Groupe WhatsApp</h2>
          {support.groupUrl.startsWith("https://chat.whatsapp.com/") ? (
            <a
              className="button secondary"
              href={support.groupUrl}
              target="_blank"
              rel="noreferrer"
            >
              Ouvrir l’invitation WhatsApp ↗
            </a>
          ) : (
            <p>L’enseignante partagera ici l’invitation au groupe.</p>
          )}
        </section>
      </div>
      <section id="videos" className="section">
        <h2>Mes corrections d’épreuves blanches</h2>
        {!support.mockReviews.length ? (
          <Empty>
            Vos épreuves blanches remises et les retours de l’enseignante
            apparaîtront ici.
          </Empty>
        ) : (
          support.mockReviews.map((review) => (
            <article className="panel" key={review.id}>
              <h3>{review.title}</h3>
              {review.feedback ? (
                <p className="preserve">{review.feedback}</p>
              ) : (
                <p>En attente du retour de l’enseignante.</p>
              )}
              {review.reviewVideoUrl.startsWith("https://") && (
                <a
                  className="button"
                  href={review.reviewVideoUrl}
                  target="_blank"
                  rel="noreferrer"
                >
                  Regarder ma correction vidéo ↗
                </a>
              )}{" "}
              <Link
                href={`/espace/epreuves/${review.id}`}
                className="text-link"
              >
                Revoir ma copie →
              </Link>
            </article>
          ))
        )}
      </section>
    </>
  );
}
