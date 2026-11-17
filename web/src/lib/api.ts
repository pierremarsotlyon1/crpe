import "server-only";
import { createAPIClient } from "@crpe/api-client";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { cache } from "react";
import { type Feature, lockedDestination } from "./access";
import { result } from "./model";

export function apiOrigin() {
  return process.env.API_INTERNAL_URL || "http://127.0.0.1:8080";
}

export function createServerAPIClient(session?: string) {
  return createAPIClient(apiOrigin(), {
    cache: "no-store",
    headers: session
      ? { cookie: `crpe_session=${encodeURIComponent(session)}` }
      : undefined,
    fetch: (request) =>
      fetch(request, {
        signal: AbortSignal.any([request.signal, AbortSignal.timeout(15000)]),
        cache: "no-store",
      }),
  });
}

export const memberApi = cache(async () => {
  const session = (await cookies()).get("crpe_session")?.value;
  if (!session) redirect("/connexion");
  const client = createServerAPIClient(session);
  const response = await client.GET("/v1/me");
  if (response.response.status === 401 || response.response.status === 403)
    redirect("/connexion");
  client.use({
    onResponse({ response, schemaPath }) {
      if (response.status === 401) redirect("/connexion");
      if (response.status === 403) {
        const resource = schemaPath.split("/")[2];
        const feature: Record<string, Feature["key"]> = {
          catalog: "course",
          practice: "practice",
          "mock-sessions": "mock",
          "oral-sessions": "oral",
          support: "live",
        };
        if (resource && feature[resource])
          redirect(`/espace/acces/${feature[resource]}`);
      }
    },
  });
  return { user: result(response), client };
});

export const adminApi = cache(async () => {
  const auth = await memberApi();
  if (auth.user.role !== "admin") redirect("/espace");
  return auth;
});

export const adminCatalog = cache(async () => {
  const { client } = await adminApi();
  return result(await client.GET("/v1/admin/content")).items;
});

export const catalog = cache(async () => {
  const { client } = await memberApi();
  return result(await client.GET("/v1/catalog")).items;
});

export const memberAccess = cache(async () => {
  const { client } = await memberApi();
  return result(await client.GET("/v1/me/access"));
});
export async function requireFeature(key: Feature["key"]) {
  const access = await memberAccess();
  const permission = access.features.find((f) => f.key === key);
  if (!permission) throw new Error("Les droits d’accès sont indisponibles.");
  if (!permission.allowed)
    redirect(lockedDestination(permission, access.salesEnabled));
}
export const availableCatalog = cache(async () => {
  const access = await memberAccess();
  return access.features.some((f) => f.key === "course" && f.allowed)
    ? catalog()
    : [];
});
