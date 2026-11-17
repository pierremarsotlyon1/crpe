import { expect, test } from "@playwright/test";
import { adminEmail, login, newestLink } from "./mail";
import { grantTestSupport, recordStoreFixture } from "./stack";

test("waiting list confirmation and unsubscribe travel through email", async ({
  page,
  request,
}) => {
  const email = `liste-${crypto.randomUUID()}@example.test`;
  await page.goto("/");
  if (process.env.E2E_CAPTURE_PUBLIC === "true") {
    await page.setViewportSize({ width: 1440, height: 980 });
    await page.reload({ waitUntil: "networkidle" });
    await page.screenshot({
      path: test.info().outputPath("accueil-bureau.png"),
      fullPage: true,
    });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.reload({ waitUntil: "networkidle" });
    await page.screenshot({
      path: test.info().outputPath("accueil-mobile.png"),
      fullPage: true,
    });
  }
  await expect(
    page.getByRole("link", { name: /acheter|paiement|tarifs/i }),
  ).toHaveCount(0);
  await page.getByLabel("Adresse email").fill(email);
  await page.getByRole("checkbox").check();
  const after = Date.now();
  await page.getByRole("button", { name: "M’inscrire à la liste" }).click();
  await expect(
    page.getByText(
      "Consultez votre boîte mail pour confirmer votre inscription.",
    ),
  ).toBeVisible();
  await page.goto(
    await newestLink(request, email, "/liste-attente/confirmer", after),
  );
  await page.getByRole("button", { name: "Confirmer mon inscription" }).click();
  await expect(
    page.getByText("Votre inscription est confirmée."),
  ).toBeVisible();
  await page.goto(
    await newestLink(request, email, "/liste-attente/desinscrire", after),
  );
  await page.getByRole("button", { name: "Me désinscrire" }).click();
  await expect(
    page.getByText("Votre adresse a été désinscrite."),
  ).toBeVisible();
});

test("teacher validates a chapter, lesson and exercise and reviews the learner's copy", async ({
  page,
  request,
  browser,
  baseURL,
}) => {
  const run = crypto.randomUUID().slice(0, 8);
  const chapter = `Test navigateur · chapitre ${run}`;
  const lesson = `Test navigateur · fiche ${run}`;
  const exercise = `Test navigateur · exercice ${run}`;
  await login(page, request, adminEmail);
  const student = await browser.newContext({ baseURL });
  const learner = await student.newPage();
  try {
    if (process.env.E2E_CAPTURE_PUBLIC === "true") {
      await page.setViewportSize({ width: 1440, height: 980 });
      await page.screenshot({
        path: test.info().outputPath("atelier-bureau.png"),
        fullPage: true,
      });
    }
    await page.getByRole("link", { name: "Créer un contenu" }).click();
    await page.getByLabel("Titre", { exact: true }).fill(chapter);
    await page
      .getByLabel("Présentation", { exact: true })
      .fill("Donnée de test du parcours navigateur.");
    await page
      .getByRole("button", { name: "Enregistrer le brouillon" })
      .click();
    await expect(page.getByRole("heading", { name: chapter })).toBeVisible();
    const fixturePage = await page.context().newPage();
    await fixturePage.setContent(
      "<article><h1>Support PDF de test</h1><p>Donnée de vérification du stockage des pièces jointes.</p></article>",
    );
    const attachment = await fixturePage.pdf({ format: "A5" });
    await fixturePage.close();
    await page.getByLabel("Titre du document").fill("Support PDF de test");
    await page.getByLabel("Document PDF").setInputFiles({
      name: "support-test.pdf",
      mimeType: "application/pdf",
      buffer: attachment,
    });
    await page.getByRole("button", { name: "Joindre le PDF" }).click();
    await expect(page.getByText("PDF joint au brouillon.")).toBeVisible();
    await login(learner, request, `eleve-${run}@example.test`);
    await expect(learner.getByRole("link", { name: chapter })).toHaveCount(0);
    await learner.goto("/atelier");
    await expect(learner).toHaveURL(/\/espace$/);
    await page.getByRole("button", { name: "Valider le contenu" }).click();
    await expect(
      page.getByText("Contenu validé", { exact: true }).first(),
    ).toBeVisible();
    const chapterId = new URL(page.url()).pathname.split("/").at(-1);
    await page.getByRole("link", { name: /Ajouter un contenu/ }).click();
    await expect(
      page.getByRole("heading", { name: "Créer un contenu" }),
    ).toBeVisible();
    await expect(page.getByLabel("Type de contenu")).toHaveValue("lesson");
    await page.getByLabel("Titre", { exact: true }).fill(lesson);
    await page
      .getByLabel("Fiche et consignes")
      .fill("Fiche de test avec $x^2$.");
    await page.getByRole("button", { name: "Afficher l’aperçu" }).click();
    await expect(page.locator(".katex")).toBeVisible();
    await page
      .getByRole("button", { name: "Enregistrer le brouillon" })
      .click();
    await expect(page.getByRole("heading", { name: lesson })).toBeVisible();
    await page.getByRole("button", { name: "Valider le contenu" }).click();
    await expect(
      page.getByText("Contenu validé", { exact: true }).first(),
    ).toBeVisible();
    await page.goto(`/atelier/contenus/nouveau?parent=${chapterId}`);
    await page.getByLabel("Type de contenu").selectOption("exercise");
    await page.getByLabel("Titre", { exact: true }).fill(exercise);
    await page
      .getByLabel("Énoncé", { exact: true })
      .fill("Donnée de test. Calculez $2+2$.");
    await page.getByLabel("Notion travaillée").fill("Calcul de test");
    await page.getByLabel("Type de réponse").selectOption("numeric");
    await page.getByLabel("Résultat numérique attendu").fill("4");
    await page
      .getByLabel("Corrigé détaillé")
      .fill("Le résultat de test est 4.");
    await page
      .getByLabel("Critères de correction, un par ligne")
      .fill("Résultat exact");
    await page
      .getByRole("button", { name: "Enregistrer le brouillon" })
      .click();
    await expect(page.getByRole("heading", { name: exercise })).toBeVisible();
    await page.getByRole("button", { name: "Valider le contenu" }).click();
    await expect(
      page.getByText("Contenu validé", { exact: true }).first(),
    ).toBeVisible();
    await learner.goto("/espace");
    await learner.getByRole("link", { name: new RegExp(chapter) }).click();
    const pdfUrl = await learner
      .getByRole("link", { name: "Support PDF de test (PDF) ↗" })
      .getAttribute("href");
    expect(pdfUrl).toBeTruthy();
    const pdf = await learner.request.get(pdfUrl || "");
    expect(pdf.ok()).toBe(true);
    expect(pdf.headers()["content-type"]).toContain("application/pdf");
    expect(pdf.headers()["content-security-policy"]).toContain("sandbox");
    await learner.getByRole("link", { name: new RegExp(lesson) }).click();
    await expect(learner.locator(".katex")).toBeVisible();
    await learner.goto(`/espace/chapitres/${chapterId}`);
    await learner.getByRole("link", { name: new RegExp(exercise) }).click();
    await learner.getByLabel("Votre réponse numérique").fill("4");
    await learner.getByRole("button", { name: "Vérifier ma réponse" }).click();
    await expect(
      learner.getByRole("heading", { name: "Réponse correcte" }),
    ).toBeVisible();
    await learner.getByText("Envoyer une copie pour relecture").click();
    await learner
      .getByLabel("Réponse rédigée", { exact: true })
      .fill("Texte de copie de test.");
    await learner.getByRole("button", { name: "Envoyer ma copie" }).click();
    await expect(learner.getByText(/Votre copie a été envoyée/)).toBeVisible();
    await page.goto("/atelier/copies");
    const submission = page
      .locator("article")
      .filter({ has: page.getByRole("heading", { name: exercise }) });
    await submission
      .getByLabel("Retour de l’enseignante")
      .fill("Retour humain de test.");
    await submission
      .getByRole("button", { name: "Envoyer mon retour" })
      .click();
    await expect(
      submission.getByText("Correction terminée", { exact: true }),
    ).toBeVisible();
    await learner.goto("/espace/copies");
    await expect(learner.getByText("Retour humain de test.")).toBeVisible();
    await learner.goto("/espace/progression");
    await expect(learner.getByText("Calcul de test").first()).toBeVisible();
  } finally {
    await student.close();
  }
});

test("CSV import, timed exam, and a teacher-reviewed oral", async ({
  page,
  request,
  browser,
  baseURL,
}) => {
  const run = crypto.randomUUID().slice(0, 8);
  await login(page, request, adminEmail);
  await page.goto("/atelier/importer");
  await page.getByLabel("Fichier Markdown ou CSV").setInputFiles({
    name: "exercice-test.csv",
    mimeType: "text/csv",
    buffer: Buffer.from(
      `titre;enonce;format;notion;reponse;corrige\nTest CSV ${run};Calculez 3 + 4.;numerique;Calcul;7;Résultat de test.\n`,
    ),
  });
  await page.getByRole("button", { name: "Prévisualiser le fichier" }).click();
  await expect(page.getByText(`Test CSV ${run}`)).toBeVisible();
  await page.getByRole("button", { name: "Importer les brouillons" }).click();
  await expect(
    page.getByText("Import terminé.", { exact: false }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Ouvrir la bibliothèque" }).click();
  await expect(
    page.getByRole("link", { name: new RegExp(`Test CSV ${run}.*Brouillon`) }),
  ).toBeVisible();
  const endpoint = "/api/v1/admin/content";
  const origin = new URL(baseURL || "http://localhost:3000").origin;
  const headers = { origin };
  async function create(body: Record<string, unknown>, validate = true) {
    const response = await page.request.post(endpoint, {
      headers,
      data: { ...body, body: "Contenu synthétique de test navigateur." },
    });
    expect(response.ok()).toBe(true);
    const item = (await response.json()) as { id: string };
    if (validate) {
      const publish = await page.request.post(
        `${endpoint}/${item.id}/validate`,
        { headers },
      );
      expect(publish.ok()).toBe(true);
    }
    return item.id;
  }
  const mock = `Test épreuve ${run}`;
  const subjectId = await create({
    kind: "mock",
    title: mock,
    durationMinutes: 2,
    sourceTitle: "Sujets synthétiques réservés au test",
  });
  const question = `Question chronométrée ${run}`;
  await create({
    kind: "exercise",
    title: question,
    parentId: subjectId,
    format: "numeric",
    skill: "Calcul de test",
    answer: "7",
    solution: "Le résultat est 7.",
    points: 3,
  });
  const written = `Justification écrite ${run}`;
  await create({
    kind: "exercise",
    title: written,
    parentId: subjectId,
    format: "text",
    skill: "Justifier",
    solution: "Expliquer les étapes du calcul.",
    criteria: ["Étapes explicites"],
    points: 4,
  });
  const oralTitle = `Test oral ${run}`;
  const oralId = await create({
    kind: "oral",
    title: oralTitle,
    criteria: ["Clarté de l’exposé"],
    choices: ["Comment avez-vous procédé ?", "Comment vérifier le résultat ?"],
    prepMinutes: 1,
    durationMinutes: 1,
  });
  await page.goto("/atelier/importer");
  await page
    .getByText("Préparer un sujet blanc avec l’IA", { exact: true })
    .click();
  await page.getByLabel("Mode de composition").selectOption("bank");
  await page.getByRole("checkbox", { name: new RegExp(question) }).check();
  await page.getByRole("checkbox", { name: new RegExp(written) }).check();
  await page
    .getByLabel("Titre du sujet proposé")
    .fill(`Assemblage de test ${run}`);
  await page
    .getByLabel("Notion ou consigne de génération")
    .fill("Composer une épreuve de révision.");
  await page.getByLabel("Nombre d’exercices").fill("2");
  const draftReply = page.waitForResponse(
    (r) =>
      r.url().endsWith("/admin/mock-drafts") && r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "Générer les brouillons" }).click();
  expect((await draftReply).status()).toBe(503);
  await expect(page.getByText(/indisponible/i).last()).toBeVisible();
  const student = await browser.newContext({ baseURL });
  const learner = await student.newPage();
  try {
    await login(learner, request, `entrainement-${run}@example.test`);
    await learner
      .getByRole("link", { name: /Rendez-vous hebdomadaire.*Suivi requis/ })
      .click();
    await expect(
      learner.getByRole("heading", { name: "Accès Suivi requis" }),
    ).toBeVisible();
    await expect(learner.locator('a[href*="/offres"]')).toHaveCount(0);
    expect((await learner.request.get("/api/v1/support")).status()).toBe(403);
    if (process.env.E2E_CAPTURE_PUBLIC === "true") {
      await learner.setViewportSize({ width: 1440, height: 980 });
      await learner.reload({ waitUntil: "networkidle" });
      await expect(learner.locator(".workspace:visible")).toHaveCount(1);
      await expect(learner.locator(".sidebar:visible")).toHaveCount(1);
      await expect(learner.locator("main:visible")).toHaveCount(1);
      await learner.screenshot({
        path: test.info().outputPath("espace-verrouille.png"),
        fullPage: false,
      });
    }
    await learner.setViewportSize({ width: 1024, height: 580 });
    await learner
      .getByRole("button", { name: "Se déconnecter" })
      .scrollIntoViewIfNeeded();
    await expect(
      learner.getByRole("button", { name: "Se déconnecter" }),
    ).toBeInViewport();
    await learner.setViewportSize({ width: 1280, height: 800 });
    if (process.env.E2E_SALES_URL) {
      const sales = process.env.E2E_SALES_URL;
      await learner.goto(`${sales}/espace`);
      await expect(
        learner.getByRole("heading", { name: "Mon parcours" }),
      ).toBeVisible();
      await expect(
        learner.getByRole("link", { name: /Mes cours.*Autonomie requis/ }),
      ).toBeVisible();
      await learner.goto(`${sales}/espace/progression`);
      await expect(
        learner.getByRole("heading", { name: "Ma progression" }),
      ).toBeVisible();
      await learner
        .getByRole("link", { name: /Rendez-vous hebdomadaire.*Suivi requis/ })
        .click();
      await expect(learner).toHaveURL(/offres\?feature=live&tier=suivi/);
      await expect(
        learner.getByRole("heading", { name: "Suivi de test" }),
      ).toBeVisible();
      await expect(
        learner.getByRole("heading", { name: "Autonomie de test" }),
      ).toHaveCount(0);
      await expect(
        learner.getByRole("button", { name: "Choisir cette formule" }),
      ).toBeVisible();
      await learner
        .getByRole("link", { name: "Voir toutes les formules →" })
        .click();
      await expect(
        learner.getByRole("heading", { name: "Autonomie de test" }),
      ).toBeVisible();
      // The card page must recheck ownership if the backend reports a purchase already in progress.
      await learner.route(
        "**/api/v1/billing/checkout",
        (route) =>
          route.fulfill({
            status: 409,
            contentType: "application/json",
            body: JSON.stringify({
              error: "Un abonnement est en cours de confirmation.",
            }),
          }),
        { times: 1 },
      );
      await learner
        .getByRole("button", { name: "Choisir cette formule" })
        .first()
        .click();
      await learner
        .getByRole("link", { name: "Consulter mon abonnement →" })
        .click();
      await expect(
        learner.getByRole("heading", { name: "Mon abonnement" }),
      ).toBeVisible();
    }
    await learner.goto(new URL("/espace/tuteur", origin).href);
    await expect(
      learner.getByRole("link", { name: new RegExp(question) }),
    ).toHaveCount(0);
    await learner.goto(new URL("/espace/epreuves", origin).href);
    await learner
      .locator("article")
      .filter({ has: learner.getByRole("heading", { name: mock }) })
      .getByRole("button", { name: "Commencer l’épreuve" })
      .click();
    await learner.getByLabel(`Votre réponse pour ${question}`).fill("7");
    await expect(learner.getByText("Réponses enregistrées.")).toBeVisible();
    await learner.reload();
    await expect(
      learner.getByLabel(`Votre réponse pour ${question}`),
    ).toHaveValue("7");
    await learner
      .getByLabel(`Votre réponse pour ${written}`)
      .fill("J'explicite les étapes de mon raisonnement.");
    await learner.getByRole("button", { name: "Remettre ma copie" }).click();
    await expect(
      learner.getByRole("heading", { name: "Résultat de l’épreuve" }),
    ).toBeVisible();
    await expect(
      learner.getByRole("heading", { name: "Correction écrite en cours" }),
    ).toBeVisible();
    await expect(learner.locator(".score")).toHaveCount(0);
    const mockSessionURL = learner.url();
    await page.goto("/atelier/suivi");
    await page.getByRole("link", { name: new RegExp(mock) }).click();
    await expect(page.getByLabel("Lien de votre correction vidéo")).toHaveCount(
      0,
    );
    await page.getByLabel(`Points pour ${written}`).fill("3");
    await page
      .getByLabel(`Appréciation pour ${written}`)
      .fill("Les étapes sont explicites.");
    await page.getByRole("button", { name: "Transmettre mon retour" }).click();
    await expect(
      page.getByText("Le retour est disponible dans l’espace de l’élève."),
    ).toBeVisible();
    await learner.reload();
    await expect(learner.locator(".score")).toHaveText("6 / 7 points");
    await expect(
      learner.getByText("Les étapes sont explicites."),
    ).toBeVisible();
    const profile = (await (
      await learner.request.get("/api/v1/me")
    ).json()) as { id: string };
    grantTestSupport(profile.id);
    await page.goto("/atelier/suivi");
    await page
      .getByLabel("Horaire hebdomadaire et fuseau")
      .fill("Mercredi 18 h · Europe/Paris");
    await page
      .getByLabel("Lien de la visioconférence")
      .fill("https://example.invalid/live-test");
    await page
      .getByLabel("Invitation du groupe WhatsApp")
      .fill("https://chat.whatsapp.com/e2e-fixture");
    await page
      .getByRole("button", { name: "Enregistrer les informations" })
      .click();
    await expect(
      page.getByText("Informations de la formule Suivi enregistrées."),
    ).toBeVisible();
    await page.getByRole("link", { name: new RegExp(mock) }).click();
    await page
      .getByLabel("Commentaires sur l’épreuve")
      .fill("Retour individuel de test.");
    await page
      .getByLabel("Lien de votre correction vidéo")
      .fill("https://example.invalid/relecture-test");
    await page.getByRole("button", { name: "Transmettre mon retour" }).click();
    await expect(
      page.getByText("Le retour est disponible dans l’espace de l’élève."),
    ).toBeVisible();
    await learner.goto("/espace/suivi");
    await expect(
      learner.getByText("Mercredi 18 h · Europe/Paris"),
    ).toBeVisible();
    await expect(
      learner.getByRole("link", { name: /Rejoindre la visioconférence/ }),
    ).toHaveAttribute("href", "https://example.invalid/live-test");
    await expect(
      learner.getByRole("link", { name: /Ouvrir l’invitation WhatsApp/ }),
    ).toHaveAttribute("href", "https://chat.whatsapp.com/e2e-fixture");
    await expect(
      learner.getByRole("link", { name: /Regarder ma correction vidéo/ }),
    ).toHaveAttribute("href", "https://example.invalid/relecture-test");
    await learner.goto("/espace/epreuves");
    await learner
      .getByRole("link", { name: new RegExp(`${mock}.*Corrigée`) })
      .click();
    await expect(learner).toHaveURL(mockSessionURL);
    const started = await learner.request.post("/api/v1/oral-sessions", {
      headers,
      data: { subjectId: oralId },
    });
    expect(started.ok()).toBe(true);
    const oral = (await started.json()) as { id: string };
    await learner.goto(`/espace/oral/${oral.id}`);
    await learner.getByRole("button", { name: "Commencer mon exposé" }).click();
    await learner
      .getByLabel("Transcription de l’exposé")
      .fill("Transcription synthétique saisie pour le test.");
    await learner
      .getByLabel("Réponse à la question 1")
      .fill("Réponse de test un.");
    await learner
      .getByLabel("Réponse à la question 2")
      .fill("Réponse de test deux.");
    await learner
      .getByRole("button", { name: "Remettre ma prestation" })
      .click();
    await expect(
      learner.getByRole("heading", {
        name: /Oral remis pour relecture|Retour par critère/,
      }),
    ).toBeVisible();
    await page.goto("/atelier/oral");
    const item = page
      .locator("article")
      .filter({ has: page.getByRole("heading", { name: oralTitle }) });
    await item.getByLabel("Appréciation").selectOption("validated");
    await item.getByLabel("Commentaire").fill("Critère relu pendant le test.");
    await item
      .getByRole("button", { name: "Transmettre l’évaluation" })
      .click();
    await expect(item.getByText("Validé", { exact: true })).toBeVisible();
    await learner.reload();
    await expect(
      learner.getByText("Critère relu pendant le test."),
    ).toBeVisible();
    const inputs = Array.from({ length: 20 }, (_, i) => ({
      kind: "exercise",
      title: `Diagnostic de test ${run} · ${i + 1}`,
      body: "Question synthétique. Saisissez 7.",
      skill: `Notion de test ${run} · ${i % 3}`,
      format: "numeric",
      answer: "7",
      solution: "La valeur de test est 7.",
      position: i,
    }));
    const batch = await page.request.post("/api/v1/admin/import", {
      headers,
      data: { clientId: crypto.randomUUID(), items: inputs },
    });
    expect(batch.ok()).toBe(true);
    const created = (await batch.json()) as { items: { id: string }[] };
    for (const row of created.items) {
      const valid = await page.request.post(`${endpoint}/${row.id}/validate`, {
        headers,
      });
      expect(valid.ok()).toBe(true);
    }
    await learner.goto("/espace/pratique/diagnostic");
    for (let n = 1; n <= 20; n++) {
      await expect(
        learner.getByText(`Question ${n} sur 20`, { exact: true }),
      ).toBeVisible();
      if (await learner.getByRole("radio").count()) {
        await learner.getByRole("radio").first().check();
      } else {
        await learner.getByLabel("Votre réponse numérique").fill("7");
      }
      await learner
        .getByRole("button", { name: "Vérifier ma réponse" })
        .click();
      await learner
        .getByRole("button", {
          name: n === 20 ? "Terminer la séance" : "Question suivante",
        })
        .click();
    }
    await expect(
      learner.getByRole("heading", { name: "Séance terminée" }),
    ).toBeVisible();
    await learner.getByRole("link", { name: "Voir ma progression" }).click();
    await expect(
      learner.getByRole("heading", { name: "Planning de révision" }),
    ).toBeVisible();
    await expect(
      learner
        .locator(".metric")
        .filter({ hasText: "Réponses données" })
        .locator("strong"),
    ).toHaveText("20");
  } finally {
    await student.close();
  }
});

for (const platform of ["google", "apple"] as const) {
  test(`web recognizes an existing ${platform} subscription across sales settings`, async ({
    page,
    request,
  }) => {
    const sales = process.env.E2E_SALES_URL;
    if (!sales)
      throw new Error("Run the isolated sales-enabled browser fixture.");
    await login(
      page,
      request,
      `${platform}-web-${crypto.randomUUID()}@example.test`,
    );
    const me = (await (await page.request.get("/api/v1/me")).json()) as {
      id: string;
    };
    recordStoreFixture(me.id, platform);
    const label = platform === "apple" ? "App Store" : "Google Play";
    const link =
      platform === "apple"
        ? "https://apps.apple.com/account/subscriptions"
        : "https://play.google.com/store/account/subscriptions";
    await page.goto("/espace/compte");
    await page
      .getByRole("link", { name: "Mon abonnement", exact: true })
      .click();
    await expect(
      page.getByRole("link", { name: `Gérer dans ${label} ↗` }),
    ).toHaveAttribute("href", link);
    await expect(
      page.getByRole("button", { name: "Choisir cette formule" }),
    ).toHaveCount(0);
    await page.goto(`${sales}/offres?feature=live&tier=suivi`);
    await expect(
      page.getByRole("heading", { name: "Votre abonnement" }),
    ).toBeVisible();
    await expect(
      page.getByRole("link", { name: `Gérer dans ${label} ↗` }),
    ).toHaveAttribute("href", link);
    await expect(
      page.getByRole("button", { name: "Choisir cette formule" }),
    ).toHaveCount(0);
    const account = await page.request.get(`${sales}/api/v1/me/access`);
    expect((await account.json()).providers).toEqual([
      expect.objectContaining({ provider: platform, tier: "autonomie" }),
    ]);
  });
}

test("closed sales retain member access to historical invoices", async ({
  page,
  request,
}) => {
  const history = process.env.E2E_HISTORY_URL;
  if (!history) throw new Error("Run the isolated invoice fixture.");
  await login(
    page,
    request,
    `historiquefacture-${crypto.randomUUID()}@example.test`,
  );
  await page.goto(`${history}/espace/compte`);
  await page.getByRole("link", { name: "Mon abonnement", exact: true }).click();
  await expect(
    page.getByText("Aucun abonnement actif sur ce compte."),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Consulter la facture ↗" }),
  ).toHaveAttribute("href", "https://invoice.stripe.com/i/test-fixture");
  await expect(
    page.getByRole("button", { name: "Gérer mon abonnement" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Choisir cette formule" }),
  ).toHaveCount(0);
  await expect(page.locator('a[href*="/offres"]')).toHaveCount(0);
});
