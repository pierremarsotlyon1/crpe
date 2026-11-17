import type { components } from "@crpe/api-client";
import Link from "next/link";
import { money } from "../lib/money";
import { Checkout } from "./Billing";
import { Empty } from "./Shell";
export function Plans({
  plans,
  member = false,
  hideEmpty = false,
}: {
  plans: components["schemas"]["Plan"][];
  member?: boolean;
  hideEmpty?: boolean;
}) {
  if (!plans.length)
    return hideEmpty ? null : (
      <Empty>Aucune formule disponible actuellement.</Empty>
    );
  return (
    <div className="tiles">
      {plans.map((plan) => (
        <section key={plan.key} className="panel">
          <h2>{plan.name}</h2>
          <p>
            {money(plan.amount, plan.currency)} par{" "}
            {plan.interval === "year" ? "an" : "mois"}
          </p>
          {member ? (
            <Checkout planKey={plan.key} />
          ) : (
            <Link href="/connexion" className="button">
              Se connecter pour choisir
            </Link>
          )}
        </section>
      ))}
    </div>
  );
}
