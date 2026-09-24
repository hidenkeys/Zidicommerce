ALTER TABLE channel_identities
    DROP CONSTRAINT IF EXISTS channel_identities_identity_type_check;

ALTER TABLE channel_identities
    ADD CONSTRAINT channel_identities_identity_type_check
        CHECK (identity_type IN (
            'phone_number', 'instagram_business_account', 'web_widget', 'page',
            'tiktok_account'
        ));
