import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";

const path = "packages/api-client/src/schema.ts";
const before = readFileSync(path, "utf8");
execFileSync("npm", ["run", "generate"], { stdio: "inherit" });
if (readFileSync(path, "utf8") !== before) {
  throw new Error(
    "Generated API types were stale. Commit the regenerated contract.",
  );
}
