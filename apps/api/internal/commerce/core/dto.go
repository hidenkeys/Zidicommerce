package core

import (
	"strings"

	"github.com/google/uuid"
)

type OrganizationInput struct {
	Name            string `json:"name"`
	Slug            string `json:"slug"`
	Description     string `json:"description"`
	LogoURL         string `json:"logo_url"`
	Country         string `json:"country"`
	Currency        string `json:"currency"`
	Timezone        string `json:"timezone"`
	ContactName     string `json:"contact_name"`
	ContactEmail    string `json:"contact_email"`
	ContactPhone    string `json:"contact_phone"`
	OnboardingState string `json:"onboarding_state"`
	Status          string `json:"status"`
	Metadata        string `json:"metadata"`
}

func (i *OrganizationInput) normalize() {
	i.Name = strings.TrimSpace(i.Name)
	i.Slug = strings.ToLower(strings.TrimSpace(i.Slug))
	i.Currency = strings.ToUpper(strings.TrimSpace(i.Currency))
	i.Timezone = strings.TrimSpace(i.Timezone)
	i.Status = strings.ToLower(strings.TrimSpace(i.Status))
	i.ContactEmail = strings.ToLower(strings.TrimSpace(i.ContactEmail))
}

type InviteInput struct {
	Email     string      `json:"email"`
	FirstName string      `json:"first_name"`
	LastName  string      `json:"last_name"`
	Role      string      `json:"role"`
	StoreIDs  []uuid.UUID `json:"store_ids"`
}

func (i *InviteInput) normalize() {
	i.Email = strings.ToLower(strings.TrimSpace(i.Email))
	i.Role = strings.ToLower(strings.TrimSpace(i.Role))
	i.FirstName = strings.TrimSpace(i.FirstName)
	i.LastName = strings.TrimSpace(i.LastName)
}

type AcceptInvitationInput struct {
	Token     string `json:"token"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type MemberUpdateInput struct {
	Role     string      `json:"role"`
	Status   string      `json:"status"`
	StoreIDs []uuid.UUID `json:"store_ids"`
}

type InvitationAcceptance struct {
	UserID         uuid.UUID `json:"user_id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Role           string    `json:"role"`
}

type StoreInput struct {
	Name            string                     `json:"name"`
	Code            string                     `json:"code"`
	Status          string                     `json:"status"`
	Address         string                     `json:"address"`
	City            string                     `json:"city"`
	Country         string                     `json:"country"`
	Latitude        *float64                   `json:"latitude"`
	Longitude       *float64                   `json:"longitude"`
	Metadata        string                     `json:"metadata"`
	Hours           []StoreHourInput           `json:"hours"`
	FulfilmentModes []StoreFulfilmentModeInput `json:"fulfilment_modes"`
}

func (i *StoreInput) normalize() {
	i.Name = strings.TrimSpace(i.Name)
	i.Code = strings.ToUpper(strings.TrimSpace(i.Code))
	i.Status = strings.ToLower(strings.TrimSpace(i.Status))
	i.Address = strings.TrimSpace(i.Address)
	i.City = strings.TrimSpace(i.City)
	i.Country = strings.TrimSpace(i.Country)
}

type StoreHourInput struct {
	DayOfWeek int    `json:"day_of_week"`
	OpensAt   string `json:"opens_at"`
	ClosesAt  string `json:"closes_at"`
	IsClosed  bool   `json:"is_closed"`
}

type StoreFulfilmentModeInput struct {
	Mode             string `json:"mode"`
	Enabled          bool   `json:"enabled"`
	DeliveryFeeMinor int64  `json:"delivery_fee_minor"`
	Metadata         string `json:"metadata"`
}

type CategoryInput struct {
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	SortOrder int    `json:"sort_order"`
	Status    string `json:"status"`
}

func (i *CategoryInput) normalize() {
	i.Name = strings.TrimSpace(i.Name)
	i.Slug = strings.ToLower(strings.TrimSpace(i.Slug))
	i.Status = strings.ToLower(strings.TrimSpace(i.Status))
}

type ProductInput struct {
	CategoryID  *uuid.UUID          `json:"category_id"`
	Name        string              `json:"name"`
	Slug        string              `json:"slug"`
	Description string              `json:"description"`
	Status      string              `json:"status"`
	Metadata    string              `json:"metadata"`
	Variants    []VariantInput      `json:"variants"`
	Images      []ProductImageInput `json:"images"`
}

func (i *ProductInput) normalize() {
	i.Name = strings.TrimSpace(i.Name)
	i.Slug = strings.ToLower(strings.TrimSpace(i.Slug))
	i.Status = strings.ToLower(strings.TrimSpace(i.Status))
}

type VariantInput struct {
	SKU        string `json:"sku"`
	Name       string `json:"name"`
	PriceMinor int64  `json:"price_minor"`
	Currency   string `json:"currency"`
	Status     string `json:"status"`
	Metadata   string `json:"metadata"`
}

func (i *VariantInput) normalize() {
	i.SKU = strings.ToUpper(strings.TrimSpace(i.SKU))
	i.Name = strings.TrimSpace(i.Name)
	i.Currency = strings.ToUpper(strings.TrimSpace(i.Currency))
	i.Status = strings.ToLower(strings.TrimSpace(i.Status))
}

type ProductImageInput struct {
	URL       string `json:"url"`
	AltText   string `json:"alt_text"`
	SortOrder int    `json:"sort_order"`
}

func (i *ProductImageInput) normalize() {
	i.URL = strings.TrimSpace(i.URL)
	i.AltText = strings.TrimSpace(i.AltText)
}

type InventoryCreateInput struct {
	StoreID          uuid.UUID `json:"store_id"`
	VariantID        uuid.UUID `json:"variant_id"`
	OnHand           int       `json:"on_hand"`
	ReorderThreshold int       `json:"reorder_threshold"`
}

type InventoryInput struct {
	SetOnHand        *int `json:"set_on_hand"`
	Adjustment       *int `json:"adjustment"`
	ReorderThreshold *int `json:"reorder_threshold"`
}

type CustomerInput struct {
	Name           string `json:"name"`
	Phone          string `json:"phone"`
	Email          string `json:"email"`
	DefaultAddress string `json:"default_address"`
	Metadata       string `json:"metadata"`
}

func (i *CustomerInput) normalize() {
	i.Name = strings.TrimSpace(i.Name)
	i.Phone = strings.TrimSpace(i.Phone)
	i.Email = strings.ToLower(strings.TrimSpace(i.Email))
	i.DefaultAddress = strings.TrimSpace(i.DefaultAddress)
}

type CartInput struct {
	CustomerID uuid.UUID `json:"customer_id"`
	StoreID    uuid.UUID `json:"store_id"`
	Currency   string    `json:"currency"`
}

type CartItemInput struct {
	VariantID uuid.UUID `json:"variant_id"`
	Quantity  int       `json:"quantity"`
}

type CartSummary struct {
	Cart          Cart              `json:"cart"`
	Items         []CartSummaryItem `json:"items"`
	SubtotalMinor int64             `json:"subtotal_minor"`
	TotalMinor    int64             `json:"total_minor"`
	Currency      string            `json:"currency"`
}

type CartSummaryItem struct {
	Item           CartItem `json:"item"`
	ProductName    string   `json:"product_name"`
	VariantName    string   `json:"variant_name"`
	UnitPriceMinor int64    `json:"unit_price_minor"`
	TotalMinor     int64    `json:"total_minor"`
}

type OrderInput struct {
	CartID             *uuid.UUID       `json:"cart_id"`
	StoreID            uuid.UUID        `json:"store_id"`
	CustomerID         uuid.UUID        `json:"customer_id"`
	FulfilmentType     string           `json:"fulfilment_type"`
	RecipientName      string           `json:"recipient_name"`
	RecipientPhone     string           `json:"recipient_phone"`
	DeliveryAddress    string           `json:"delivery_address"`
	Currency           string           `json:"currency"`
	Metadata           string           `json:"metadata"`
	FulfilmentMetadata string           `json:"fulfilment_metadata"`
	IdempotencyKey     string           `json:"idempotency_key"`
	Items              []OrderItemInput `json:"items"`
}

func (i *OrderInput) normalize() {
	i.FulfilmentType = strings.ToLower(strings.TrimSpace(i.FulfilmentType))
	i.Currency = strings.ToUpper(strings.TrimSpace(i.Currency))
	i.RecipientName = strings.TrimSpace(i.RecipientName)
	i.RecipientPhone = strings.TrimSpace(i.RecipientPhone)
	i.DeliveryAddress = strings.TrimSpace(i.DeliveryAddress)
}

type OrderItemInput struct {
	VariantID uuid.UUID `json:"variant_id"`
	Quantity  int       `json:"quantity"`
}

type OrderFilter struct {
	StoreID *uuid.UUID
	Status  string
}

type TransitionInput struct {
	Status         string `json:"status"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

type PaymentInput struct {
	OrderID        uuid.UUID `json:"order_id"`
	Provider       string    `json:"provider"`
	Email          string    `json:"email"`
	CallbackURL    string    `json:"callback_url"`
	IdempotencyKey string    `json:"idempotency_key"`
}

type PaymentVerifyInput struct {
	Reference string `json:"reference"`
}

type FulfilmentInput struct {
	Status          string `json:"status"`
	RecipientName   string `json:"recipient_name"`
	RecipientPhone  string `json:"recipient_phone"`
	DeliveryAddress string `json:"delivery_address"`
	Metadata        string `json:"metadata"`
}

type ChannelInput struct {
	Provider      string `json:"provider"`
	DisplayName   string `json:"display_name"`
	PhoneNumberID string `json:"phone_number_id"`
	DisplayNumber string `json:"display_number"`
	Status        string `json:"status"`
	Config        string `json:"config"`
	SecretConfig  string `json:"secret_config"`
}

func (i *ChannelInput) normalize() {
	i.Provider = strings.ToLower(strings.TrimSpace(i.Provider))
	i.DisplayName = strings.TrimSpace(i.DisplayName)
	i.PhoneNumberID = strings.TrimSpace(i.PhoneNumberID)
	i.DisplayNumber = strings.TrimSpace(i.DisplayNumber)
	i.Status = strings.ToLower(strings.TrimSpace(i.Status))
}
