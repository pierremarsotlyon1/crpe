ALTER TABLE store_orders
 ADD COLUMN id uuid NOT NULL DEFAULT gen_random_uuid(),
 ADD COLUMN product_id text NOT NULL DEFAULT '',
 ADD COLUMN base_plan_id text NOT NULL DEFAULT '',
 ADD COLUMN client_id uuid;
CREATE TABLE store_deployment (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 environment text NOT NULL CHECK(environment IN('Sandbox','Production'))
);
