ALTER TABLE organization_invitations
    ADD COLUMN IF NOT EXISTS email_status TEXT NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS email_error TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS email_sent_at TIMESTAMPTZ;

ALTER TABLE organization_invitations
    DROP CONSTRAINT IF EXISTS organization_invitations_email_status_check;

ALTER TABLE organization_invitations
    ADD CONSTRAINT organization_invitations_email_status_check
    CHECK (email_status IN ('pending', 'sent', 'failed'));
