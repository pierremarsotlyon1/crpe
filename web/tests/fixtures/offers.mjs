// Isolated Stripe display fixtures. Go owns all session and entitlement checks.
import { createServer } from "node:http";

const plans = {
  items: [
    {
      key: "auto-test",
      name: "Autonomie de test",
      tier: "autonomie",
      amount: 1000,
      currency: "eur",
      interval: "month",
    },
    {
      key: "suivi-test",
      name: "Suivi de test",
      tier: "suivi",
      amount: 2000,
      currency: "eur",
      interval: "month",
    },
  ],
};
function readFixture(port, upstream, history) {
  createServer(async (req, res) => {
    if (
      req.method !== "GET" ||
      !req.url?.startsWith("/") ||
      req.url.startsWith("//")
    ) {
      res.writeHead(405).end();
      return;
    }
    const options = {
      headers: { cookie: req.headers.cookie || "" },
      signal: AbortSignal.timeout(5000),
    };
    try {
      if (!history && req.url === "/v1/billing/plans") {
        res
          .writeHead(200, { "Content-Type": "application/json" })
          .end(JSON.stringify(plans));
        return;
      }
      if (history && req.url === "/v1/billing/invoices") {
        const identity = await fetch(`${upstream}/v1/me`, options);
        if (!identity.ok) {
          res.writeHead(identity.status).end();
          return;
        }
        const member = await identity.json();
        const items = member.email.startsWith("historiquefacture-")
          ? [
              {
                id: "in_fixture",
                status: "paid",
                currency: "eur",
                amount: 1000,
                created: 1700000000,
                url: "https://invoice.stripe.com/i/test-fixture",
              },
            ]
          : [];
        res
          .writeHead(200, { "Content-Type": "application/json" })
          .end(JSON.stringify({ items }));
        return;
      }
      const response = await fetch(upstream + req.url, options);
      res
        .writeHead(response.status, {
          "Content-Type":
            response.headers.get("Content-Type") || "application/json",
        })
        .end(Buffer.from(await response.arrayBuffer()));
    } catch {
      res.writeHead(503).end();
    }
  }).listen(port, "0.0.0.0");
}
readFixture(8080, "http://sales-api:8080", false);
readFixture(8081, "http://api:8080", true);
