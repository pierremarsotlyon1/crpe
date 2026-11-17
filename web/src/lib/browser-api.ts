"use client";
import { createAPIClient } from "@crpe/api-client";
export function browserApi() {
  return createAPIClient(`${window.location.origin}/api`, {
    credentials: "same-origin",
    cache: "no-store",
  });
}
