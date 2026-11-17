CREATE TABLE jobs (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    deduplication_key text NOT NULL UNIQUE,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts BETWEEN 1 AND 100),
    available_at timestamptz NOT NULL DEFAULT now(),
    lease_until timestamptz,
    claim_token uuid,
    completed_at timestamptz,
    last_error text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'running') = (claim_token IS NOT NULL AND lease_until IS NOT NULL))
);

CREATE INDEX jobs_pending_idx ON jobs (available_at, id) WHERE status = 'pending';
CREATE INDEX jobs_expired_idx ON jobs (lease_until, id) WHERE status = 'running';

-- Operational probe only: this records a durable effect for the scale smoke test.
CREATE TABLE job_probe_results (
    job_id bigint PRIMARY KEY REFERENCES jobs (id),
    completed_at timestamptz NOT NULL DEFAULT now()
);
