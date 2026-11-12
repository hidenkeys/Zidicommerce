ALTER TABLE journey_bindings DROP CONSTRAINT journey_bindings_mode_check;
ALTER TABLE journey_bindings ADD CONSTRAINT journey_bindings_mode_check
CHECK (mode IN ('guided_chat', 'automated_commerce', 'guided_questions', 'whatsapp_flow', 'ai'));
