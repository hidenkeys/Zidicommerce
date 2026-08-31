UPDATE channel_provider_accounts AS account
SET status = 'connected', updated_at = now()
FROM channels AS connection
WHERE account.organization_id = connection.organization_id
  AND account.channel_connection_id = connection.id
  AND account.provider = 'whatsapp'
  AND account.status = 'connecting'
  AND connection.status IN ('connected', 'healthy');

UPDATE channel_identities AS identity
SET status = 'connected', updated_at = now()
FROM channels AS connection
WHERE identity.organization_id = connection.organization_id
  AND identity.channel_connection_id = connection.id
  AND identity.provider = 'whatsapp'
  AND identity.status = 'connecting'
  AND connection.status IN ('connected', 'healthy');
