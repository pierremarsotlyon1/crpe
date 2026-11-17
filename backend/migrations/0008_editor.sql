ALTER TABLE content ADD COLUMN public_correction text NOT NULL DEFAULT '',
 ADD COLUMN video_description text NOT NULL DEFAULT '', ADD COLUMN demo boolean NOT NULL DEFAULT false,
 ADD CONSTRAINT demo_requires_draft CHECK(NOT demo OR status='draft');
CREATE OR REPLACE VIEW published_content AS WITH RECURSIVE visible AS (
 SELECT * FROM content WHERE parent_id IS NULL AND status='validated' AND NOT archived
 UNION ALL SELECT c.* FROM content c JOIN visible v ON c.parent_id=v.id WHERE c.status='validated' AND NOT c.archived
) SELECT * FROM visible;
