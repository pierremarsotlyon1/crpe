CREATE TABLE support_settings (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
 live_url text NOT NULL DEFAULT '', live_schedule text NOT NULL DEFAULT '', group_url text NOT NULL DEFAULT ''
);
INSERT INTO support_settings(singleton) VALUES(true);
CREATE TABLE submission_video_reviews (
 submission_id uuid PRIMARY KEY REFERENCES submissions ON DELETE CASCADE,
 review_video_url text NOT NULL
);
CREATE TABLE mock_video_reviews (
 session_id uuid PRIMARY KEY REFERENCES mock_sessions ON DELETE CASCADE,
 feedback text NOT NULL, review_video_url text NOT NULL
);
