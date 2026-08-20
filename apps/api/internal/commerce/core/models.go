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
