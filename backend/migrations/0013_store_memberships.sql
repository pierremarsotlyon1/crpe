CREATE TABLE store_memberships (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 platform text NOT NULL CHECK(platform IN ('apple','google')),
 environment text NOT NULL CHECK(environment IN ('Sandbox','Production')),
 original_hash bytea NOT NULL, lookup_cipher bytea NOT NULL,
 user_id uuid REFERENCES users ON DELETE SET NULL,
 plan_key text NOT NULL, product_id text NOT NULL, base_plan_id text NOT NULL,
 tier text NOT NULL CHECK(tier IN ('autonomie','suivi')),
 state text NOT NULL CHECK(state IN ('active','pending','expired','revoked')),
 expires_at timestamptz, observed_at timestamptz NOT NULL, auto_renew boolean NOT NULL,
 superseded boolean NOT NULL DEFAULT false, acknowledged boolean NOT NULL DEFAULT false,
 UNIQUE(platform,environment,original_hash)
);
CREATE INDEX store_memberships_owner ON store_memberships(user_id);
CREATE TABLE store_replacements (
 platform text NOT NULL, environment text NOT NULL, replaced_hash bytea NOT NULL, successor uuid NOT NULL REFERENCES store_memberships,
 PRIMARY KEY(platform,environment,replaced_hash)
);
CREATE TABLE store_events (
 platform text NOT NULL, environment text NOT NULL, event_hash bytea NOT NULL, processed_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(platform,environment,event_hash)
);
CREATE TABLE store_orders (
 user_id uuid PRIMARY KEY REFERENCES users ON DELETE CASCADE, platform text NOT NULL CHECK(platform IN ('stripe','apple','google')),
 expires_at timestamptz NOT NULL
);
CREATE VIEW member_rights AS
 SELECT user_id,plan_key,source,valid_until,refunded,''::text AS tier FROM entitlements
 UNION ALL
 SELECT user_id,plan_key,platform,expires_at,state<>'active' OR superseded,tier FROM store_memberships WHERE user_id IS NOT NULL;
