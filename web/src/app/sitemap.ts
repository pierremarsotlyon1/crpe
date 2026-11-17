import type { MetadataRoute } from "next";
import { publicSubjects, siteUrl } from "../lib/public-site";
export const dynamic = "force-dynamic";
export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const base = siteUrl();
  if (!base) return [];
  const items = await publicSubjects();
  return [
    { url: base.href },
    ...items.map((item) => ({ url: new URL(`/sujets/${item.id}`, base).href })),
  ];
}
