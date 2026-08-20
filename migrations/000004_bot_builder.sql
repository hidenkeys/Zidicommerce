CREATE TABLE IF NOT EXISTS bots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'inactive', 'archived')),
    default_language TEXT NOT NULL DEFAULT 'en',
    timezone TEXT NOT NULL DEFAULT 'UTC',
    fallback_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    handoff_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    published_version_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, name)
);

CREATE TABLE IF NOT EXISTS bot_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    version_number INTEGER NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'validated', 'published', 'archived')),
    start_step_key TEXT NOT NULL DEFAULT '',
    validation_errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    validated_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (bot_id, version_number)
);

ALTER TABLE bots
    ADD CONSTRAINT fk_bots_published_version
    FOREIGN KEY (published_version_id) REFERENCES bot_versions(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS bot_version_modules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    version_id UUID NOT NULL REFERENCES bot_versions(id) ON DELETE CASCADE,
    module_key TEXT NOT NULL,
    name TEXT NOT NULL,
    category TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT 'system' CHECK (source IN ('system', 'organization')),
    description TEXT NOT NULL DEFAULT '',
    parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS bot_variables (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    version_id UUID NOT NULL REFERENCES bot_versions(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('string', 'number', 'boolean', 'date', 'datetime', 'location', 'object', 'array')),
    scope TEXT NOT NULL DEFAULT 'user' CHECK (scope IN ('system', 'user', 'conversation', 'module')),
    description TEXT NOT NULL DEFAULT '',
    default_value TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (version_id, name)
);

CREATE TABLE IF NOT EXISTS bot_questions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    version_id UUID NOT NULL REFERENCES bot_versions(id) ON DELETE CASCADE,
    question_key TEXT NOT NULL,
    text TEXT NOT NULL,
    type TEXT NOT NULL,
    response_mode TEXT NOT NULL,
    required BOOLEAN NOT NULL DEFAULT TRUE,
    variable_name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    help_text TEXT NOT NULL DEFAULT '',
    options JSONB NOT NULL DEFAULT '[]'::jsonb,
    validation JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (version_id, question_key)
);

CREATE TABLE IF NOT EXISTS bot_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    version_id UUID NOT NULL REFERENCES bot_versions(id) ON DELETE CASCADE,
    action_key TEXT NOT NULL,
    action_type TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    input_mappings JSONB NOT NULL DEFAULT '{}'::jsonb,
    output_mappings JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (version_id, action_key)
);

CREATE TABLE IF NOT EXISTS bot_conditions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    version_id UUID NOT NULL REFERENCES bot_versions(id) ON DELETE CASCADE,
    condition_key TEXT NOT NULL,
    name TEXT NOT NULL,
    combinator TEXT NOT NULL DEFAULT 'and' CHECK (combinator IN ('and', 'or')),
    rules JSONB NOT NULL DEFAULT '[]'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (version_id, condition_key)
);

CREATE TABLE IF NOT EXISTS bot_integrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    version_id UUID NOT NULL REFERENCES bot_versions(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    display_name TEXT NOT NULL,
    required BOOLEAN NOT NULL DEFAULT FALSE,
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (version_id, provider)
);

CREATE TABLE IF NOT EXISTS bot_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    version_id UUID NOT NULL REFERENCES bot_versions(id) ON DELETE CASCADE,
    step_key TEXT NOT NULL,
    type TEXT NOT NULL CHECK (type IN ('message', 'question', 'choice', 'module', 'condition', 'action', 'handoff', 'end')),
    title TEXT NOT NULL,
    message TEXT NOT NULL DEFAULT '',
    question_id UUID REFERENCES bot_questions(id) ON DELETE SET NULL,
    module_id UUID REFERENCES bot_version_modules(id) ON DELETE SET NULL,
    action_id UUID REFERENCES bot_actions(id) ON DELETE SET NULL,
    condition_id UUID REFERENCES bot_conditions(id) ON DELETE SET NULL,
    response_mode TEXT NOT NULL DEFAULT 'free_text',
    options JSONB NOT NULL DEFAULT '[]'::jsonb,
    next_step_key TEXT NOT NULL DEFAULT '',
    fallback_step_key TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (version_id, step_key)
);

CREATE TABLE IF NOT EXISTS bot_published_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    bot_id UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    version_id UUID NOT NULL REFERENCES bot_versions(id) ON DELETE RESTRICT,
    version_number INTEGER NOT NULL,
    snapshot JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (version_id)
);

CREATE INDEX IF NOT EXISTS idx_bots_org_status ON bots(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_bot_versions_bot ON bot_versions(bot_id, version_number DESC);
CREATE INDEX IF NOT EXISTS idx_bot_steps_version_order ON bot_steps(version_id, sort_order);
