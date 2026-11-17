import assert from "node:assert/strict";
import test from "node:test";
import { createAPIClient, getPublicConfig } from "./index.ts";

test("requests the generated contract endpoint without caching the sales flag", async () => {
  const client = createAPIClient("https://api.example.test", {
    fetch: async (request) => {
      assert.equal(request.url, "https://api.example.test/v1/config");
      assert.equal(request.method, "GET");
      assert.equal(request.cache, "no-store");
      return Response.json({ salesEnabled: true });
    },
  });
  assert.deepEqual(await getPublicConfig(client), { salesEnabled: true });
});

test("sales remain disabled on failures or invalid responses", async () => {
  const responses = [
    () => Response.json({ salesEnabled: false }),
    () => Response.json({ salesEnabled: "true" }),
    () => Response.json({}),
    () => Response.json({ salesEnabled: true }, { status: 503 }),
    () => new Response("invalid JSON", { status: 200 }),
    () => {
      throw new Error("Network unavailable");
    },
  ];
  for (const response of responses) {
    const client = createAPIClient("https://api.example.test", {
      fetch: async () => response(),
    });
    assert.deepEqual(await getPublicConfig(client), { salesEnabled: false });
  }
});
