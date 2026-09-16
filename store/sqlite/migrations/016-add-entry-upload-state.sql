ALTER TABLE entries ADD COLUMN expected_size INTEGER CHECK (
    expected_size IS NULL OR expected_size > 0
);

ALTER TABLE entries
ADD COLUMN upload_state TEXT NOT NULL DEFAULT 'complete' CHECK (
    upload_state IN ('pending', 'complete')
    AND (upload_state = 'complete' OR expected_size IS NOT NULL)
);
