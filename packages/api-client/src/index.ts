import createClient, { type ClientOptions } from "openapi-fetch";
import type { components, paths } from "./schema.ts";

export type { components, paths } from "./schema.ts";

export function createAPIClient(
  baseUrl: string,
  options: Omit<ClientOptions, "baseUrl"> = {},
) {
  return createClient<paths>({ ...options, baseUrl });
}

type APIClient = ReturnType<typeof createAPIClient>;
type PublicConfig = components["schemas"]["PublicConfig"];

// Fail closed if the backend is unavailable or returns an unexpected value.
// This controls presentation only; the backend must enforce purchase permissions.
export async function getPublicConfig(
  client: APIClient,
): Promise<PublicConfig> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 3000);
  try {
    const { data, response } = await client.GET("/v1/config", {
      signal: controller.signal,
      cache: "no-store",
    });
    return { salesEnabled: response.ok && data?.salesEnabled === true };
  } catch {
    return { salesEnabled: false };
  } finally {
    clearTimeout(timeout);
  }
}
