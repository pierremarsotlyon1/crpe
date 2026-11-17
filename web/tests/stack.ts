import { execFileSync } from "node:child_process";
export function ownedContainer(project: string, service: string) {
  if (!/^crpe-browser-(\d+|test)$/.test(project))
    throw new Error("Expected an isolated browser test project.");
  const ids = execFileSync(
    "docker",
    [
      "ps",
      "--quiet",
      "--filter",
      `label=com.docker.compose.project=${project}`,
      "--filter",
      `label=com.docker.compose.service=${service}`,
    ],
    { encoding: "utf8" },
  )
    .trim()
    .split("\n");
  if (ids.length !== 1 || !ids[0] || !/^[a-f0-9]+$/.test(ids[0]))
    throw new Error(`Expected one isolated ${service} container.`);
  return ids[0];
}

function fixtureDB(userId: string) {
  if (
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(
      userId,
    )
  )
    throw new Error("Invalid test user ID");
  return ownedContainer(process.env.E2E_COMPOSE_PROJECT || "", "postgres");
}
function applyFixture(userId: string, sql: string) {
  const db = fixtureDB(userId);
  execFileSync("docker", [
    "exec",
    db,
    "psql",
    "-X",
    "-v",
    "ON_ERROR_STOP=1",
    "-U",
    "crpe",
    "-d",
    "crpe",
    "-c",
    sql,
  ]);
}
export function grantTestSupport(userId: string) {
  const sql = `INSERT INTO entitlements(user_id,plan_key,valid_until,source) VALUES('${userId}','suivi-test',now()+interval '1 day','e2e')`;
  applyFixture(userId, sql);
}
export function recordStoreFixture(
  userId: string,
  platform: "google" | "apple",
) {
  // This browser test starts from an entitlement record. Provider verification has its own signed fixtures.
  const sql = `INSERT INTO store_memberships(platform,environment,original_hash,lookup_cipher,user_id,plan_key,product_id,base_plan_id,tier,state,expires_at,observed_at,auto_renew,acknowledged)
 VALUES ('${platform}','Sandbox',decode(replace('${userId}','-',''),'hex'),decode('00','hex'),'${userId}','test-autonomie','test-autonomie','mensuel','autonomie','active',now()+interval '1 day',now(),true,true)`;
  applyFixture(userId, sql);
}
