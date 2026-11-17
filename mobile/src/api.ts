import { createAPIClient } from "@crpe/api-client";

export function createMobileAPIClient() {
  const baseUrl = process.env.EXPO_PUBLIC_API_URL;
  if (!baseUrl) {
    throw new Error("EXPO_PUBLIC_API_URL is required for backend requests");
  }
  return createAPIClient(baseUrl);
}
