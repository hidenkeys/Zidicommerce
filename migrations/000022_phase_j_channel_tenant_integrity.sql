CREATE UNIQUE INDEX IF NOT EXISTS idx_channels_organization_connection
    ON channels(organization_id, id);

ALTER TABLE channel_provider_accounts
    ADD CONSTRAINT channel_provider_accounts_tenant_connection_fk
    FOREIGN KEY (organization_id, channel_connection_id)
    REFERENCES channels(organization_id, id)
    ON DELETE CASCADE;

ALTER TABLE channel_identities
    ADD CONSTRAINT channel_identities_tenant_connection_fk
    FOREIGN KEY (organization_id, channel_connection_id)
    REFERENCES channels(organization_id, id)
    ON DELETE CASCADE;

ALTER TABLE channel_credential_references
    ADD CONSTRAINT channel_credential_references_tenant_connection_fk
    FOREIGN KEY (organization_id, channel_connection_id)
    REFERENCES channels(organization_id, id)
    ON DELETE CASCADE;

ALTER TABLE channel_health_checks
    ADD CONSTRAINT channel_health_checks_tenant_connection_fk
    FOREIGN KEY (organization_id, channel_connection_id)
    REFERENCES channels(organization_id, id)
    ON DELETE CASCADE;

ALTER TABLE channel_provider_events
    ADD CONSTRAINT channel_provider_events_tenant_connection_fk
    FOREIGN KEY (organization_id, channel_connection_id)
    REFERENCES channels(organization_id, id)
    ON DELETE CASCADE;

ALTER TABLE channel_metrics_daily
    ADD CONSTRAINT channel_metrics_daily_tenant_connection_fk
    FOREIGN KEY (organization_id, channel_connection_id)
    REFERENCES channels(organization_id, id)
    ON DELETE CASCADE;
