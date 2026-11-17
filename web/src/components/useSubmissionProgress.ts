"use client";
import { useEffect, useState } from "react";
import { browserApi } from "../lib/browser-api";
import { result, type Submission } from "../lib/model";

const inFlight = new Set<Submission["correctionProgress"]["stage"]>([
  "queued",
  "processing",
  "retrying",
]);
export function useSubmissionProgress(
  initial: Submission[],
  teacher: boolean,
  single: boolean,
) {
  const [items, setItems] = useState(initial);
  const [stale, setStale] = useState(false);
  useEffect(() => {
    setItems(initial);
    setStale(false);
  }, [initial]);
  const active = items.some((item) =>
    inFlight.has(item.correctionProgress.stage),
  );
  const id = single ? initial[0]?.id : undefined;
  useEffect(() => {
    if (!active) return;
    let closed = false,
      loading = false;
    let timer: number | undefined;
    const abort = new AbortController();
    function schedule() {
      window.clearTimeout(timer);
      if (!closed && document.visibilityState === "visible")
        timer = window.setTimeout(poll, 3000);
    }
    async function poll() {
      if (closed || loading || document.visibilityState !== "visible") return;
      loading = true;
      try {
        const api = browserApi();
        const updated = id
          ? [
              result(
                await api.GET("/v1/submissions/{id}", {
                  params: { path: { id } },
                  signal: abort.signal,
                }),
              ),
            ]
          : result(
              teacher
                ? await api.GET("/v1/admin/submissions", {
                    signal: abort.signal,
                  })
                : await api.GET("/v1/submissions", { signal: abort.signal }),
            ).items;
        if (!closed) {
          setItems(updated);
          setStale(false);
        }
      } catch {
        if (!closed) setStale(true);
      } finally {
        loading = false;
        schedule();
      }
    }
    function visibility() {
      window.clearTimeout(timer);
      if (document.visibilityState === "visible") void poll();
    }
    document.addEventListener("visibilitychange", visibility);
    window.addEventListener("online", visibility);
    schedule();
    return () => {
      closed = true;
      abort.abort();
      window.clearTimeout(timer);
      document.removeEventListener("visibilitychange", visibility);
      window.removeEventListener("online", visibility);
    };
  }, [active, teacher, id]);
  return { items, stale };
}
