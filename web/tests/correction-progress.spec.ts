import { execFileSync, spawn } from "node:child_process";
import { once } from "node:events";
import { expect, test } from "@playwright/test";
import { adminEmail, login } from "./mail";

import { ownedContainer } from "./stack";

async function lockSubmission(database: string, id: string) {
  if (!/^[0-9a-f-]{36}$/i.test(id)) throw new Error("Invalid submission ID.");
  const sql = spawn(
    "docker",
    [
      "exec",
      "-i",
      database,
      "psql",
      "-X",
      "-qAt",
      "-v",
      "ON_ERROR_STOP=1",
      "-U",
      "crpe",
      "-d",
      "crpe",
    ],
    { stdio: ["pipe", "pipe", "pipe"] },
  );
  let output = "";
  sql.stdout.setEncoding("utf8");
  sql.stdout.on("data", (chunk) => {
    output += chunk;
  });
  const exit = once(sql, "exit");
  const release = async () => {
    if (sql.exitCode === null) sql.stdin.end("COMMIT;\n\\q\n");
    await exit;
  };
  try {
    sql.stdin.write(
      `BEGIN; SET LOCAL idle_in_transaction_session_timeout='45s'; SELECT id FROM submissions WHERE id='${id}' FOR UPDATE;\n`,
    );
    await expect.poll(() => output, { timeout: 8000 }).toContain(id);
  } catch (error) {
    await release();
    throw error;
  }
  return release;
}

test("the learner sees real queued, running, completed and teacher-review stages", async ({
  page,
  request,
  browser,
  baseURL,
}) => {
  const project = process.env.E2E_COMPOSE_PROJECT;
  if (!project || !/^crpe-browser-(\d+|test)$/.test(project))
    throw new Error(
      "Run scripts/browser-e2e.sh to provide an owned, disposable worker and database.",
    );
  const worker = ownedContainer(project, "worker");
  const db = ownedContainer(project, "postgres");
  await login(page, request, adminEmail);
  const headers = {
    origin: new URL(baseURL || "http://localhost:3000").origin,
  };
  const create = await page.request.post("/api/v1/admin/content", {
    headers,
    data: {
      kind: "exercise",
      format: "numeric",
      title: `Suivi de test ${crypto.randomUUID().slice(0, 8)}`,
      body: "Donnée de test. Calculez 3 + 4.",
      skill: "Suivi test",
      answer: "7",
      solution: "Le résultat est 7.",
    },
  });
  expect(create.ok()).toBe(true);
  const exercise = (await create.json()) as { id: string };
  const valid = await page.request.post(
    `/api/v1/admin/content/${exercise.id}/validate`,
    { headers },
  );
  expect(valid.ok()).toBe(true);
  const context = await browser.newContext({ baseURL });
  const learner = await context.newPage();
  let paused = false;
  let release: (() => Promise<void>) | undefined;
  try {
    await login(learner, request, `suivi-${crypto.randomUUID()}@example.test`);
    async function submit(text: string) {
      await learner.goto(`/espace/chapitres/${exercise.id}`);
      await learner.getByText("Envoyer une copie pour relecture").click();
      await learner.getByLabel("Réponse rédigée", { exact: true }).fill(text);
      await learner.getByRole("button", { name: "Envoyer ma copie" }).click();
      await expect(learner).toHaveURL(/\/espace\/copies\/[0-9a-f-]+/);
      const id = new URL(learner.url()).pathname.split("/").at(-1);
      if (!id) throw new Error("Missing copy ID.");
      return id;
    }
    execFileSync("docker", ["pause", worker]);
    paused = true;
    const id = await submit("7");
    await expect(
      learner.getByText("En file d’attente", { exact: true }),
    ).toBeVisible();
    // A real row lock keeps the claimed worker inside the grader until the browser sees it run.
    release = await lockSubmission(db, id);
    execFileSync("docker", ["unpause", worker]);
    paused = false;
    await expect(
      learner.getByText("Correction en cours", { exact: true }),
    ).toBeVisible();
    await release();
    release = undefined;
    await expect(
      learner.getByText("Correction terminée", { exact: true }),
    ).toBeVisible();
    await expect(
      learner.getByText("Vérification numérique", { exact: true }),
    ).toBeVisible();
    await submit("Ma copie est illisible.");
    await expect(
      learner.getByText("En attente de l’enseignante", { exact: true }),
    ).toBeVisible();
    await expect(learner.getByText(/Cette copie a été signalée/)).toBeVisible();
  } finally {
    if (release) await release();
    if (paused) execFileSync("docker", ["unpause", worker]);
    await context.close();
  }
});
