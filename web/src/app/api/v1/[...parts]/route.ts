import type { NextRequest } from "next/server";
import { apiOrigin } from "../../../../lib/api";

type Context = { params: Promise<{ parts: string[] }> };
async function proxy(request: NextRequest, { params }: Context) {
  const { parts } = await params;
  if (!parts.every((segment) => /^[A-Za-z0-9_-]+$/.test(segment)))
    return new Response(null, { status: 404 });
  const read = request.method === "GET" || request.method === "HEAD";
  if (!read) {
    const origin = request.headers.get("origin");
    if (
      !origin ||
      !URL.canParse(origin) ||
      !["http:", "https:"].includes(new URL(origin).protocol) ||
      new URL(origin).host !== request.headers.get("host") ||
      request.headers.get("sec-fetch-site") === "cross-site"
    ) {
      return Response.json({ error: "origin_rejected" }, { status: 403 });
    }
  }
  const destination = new URL(`/v1/${parts.join("/")}`, apiOrigin());
  destination.search = request.nextUrl.search;
  const headers = new Headers();
  const session = request.cookies.get("crpe_session")?.value;
  if (session)
    headers.set("cookie", `crpe_session=${encodeURIComponent(session)}`);
  const contentType = request.headers.get("content-type");
  if (contentType) headers.set("content-type", contentType);
  const init: RequestInit & { duplex?: "half" } = {
    method: request.method,
    headers,
    redirect: "manual",
    cache: "no-store",
    signal: AbortSignal.any([
      request.signal,
      AbortSignal.timeout(parts.at(-1) === "transcribe" ? 120000 : 45000),
    ]),
  };
  if (!read && request.body) {
    init.body = request.body;
    init.duplex = "half";
  }
  try {
    const upstream = await fetch(destination, init);
    const output = new Headers({
      "cache-control": "private, no-store",
      "x-content-type-options": "nosniff",
    });
    for (const key of [
      "content-type",
      "content-disposition",
      "content-security-policy",
      "retry-after",
    ]) {
      const value = upstream.headers.get(key);
      if (value) output.set(key, value);
    }
    for (const cookie of upstream.headers.getSetCookie())
      output.append("set-cookie", cookie);
    return new Response(upstream.body, {
      status: upstream.status,
      headers: output,
    });
  } catch {
    return Response.json(
      { error: "service_unavailable" },
      { status: 503, headers: { "cache-control": "no-store" } },
    );
  }
}

export { proxy as GET, proxy as POST, proxy as PUT, proxy as DELETE };
