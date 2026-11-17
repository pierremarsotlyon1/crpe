import "server-only";
import { getPublicConfig } from "@crpe/api-client";
import { cache } from "react";
import { createServerAPIClient } from "./api";
import { result } from "./model";

export const publicConfig = cache(() =>
  getPublicConfig(createServerAPIClient()),
);
export const publicSubjects = cache(async () => {
  if (!(await publicConfig()).salesEnabled) return [];
  return result(await createServerAPIClient().GET("/v1/public/subjects")).items;
});
export function siteUrl() {
  const url = process.env.PUBLIC_WEB_URL;
  return url ? new URL(url) : undefined;
}
