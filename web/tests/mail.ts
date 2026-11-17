import { type APIRequestContext, expect, type Page } from "@playwright/test";

const mailRoot = process.env.E2E_MAIL_URL || "http://127.0.0.1:8025";
export const adminEmail =
  process.env.E2E_ADMIN_EMAIL || "enseignante@example.test";

export async function newestLink(
  request: APIRequestContext,
  email: string,
  action: string,
  after: number,
) {
  let link = "";
  await expect
    .poll(
      async () => {
        const inbox = await request.get(`${mailRoot}/api/v1/search`, {
          params: { query: `to:${email}`, limit: 15 },
        });
        if (!inbox.ok()) return false;
        const data = (await inbox.json()) as {
          messages: { ID: string; Created: string }[];
        };
        for (const message of data.messages) {
          if (new Date(message.Created).getTime() < after - 1000) continue;
          const detail = await request.get(
            `${mailRoot}/api/v1/message/${message.ID}`,
          );
          if (!detail.ok()) continue;
          const content = (await detail.json()) as {
            Text: string;
            HTML: string;
          };
          const matches =
            (content.Text || content.HTML).match(/https?:\/\/[^\s"'<>]+/g) ||
            [];
          const url = matches
            .map((value) => new URL(value))
            .find(
              (url) => url.pathname === action && url.hash.includes("token="),
            );
          if (url) {
            expect(url.origin).toBe(
              new URL(process.env.E2E_WEB_URL || "http://localhost:3000")
                .origin,
            );
            link = url.href;
            return true;
          }
        }
        return false;
      },
      { timeout: 35000, intervals: [300, 600, 1000] },
    )
    .toBe(true);
  return link;
}

export async function login(
  page: Page,
  request: APIRequestContext,
  email: string,
) {
  await page.goto("/connexion");
  await page.getByLabel("Adresse email").fill(email);
  const after = Date.now();
  await page.getByRole("button", { name: "Recevoir mon lien" }).click();
  await expect(
    page.getByText(
      "Consultez votre boîte mail pour ouvrir votre lien personnel.",
    ),
  ).toBeVisible();
  await page.goto(
    await newestLink(request, email, "/connexion/confirmer", after),
  );
  await page.getByRole("button", { name: "Ouvrir mon espace" }).click();
  await expect(page).toHaveURL(/\/(atelier|espace)$/);
}
