package core

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusActive   = "active"
	StatusInactive = "inactive"

	FulfilmentPickup        = "pickup"
	FulfilmentCustomerRider = "customer_rider"
	FulfilmentMerchantRider = "merchant_rider"

	OrderPending         = "pending"
	OrderAwaitingPayment = "awaiting_payment"
	OrderPaid            = "paid"
	OrderProcessing      = "processing"
	OrderReady           = "ready"
	OrderOutForDelivery  = "out_for_delivery"
	OrderCompleted       = "completed"
	OrderCancelled       = "cancelled"
	OrderRefunded        = "refunded"

	PaymentPending  = "pending"
	PaymentPaid     = "paid"
	PaymentFailed   = "failed"
	PaymentExpired  = "expired"
	PaymentRefunded = "refunded"
)

type Store struct {
	ID              uuid.UUID             `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID  uuid.UUID             `gorm:"type:uuid;index" json:"organization_id"`
	Name            string                `json:"name"`
	Code            string                `json:"code"`
	Status          string                `json:"status"`
	Address         string                `json:"address"`
	City            string                `json:"city"`
	Country         string                `json:"country"`
	Latitude        *float64              `json:"latitude"`
	Longitude       *float64              `json:"longitude"`
	Metadata        string                `json:"metadata"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
	Hours           []StoreHour           `gorm:"foreignKey:StoreID" json:"hours,omitempty"`
	FulfilmentModes []StoreFulfilmentMode `gorm:"foreignKey:StoreID" json:"fulfilment_modes,omitempty"`
}

func (Store) TableName() string { return "stores" }

type StoreHour struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	StoreID        uuid.UUID `gorm:"type:uuid;index" json:"store_id"`
	DayOfWeek      int       `json:"day_of_week"`
	OpensAt        string    `json:"opens_at"`
	ClosesAt       string    `json:"closes_at"`
	IsClosed       bool      `json:"is_closed"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (StoreHour) TableName() string { return "store_hours" }

type StoreFulfilmentMode struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID   uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	StoreID          uuid.UUID `gorm:"type:uuid;index" json:"store_id"`
	Mode             string    `json:"mode"`
	Enabled          bool      `json:"enabled"`
	DeliveryFeeMinor int64     `json:"delivery_fee_minor"`
	Metadata         string    `json:"metadata"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (StoreFulfilmentMode) TableName() string { return "store_fulfilment_modes" }

type StoreUserAssignment struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	StoreID        uuid.UUID `gorm:"type:uuid;index" json:"store_id"`
	UserID         uuid.UUID `gorm:"type:uuid;index" json:"user_id"`
	Role           string    `json:"role"`
	CreatedAt      time.Time `json:"created_at"`
	Store          Store     `gorm:"foreignKey:StoreID" json:"store,omitempty"`
}

func (StoreUserAssignment) TableName() string { return "store_user_assignments" }

type Category struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	Name           string    `json:"name"`
	Slug           string    `json:"slug"`
	SortOrder      int       `json:"sort_order"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Category) TableName() string { return "catalogue_categories" }

type Product struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;index" json:"organization_id"`
	CategoryID     *uuid.UUID     `gorm:"type:uuid" json:"category_id,omitempty"`
	Name           string         `json:"name"`
	Slug           string         `json:"slug"`
	Description    string         `json:"description"`
	Status         string         `json:"status"`
	Metadata       string         `json:"metadata"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	Variants       []Variant      `gorm:"foreignKey:ProductID" json:"variants,omitempty"`
	Images         []ProductImage `gorm:"foreignKey:ProductID" json:"images,omitempty"`
}

func (Product) TableName() string { return "products" }

type Variant struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	ProductID      uuid.UUID `gorm:"type:uuid;index" json:"product_id"`
	SKU            string    `json:"sku"`
	Name           string    `json:"name"`
	PriceMinor     int64     `json:"price_minor"`
	Currency       string    `json:"currency"`
	Status         string    `json:"status"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Product        Product   `gorm:"foreignKey:ProductID" json:"product,omitempty"`
}

func (Variant) TableName() string { return "product_variants" }

type ProductImage struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	ProductID      uuid.UUID `gorm:"type:uuid;index" json:"product_id"`
	URL            string    `json:"url"`
	AltText        string    `json:"alt_text"`
	SortOrder      int       `json:"sort_order"`
	CreatedAt      time.Time `json:"created_at"`
}

func (ProductImage) TableName() string { return "product_images" }

type InventoryLevel struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID   uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	StoreID          uuid.UUID `gorm:"type:uuid;index" json:"store_id"`
	VariantID        uuid.UUID `gorm:"type:uuid;index" json:"variant_id"`
	OnHand           int       `json:"on_hand"`
	Reserved         int       `json:"reserved"`
	ReorderThreshold int       `json:"reorder_threshold"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Variant          Variant   `gorm:"foreignKey:VariantID" json:"variant,omitempty"`
}

func (InventoryLevel) TableName() string { return "inventory_levels" }

func (i InventoryLevel) Available() int { return i.OnHand - i.Reserved }

type Customer struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	Name           string    `json:"name"`
	Phone          string    `json:"phone"`
	Email          string    `json:"email"`
	DefaultAddress string    `json:"default_address"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Customer) TableName() string { return "customers" }

type Cart struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	CustomerID     uuid.UUID  `gorm:"type:uuid;index" json:"customer_id"`
	StoreID        uuid.UUID  `gorm:"type:uuid;index" json:"store_id"`
	Status         string     `json:"status"`
	Currency       string     `json:"currency"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	Items          []CartItem `gorm:"foreignKey:CartID" json:"items,omitempty"`
}

func (Cart) TableName() string { return "carts" }

type CartItem struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	CartID         uuid.UUID `gorm:"type:uuid;index" json:"cart_id"`
	VariantID      uuid.UUID `gorm:"type:uuid;index" json:"variant_id"`
	Quantity       int       `json:"quantity"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Variant        Variant   `gorm:"foreignKey:VariantID" json:"variant,omitempty"`
}

func (CartItem) TableName() string { return "cart_items" }

type Order struct {
	ID               uuid.UUID   `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID   uuid.UUID   `gorm:"type:uuid;index" json:"organization_id"`
	StoreID          uuid.UUID   `gorm:"type:uuid;index" json:"store_id"`
	CustomerID       uuid.UUID   `gorm:"type:uuid;index" json:"customer_id"`
	OrderNumber      string      `json:"order_number"`
	Status           string      `json:"status"`
	FulfilmentType   string      `json:"fulfilment_type"`
	IdempotencyKey   string      `json:"idempotency_key"`
	SubtotalMinor    int64       `json:"subtotal_minor"`
	DeliveryFeeMinor int64       `json:"delivery_fee_minor"`
	TotalMinor       int64       `json:"total_minor"`
	Currency         string      `json:"currency"`
	Metadata         string      `json:"metadata"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	Items            []OrderItem `gorm:"foreignKey:OrderID" json:"items,omitempty"`
	Customer         Customer    `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	Store            Store       `gorm:"foreignKey:StoreID" json:"store,omitempty"`
}

func (Order) TableName() string { return "orders" }

type OrderItem struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	OrderID        uuid.UUID `gorm:"type:uuid;index" json:"order_id"`
	VariantID      uuid.UUID `gorm:"type:uuid;index" json:"variant_id"`
	ProductName    string    `json:"product_name"`
	VariantName    string    `json:"variant_name"`
	Quantity       int       `json:"quantity"`
	UnitPriceMinor int64     `json:"unit_price_minor"`
	TotalMinor     int64     `json:"total_minor"`
	CreatedAt      time.Time `json:"created_at"`
}

func (OrderItem) TableName() string { return "order_items" }

type OrderEvent struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	OrderID        uuid.UUID `gorm:"type:uuid;index" json:"order_id"`
	FromStatus     string    `json:"from_status"`
	ToStatus       string    `json:"to_status"`
	EventType      string    `json:"event_type"`
	ActorUserID    uuid.UUID `gorm:"type:uuid" json:"actor_user_id"`
	Reason         string    `json:"reason"`
	IdempotencyKey string    `json:"idempotency_key"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
}

func (OrderEvent) TableName() string { return "order_events" }

type Payment struct {
	ID               uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID   uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	OrderID          uuid.UUID  `gorm:"type:uuid;index" json:"order_id"`
	Provider         string     `json:"provider"`
	Reference        string     `json:"reference"`
	Status           string     `json:"status"`
	AmountMinor      int64      `json:"amount_minor"`
	Currency         string     `json:"currency"`
	AuthorizationURL string     `json:"authorization_url"`
	IdempotencyKey   string     `json:"idempotency_key"`
	Metadata         string     `json:"metadata"`
	VerifiedAt       *time.Time `json:"verified_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (Payment) TableName() string { return "payments" }

type PaymentWebhookEvent struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID  *uuid.UUID `gorm:"type:uuid;index" json:"organization_id,omitempty"`
	Provider        string     `json:"provider"`
	ExternalEventID string     `json:"external_event_id"`
	Reference       string     `json:"reference"`
	EventType       string     `json:"event_type"`
	Status          string     `json:"status"`
	Payload         string     `json:"payload"`
	ErrorMessage    string     `json:"error_message"`
	ProcessedAt     *time.Time `json:"processed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (PaymentWebhookEvent) TableName() string { return "payment_webhook_events" }

type Fulfilment struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID  uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	OrderID         uuid.UUID `gorm:"type:uuid;index" json:"order_id"`
	Type            string    `json:"type"`
	Status          string    `json:"status"`
	RecipientName   string    `json:"recipient_name"`
	RecipientPhone  string    `json:"recipient_phone"`
	DeliveryAddress string    `json:"delivery_address"`
	Metadata        string    `json:"metadata"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (Fulfilment) TableName() string { return "fulfilments" }

type Channel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	Provider       string    `json:"provider"`
	DisplayName    string    `json:"display_name"`
	PhoneNumberID  string    `json:"phone_number_id"`
	DisplayNumber  string    `json:"display_number"`
	Status         string    `json:"status"`
	Config         string    `json:"config"`
	SecretConfig   string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Channel) TableName() string { return "channels" }

type CommerceNotification struct {
	ID                uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID    uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	OrderID           *uuid.UUID `gorm:"type:uuid;index" json:"order_id,omitempty"`
	CustomerID        *uuid.UUID `gorm:"type:uuid;index" json:"customer_id,omitempty"`
	ChannelID         *uuid.UUID `gorm:"type:uuid" json:"channel_id,omitempty"`
	NotificationType  string     `json:"notification_type"`
	Recipient         string     `json:"recipient"`
	Status            string     `json:"status"`
	Payload           string     `json:"payload"`
	OutboundMessageID *uuid.UUID `gorm:"type:uuid" json:"outbound_message_id,omitempty"`
	ErrorMessage      string     `json:"error_message"`
	Attempts          int        `json:"attempts"`
	NextAttemptAt     *time.Time `json:"next_attempt_at,omitempty"`
	SentAt            *time.Time `json:"sent_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (CommerceNotification) TableName() string { return "commerce_notifications" }

type MerchantImportJob struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;index" json:"organization_id"`
	ActorUserID    *uuid.UUID `gorm:"type:uuid" json:"actor_user_id,omitempty"`
	Status         string     `json:"status"`
	Source         string     `json:"source"`
	Summary        string     `json:"summary"`
	Errors         string     `json:"errors"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (MerchantImportJob) TableName() string { return "merchant_import_jobs" }

type PaymentReconciliation struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index" json:"organization_id"`
	PaymentID      uuid.UUID `gorm:"type:uuid;index" json:"payment_id"`
	Provider       string    `json:"provider"`
	Reference      string    `json:"reference"`
	InternalStatus string    `json:"internal_status"`
	ProviderStatus string    `json:"provider_status"`
	Status         string    `json:"status"`
	ActionTaken    string    `json:"action_taken"`
	Discrepancy    string    `json:"discrepancy"`
	Metadata       string    `json:"metadata"`
	CreatedAt      time.Time `json:"created_at"`
}

func (PaymentReconciliation) TableName() string { return "payment_reconciliations" }

type PaymentConfiguration struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;index;uniqueIndex:idx_payment_config_provider" json:"organization_id"`
	Provider       string     `gorm:"uniqueIndex:idx_payment_config_provider" json:"provider"`
	DisplayName    string     `json:"display_name"`
	Status         string     `json:"status"`
	Enabled        bool       `json:"enabled"`
	PublicConfig   string     `json:"public_config"`
	SecretSource   string     `json:"secret_source"`
	HasSecret      bool       `gorm:"-" json:"has_secret"`
	TestedAt       *time.Time `json:"tested_at,omitempty"`
	Metadata       string     `json:"metadata"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (PaymentConfiguration) TableName() string { return "payment_configurations" }

type PaymentProviderSecret struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID `gorm:"type:uuid;index;uniqueIndex:idx_payment_provider_secret" json:"organization_id"`
	Provider       string    `gorm:"uniqueIndex:idx_payment_provider_secret" json:"provider"`
	SecretName     string    `gorm:"uniqueIndex:idx_payment_provider_secret" json:"secret_name"`
	Ciphertext     string    `json:"-"`
	Nonce          string    `json:"-"`
	KeyVersion     string    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (PaymentProviderSecret) TableName() string { return "payment_provider_secrets" }
