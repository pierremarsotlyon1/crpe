CREATE TABLE billing_customers (
  user_id uuid PRIMARY KEY REFERENCES users ON DELETE CASCADE,
  stripe_customer_id text NOT NULL UNIQUE
);
CREATE TABLE entitlements (
  user_id uuid PRIMARY KEY REFERENCES users ON DELETE CASCADE,
  plan_key text NOT NULL, valid_until timestamptz NOT NULL,
  source text NOT NULL, stripe_customer_id text NOT NULL DEFAULT '',
  stripe_subscription_id text NOT NULL DEFAULT '',
  refunded boolean NOT NULL DEFAULT false
);
CREATE INDEX entitlements_subscription ON entitlements(stripe_subscription_id);
CREATE TABLE billing_events (
  id text PRIMARY KEY, kind text NOT NULL, processed_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE billing_payments (
  payment_intent_id text PRIMARY KEY,
  user_id uuid REFERENCES users ON DELETE SET NULL,
  amount bigint NOT NULL, currency text NOT NULL,
  refunded boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE billing_refunds (
  payment_intent_id text PRIMARY KEY REFERENCES billing_payments,
  actor_id uuid REFERENCES users ON DELETE SET NULL,
  stripe_refund_id text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
