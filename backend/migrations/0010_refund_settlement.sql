ALTER TABLE billing_refunds ADD COLUMN provider_status text NOT NULL DEFAULT 'succeeded',
 ADD COLUMN finalized_at timestamptz;
UPDATE billing_refunds SET finalized_at=created_at;
CREATE UNIQUE INDEX billing_refund_stripe_id ON billing_refunds(stripe_refund_id);
