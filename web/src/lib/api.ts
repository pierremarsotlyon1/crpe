import "server-only";
import { createAPIClient } from "@crpe/api-client";

// Only import from server components or route handlers. The origin stays server-side.
export function createServerAPIClient() {
  const baseUrl = process.env.API_INTERNAL_URL;
  if (!baseUrl) {
    throw new Error("API_INTERNAL_URL is required for backend requests");
  }
  return createAPIClient(baseUrl);
}
