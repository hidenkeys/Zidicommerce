# Merchant Deployment Runtime

Phase 9 makes ZidiCommerce ready for a production merchant pilot without adding merchant-specific runtime code. Bing Chun, or any later merchant, is represented as organization-scoped configuration data.

## Onboard A Merchant

1. Create or select the merchant organization in Admin.
2. Add stores, hours, fulfilment modes, catalogue, variants, product images, inventory, channels, payments, and bot versions from the admin screens.
3. For bulk setup, use `POST /v1/merchant-imports/configuration` with an authenticated merchant admin token.

The import is validation-first and repeatable. It upserts using stable tenant-scoped natural keys:

- store `code`
- category `slug`
- product `slug`
- variant `sku`
- inventory `store + variant`
- channel `provider + phone_number_id` when available, otherwise `provider + display_name`

Running the same import twice updates the existing tenant records instead of duplicating them.

Minimal import shape:

```json
{
  "stores": [
    {
      "external_key": "store-1",
      "name": "Merchant Main Store",
      "code": "MAIN",
      "status": "active",
      "address": "Store address",
      "fulfilment_modes": [
        { "mode": "pickup", "enabled": true },
        { "mode": "customer_rider", "enabled": true },
        { "mode": "merchant_rider", "enabled": true, "delivery_fee_minor": 0 }
      ]
    }
  ],
  "categories": [
    { "external_key": "cat-drinks", "name": "Drinks", "slug": "drinks" }
  ],
  "products": [
    {
      "external_key": "product-1",
      "category_external_key": "cat-drinks",
      "name": "Product name",
      "slug": "product-name",
      "status": "active",
      "variants": [
        {
          "external_key": "variant-1",
          "sku": "SKU-001",
          "name": "Regular",
          "price_minor": 0,
          "currency": "NGN",
          "status": "active"
        }
      ],
      "images": [
        { "url": "https://cdn.example.com/product.png", "alt_text": "Product name" }
      ]
    }
  ],
  "inventory": [
    {
      "store_external_key": "store-1",
      "variant_external_key": "variant-1",
      "on_hand": 10,
      "reorder_threshold": 2
    }
  ],
  "channels": [
    {
      "provider": "whatsapp",
      "display_name": "WhatsApp",
      "phone_number_id": "META_PHONE_NUMBER_ID",
      "display_number": "+234...",
      "status": "active",
      "config": "{\"bot_id\":\"published-bot-id\"}",
      "secret_config": "{}"
    }
  ]
}
```

Do not put real secrets in import files or source control. The normal merchant path is Organization -> Channels -> Connect with Meta. Manual encrypted values or server-side secret references are reserved for legacy/operator recovery.

For Bing Chun, keep the real catalogue and pricing in import/configuration data such as `merchant-config/bingchun/*.json`. Do not add Bing Chun conditionals to backend/runtime code.

## WhatsApp

Meta webhook URL:

```text
https://api.example.com/v1/runtime/webhooks/whatsapp
```

The runtime verifies `X-Hub-Signature-256`, resolves the organization from `phone_number_id`, resolves or creates the customer from the WhatsApp sender phone number, runs the published bot snapshot, persists outbound delivery records, then sends replies through the WhatsApp Cloud API adapter.

Required Zidi environment configuration for Embedded Signup:

- `META_APP_ID`
- `META_APP_SECRET`
- `META_EMBEDDED_SIGNUP_CONFIGURATION_ID`
- an explicit `META_GRAPH_API_VERSION`
- `META_WEBHOOK_VERIFY_TOKEN`
- `CHANNEL_SECRET_ENCRYPTION_KEY`
- a public HTTPS `WHATSAPP_WEBHOOK_PUBLIC_BASE_URL`
- a published bot attached to the resulting channel connection

Meta still requires an appropriately configured app, Embedded Signup configuration, business assets, permissions/App Review where applicable, and webhook subscription. The Phase K sender no longer reads `channels.secret_config` directly. Embedded Signup stores the exchanged access token as a write-only encrypted `dbenc://` credential and uses server-side references for the shared app secret and verify token. Existing legacy values can be migrated explicitly from the Channels page and retained for recovery. See [Meta WhatsApp Connection](meta-whatsapp-connection.md) and [Meta/WhatsApp Production Adapter](whatsapp-adapter.md).

## Bot Configuration

Use the Bot Builder. Keep the flow generic:

- welcome/menu
- order module
- track order module
- catalogue module
- complaint module
- FAQ module
- handoff module

Published versions are immutable. Existing sessions stay pinned to the version they started on. New sessions use the latest published version attached to the channel.

Reusable modules now use a call stack. A module step can enter a module and return to its parent step when the nested module ends.

## Commerce Actions

The runtime exposes commerce actions through the existing commerce service, not direct bot database writes. Available action keys include:

- `get_stores`, `get_store`
- `get_categories`, `get_products`, `get_store_catalogue`, `get_product`, `get_variant`
- `get_inventory`, `check_inventory`
- `create_cart`, `get_or_create_cart`, `add_to_cart`, `update_cart_item`, `remove_cart_item`, `calculate_cart`
- `select_fulfilment_mode`, `create_order`, `generate_invoice`
- `initialize_payment`, `check_payment`
- `get_order`, `get_order_status`, `get_customer_orders`
- `create_complaint`, `notify_customer`, `notify_store`, `handoff_to_agent`

Prices and inventory are authoritative in the commerce service.

## Complaints And Handoff

The generic complaint action creates an organization-scoped `support_tickets` row and triggers a support handoff. The runtime also stores `support_handoffs` for conversations that require human intervention.

Admin users can:

- view active conversations from `Automation > Conversations`
- view support tickets from `Automation > Support Handoffs`
- claim a handoff
- add internal notes
- resolve the handoff and optionally resume the bot

Support records are scoped by organization and linked to customer/session/order when available.

## Paystack

Paystack webhook URL:

```text
https://api.example.com/v1/payments/paystack/webhook
```

The API verifies `X-Paystack-Signature` with `PAYSTACK_SECRET_KEY`. `charge.success` webhooks are idempotent and transition the payment and order to paid only once. Duplicate webhooks return the existing processed result.

## Store Operations

Store staff and managers are scoped by `store_user_assignments`. They can list and update orders only for assigned stores. Merchant admins can see all stores in their organization. Platform admins are broader according to existing policy.

Operational status changes should use:

```text
POST /v1/orders/{order_id}/transition
```

with statuses such as `processing`, `ready`, `out_for_delivery`, `completed`, or `cancelled`.

## Production Readiness

Admin `System > Readiness` calls `GET /v1/bot-setup/status` and reports:

- database query readiness
- active stores
- catalogue and variants
- available inventory
- active WhatsApp channel with provider identifiers
- active payment configuration
- published bot
- FAQ coverage
- outbound worker backlog
- support backlog

The screen returns `READY` only when all checklist items pass. It does not print or expose provider secrets.

## Local Verification

Run:

```sh
cd apps/api
go test ./...
go test -race ./...
cd ../..
npm run build --workspace apps/admin
git diff --check
```

Start local API and admin:

```sh
APP_ENV=development SERVER_PORT=8081 DATABASE_URL='postgres://postgres:postgres@localhost:55432/zidicommerce?sslmode=disable' JWT_SECRET='local-development-secret-for-zidicommerce-runtime' MIGRATIONS_DIR=../../migrations EMAIL_MODE=log APP_BASE_URL=http://localhost:3001 go run ./cmd/api
VITE_API_BASE_URL=http://localhost:8081/v1 npm run dev --workspace apps/admin -- --port 3001
```

Open `http://localhost:3001`.
