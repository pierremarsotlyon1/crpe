import type { components } from "@crpe/api-client";
export type Access = components["schemas"]["MemberAccess"];
export type Feature = components["schemas"]["Capability"];
export const featurePages: Record<Feature["key"], string> = {
  course: "/espace#cours",
  practice: "/espace/pratique/quotidien",
  tutor: "/espace/tuteur",
  mock: "/espace/epreuves",
  oral: "/espace/oral",
  live: "/espace/suivi#live",
  mock_video: "/espace/suivi#videos",
  group: "/espace/suivi#groupe",
};
export function lockedDestination(feature: Feature, salesEnabled: boolean) {
  if (salesEnabled && feature.upgradeUrl) {
    const target = new URL(feature.upgradeUrl, "https://local.invalid");
    if (
      target.pathname === "/offres" &&
      target.searchParams.get("tier") === feature.requiredTier &&
      target.searchParams.get("feature") === feature.key
    )
      return target.pathname + target.search;
  }
  return `/espace/acces/${feature.key}`;
}
