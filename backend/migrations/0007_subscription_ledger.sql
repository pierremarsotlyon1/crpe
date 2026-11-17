-- A member can have successive subscriptions. Events for an older subscription
-- must never replace the state or extend the lifetime of a newer one.
ALTER TABLE entitlements DROP CONSTRAINT entitlements_pkey;
ALTER TABLE entitlements ADD PRIMARY KEY(user_id,source,stripe_subscription_id);
ALTER TABLE billing_payments ADD COLUMN stripe_subscription_id text NOT NULL DEFAULT '';
CREATE INDEX billing_refunded_subscription ON billing_payments(stripe_subscription_id) WHERE refunded;
