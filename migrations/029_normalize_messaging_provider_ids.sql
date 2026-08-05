-- Use one canonical provider identifier across sender verification and delivery.
-- Legacy sender domains use "aws_ses", while submitted email attempts and
-- provider feedback use "ses".

UPDATE sender_provider_bindings
SET provider = 'ses',
    updated_at = now()
WHERE lower(trim(provider)) = 'aws_ses';

UPDATE message_delivery_attempts
SET provider = 'ses',
    updated_at = now()
WHERE channel = 'email'
  AND lower(trim(provider)) = 'aws_ses';

-- Some submitted legacy attempts were imported before their aws_ses binding
-- was normalized, so resolve their sender asset and binding after canonicalizing
-- the provider identifier.
UPDATE message_delivery_attempts AS attempt
SET sender_asset_id = asset_link.sender_asset_id,
    sender_provider_binding_id = binding.id,
    updated_at = now()
FROM email_messages AS message
JOIN sender_asset_legacy_links AS asset_link
  ON asset_link.sender_domain_id = message.sender_domain_id
JOIN sender_provider_bindings AS binding
  ON binding.sender_asset_id = asset_link.sender_asset_id
 AND binding.provider = 'ses'
 AND binding.legacy_sender_domain_id = message.sender_domain_id
WHERE attempt.channel = 'email'
  AND attempt.email_message_id = message.id
  AND attempt.sender_provider_binding_id IS NULL;
