import type { components } from "@crpe/api-client";
import { dateLabel } from "../lib/model";
import { Portal } from "./Billing";

type Subscription = components["schemas"]["ActiveProvider"];
const names = {
  stripe: "Stripe",
  apple: "App Store",
  google: "Google Play",
} as const;
function storeManagement(p: Subscription) {
  if (!p.managementUrl || !URL.canParse(p.managementUrl)) return;
  const link = new URL(p.managementUrl);
  const correct =
    p.provider === "apple"
      ? link.hostname === "apps.apple.com" &&
        link.pathname === "/account/subscriptions"
      : p.provider === "google" &&
        link.hostname === "play.google.com" &&
        link.pathname === "/store/account/subscriptions";
  if (
    link.protocol === "https:" &&
    !link.username &&
    !link.password &&
    !link.port &&
    correct
  )
    return link.href;
}
export function CurrentSubscriptions({
  providers,
}: {
  providers: Subscription[];
}) {
  return (
    <section className="section" aria-label="Abonnements actifs">
      {providers.map((p) => {
        const store = names[p.provider];
        const manage = storeManagement(p);
        return (
          <article
            className="panel"
            key={[p.provider, p.planKey, p.expiresAt].join("/")}
          >
            <p className="overline">{store}</p>
            <h2>Formule {p.tier === "suivi" ? "Suivi" : "Autonomie"}</h2>
            <p>Votre accès est confirmé jusqu’au {dateLabel(p.expiresAt)}.</p>
            {p.provider === "stripe" ? (
              <Portal />
            ) : (
              <>
                <p>
                  Les changements de formule et la résiliation se gèrent dans{" "}
                  {store}.
                </p>
                {manage && (
                  <a
                    className="button secondary"
                    href={manage}
                    rel="noreferrer"
                    target="_blank"
                  >
                    Gérer dans {store} ↗
                  </a>
                )}
              </>
            )}
          </article>
        );
      })}
    </section>
  );
}
