ALTER TABLE users
    DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users
    ADD CONSTRAINT users_role_check
    CHECK (role IN ('platform_admin', 'merchant_admin', 'store_manager', 'store_staff', 'support_agent', 'viewer', 'service_provider'));

ALTER TABLE organization_memberships
    DROP CONSTRAINT IF EXISTS organization_memberships_role_check;
ALTER TABLE organization_memberships
    ADD CONSTRAINT organization_memberships_role_check
    CHECK (role IN ('merchant_admin', 'store_manager', 'store_staff', 'support_agent', 'viewer', 'service_provider'));

CREATE TABLE IF NOT EXISTS service_org_settings (
    organization_id UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    booking_fee_minor BIGINT NOT NULL DEFAULT 500000,
    currency TEXT NOT NULL DEFAULT 'NGN',
    require_booking_fee BOOLEAN NOT NULL DEFAULT TRUE,
    booking_fee_refundable BOOLEAN NOT NULL DEFAULT FALSE,
    dispatch_strategy TEXT NOT NULL DEFAULT 'sequential',
    acceptance_window_seconds INT NOT NULL DEFAULT 120,
    max_distance_km DOUBLE PRECISION NOT NULL DEFAULT 25,
    weight_service DOUBLE PRECISION NOT NULL DEFAULT 0.40,
    weight_availability DOUBLE PRECISION NOT NULL DEFAULT 0.20,
    weight_distance DOUBLE PRECISION NOT NULL DEFAULT 0.20,
    weight_rating DOUBLE PRECISION NOT NULL DEFAULT 0.15,
    weight_experience DOUBLE PRECISION NOT NULL DEFAULT 0.05,
    welcome_message TEXT NOT NULL DEFAULT '',
    company_display_name TEXT NOT NULL DEFAULT '',
    provider_portal_base_url TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS service_pools (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    slug TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    sort_order INT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, slug)
);

CREATE TABLE IF NOT EXISTS service_providers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    public_code TEXT NOT NULL,
    name TEXT NOT NULL,
    phone TEXT NOT NULL DEFAULT '',
    whatsapp_number TEXT NOT NULL DEFAULT '',
    rating_average DOUBLE PRECISION NOT NULL DEFAULT 0,
    jobs_completed INT NOT NULL DEFAULT 0,
    area TEXT NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    availability TEXT NOT NULL DEFAULT 'available' CHECK (availability IN ('available', 'busy', 'offline')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    current_job_status TEXT NOT NULL DEFAULT 'idle',
    profile_image_url TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, public_code)
);

CREATE TABLE IF NOT EXISTS service_provider_pools (
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES service_providers(id) ON DELETE CASCADE,
    pool_id UUID NOT NULL REFERENCES service_pools(id) ON DELETE CASCADE,
    PRIMARY KEY (provider_id, pool_id)
);

CREATE TABLE IF NOT EXISTS service_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    public_code TEXT NOT NULL,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    pool_id UUID NOT NULL REFERENCES service_pools(id) ON DELETE RESTRICT,
    session_id UUID,
    channel_id UUID,
    customer_name TEXT NOT NULL DEFAULT '',
    customer_phone TEXT NOT NULL DEFAULT '',
    area TEXT NOT NULL DEFAULT '',
    address TEXT NOT NULL DEFAULT '',
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    description TEXT NOT NULL DEFAULT '',
    preferred_at TEXT NOT NULL DEFAULT '',
    media_url TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft',
    booking_fee_minor BIGINT NOT NULL DEFAULT 0,
    booking_order_id UUID,
    assigned_provider_id UUID REFERENCES service_providers(id) ON DELETE SET NULL,
    conversation_session_id UUID,
    handoff_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, public_code)
);

CREATE INDEX IF NOT EXISTS idx_service_requests_org_status ON service_requests(organization_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_service_requests_customer ON service_requests(organization_id, customer_id);

CREATE TABLE IF NOT EXISTS service_matches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    request_id UUID NOT NULL REFERENCES service_requests(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES service_providers(id) ON DELETE CASCADE,
    rank INT NOT NULL,
    score DOUBLE PRECISION NOT NULL,
    breakdown JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (request_id, provider_id)
);

CREATE TABLE IF NOT EXISTS service_dispatch_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    request_id UUID NOT NULL REFERENCES service_requests(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES service_providers(id) ON DELETE CASCADE,
    status TEXT NOT NULL DEFAULT 'notified' CHECK (status IN ('notified', 'accepted', 'declined', 'timeout', 'cancelled')),
    notified_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    responded_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_service_dispatch_request ON service_dispatch_attempts(request_id, created_at);

CREATE TABLE IF NOT EXISTS service_assignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    request_id UUID NOT NULL REFERENCES service_requests(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES service_providers(id) ON DELETE RESTRICT,
    status TEXT NOT NULL DEFAULT 'accepted',
    notes TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (request_id)
);

CREATE TABLE IF NOT EXISTS service_quotes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    request_id UUID NOT NULL REFERENCES service_requests(id) ON DELETE CASCADE,
    assignment_id UUID NOT NULL REFERENCES service_assignments(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES service_providers(id) ON DELETE RESTRICT,
    public_code TEXT NOT NULL,
    labour_minor BIGINT NOT NULL DEFAULT 0,
    materials_minor BIGINT NOT NULL DEFAULT 0,
    additional_minor BIGINT NOT NULL DEFAULT 0,
    discount_minor BIGINT NOT NULL DEFAULT 0,
    total_minor BIGINT NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'NGN',
    notes TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'sent', 'approved', 'declined', 'expired', 'paid')),
    expires_at TIMESTAMPTZ,
    payment_order_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, public_code)
);

CREATE TABLE IF NOT EXISTS service_quote_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    quote_id UUID NOT NULL REFERENCES service_quotes(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    amount_minor BIGINT NOT NULL DEFAULT 0,
    sort_order INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS service_ratings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    request_id UUID NOT NULL REFERENCES service_requests(id) ON DELETE CASCADE,
    provider_id UUID NOT NULL REFERENCES service_providers(id) ON DELETE CASCADE,
    customer_id UUID NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
    score INT NOT NULL CHECK (score BETWEEN 1 AND 5),
    feedback TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (request_id)
);

CREATE TABLE IF NOT EXISTS service_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    request_id UUID NOT NULL REFERENCES service_requests(id) ON DELETE CASCADE,
    author_type TEXT NOT NULL,
    author_user_id UUID,
    body TEXT NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_service_messages_request ON service_messages(request_id, created_at);
