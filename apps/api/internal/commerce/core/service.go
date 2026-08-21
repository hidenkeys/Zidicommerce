package core

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/email"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/jobs"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PaymentProvider interface {
	Name() string
	Initialize(ctx context.Context, req PaymentInitializeRequest) (PaymentInitializeResponse, error)
	Verify(ctx context.Context, reference string) (PaymentVerification, error)
}

type PaymentInitializeRequest struct {
	Reference   string
	Email       string
	AmountMinor int64
	Currency    string
	CallbackURL string
}

type PaymentInitializeResponse struct {
	Reference        string
	AuthorizationURL string
	ProviderMetadata string
}

type PaymentVerification struct {
	Reference        string
	Paid             bool
	Status           string
	AmountMinor      int64
	Currency         string
	ProviderMetadata string
}

type Service struct {
	db                      *gorm.DB
	paymentProvider         PaymentProvider
	paymentSecretStore      PaymentProviderSecretStore
	paystackProviderFactory func(secret string) PaymentProvider
	paystackSecret          string
	mailer                  email.Sender
	appBaseURL              string
	log                     *slog.Logger
	jobs                    *jobs.Service
	afterPaymentPaid        AfterPaymentPaid
	now                     func() time.Time
}

type AfterPaymentPaid func(ctx context.Context, organizationID, orderID uuid.UUID, metadata string) error

func NewService(db *gorm.DB, provider PaymentProvider) *Service {
	return &Service{db: db, paymentProvider: provider, paystackProviderFactory: func(secret string) PaymentProvider { return NewPaystackProvider(secret) }, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) ConfigureNotifications(mailer email.Sender, appBaseURL string, logger *slog.Logger) {
	s.mailer = mailer
	s.appBaseURL = strings.TrimRight(appBaseURL, "/")
	s.log = logger
}

func (s *Service) ConfigurePaymentWebhooks(paystackSecret string) {
	s.paystackSecret = strings.TrimSpace(paystackSecret)
}

func (s *Service) ConfigurePaymentSecretStore(store PaymentProviderSecretStore) {
	s.paymentSecretStore = store
}

func (s *Service) ConfigureJobs(jobService *jobs.Service) {
	s.jobs = jobService
}

func (s *Service) ConfigureAfterPaymentPaid(handler AfterPaymentPaid) {
	s.afterPaymentPaid = handler
}

func (s *Service) fireAfterPaymentPaid(ctx context.Context, organizationID, orderID uuid.UUID) {
	if s.afterPaymentPaid == nil || orderID == uuid.Nil {
		return
	}
	var order Order
	if err := s.db.WithContext(ctx).Select("id, metadata").Where("organization_id = ? AND id = ?", organizationID, orderID).First(&order).Error; err != nil {
		return
	}
	if err := s.afterPaymentPaid(ctx, organizationID, orderID, order.Metadata); err != nil && s.log != nil {
		s.log.Error("after payment paid hook failed", "error", err)
	}
}

func (s *Service) CreateOrganization(ctx context.Context, actor auth.CurrentUser, input OrganizationInput) (organization.Organization, error) {
	if actor.Role != authz.PlatformAdmin {
		return organization.Organization{}, httperror.Forbidden("Only platform administrators can create organizations")
	}
	input.normalize()
	if input.Name == "" || input.Slug == "" {
		return organization.Organization{}, httperror.BadRequest("Organization name and slug are required")
	}

	org := organization.Organization{
		ID:              uuid.New(),
		Name:            input.Name,
		Slug:            input.Slug,
		Description:     input.Description,
		LogoURL:         input.LogoURL,
		Country:         input.Country,
		ContactName:     input.ContactName,
		ContactEmail:    input.ContactEmail,
		ContactPhone:    input.ContactPhone,
		OnboardingState: jsonObject(input.OnboardingState),
		Currency:        defaultString(input.Currency, "NGN"),
		Timezone:        defaultString(input.Timezone, "Africa/Lagos"),
		Status:          defaultString(input.Status, "active"),
		Metadata:        jsonObject(input.Metadata),
	}
	if err := s.db.WithContext(ctx).Create(&org).Error; err != nil {
		return organization.Organization{}, err
	}
	return org, nil
}

func (s *Service) ListOrganizations(ctx context.Context, actor auth.CurrentUser) ([]organization.Organization, error) {
	if actor.Role != authz.PlatformAdmin {
		return nil, httperror.Forbidden("Only platform administrators can list organizations")
	}
	var orgs []organization.Organization
	err := s.db.WithContext(ctx).Order("created_at DESC").Find(&orgs).Error
	return orgs, err
}

func (s *Service) GetOrganization(ctx context.Context, actor auth.CurrentUser, id uuid.UUID) (organization.Organization, error) {
	if id == uuid.Nil {
		return organization.Organization{}, httperror.BadRequest("Organization ID is required")
	}
	if actor.Role != authz.PlatformAdmin && actor.OrganizationID != id {
		return organization.Organization{}, httperror.Forbidden("You cannot access this organization")
	}
	var org organization.Organization
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&org).Error
	return org, mapNotFound(err, "Organization not found")
}

func (s *Service) UpdateOrganization(ctx context.Context, actor auth.CurrentUser, id uuid.UUID, input OrganizationInput) (organization.Organization, error) {
	if actor.Role != authz.PlatformAdmin && actor.OrganizationID != id {
		return organization.Organization{}, httperror.Forbidden("You cannot update this organization")
	}
	org, err := s.GetOrganization(ctx, actor, id)
	if err != nil {
		return organization.Organization{}, err
	}
	input.normalize()
	updates := map[string]any{
		"updated_at": s.now(),
	}
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.Slug != "" {
		updates["slug"] = input.Slug
	}
	if input.Description != "" {
		updates["description"] = input.Description
	}
	if input.LogoURL != "" {
		updates["logo_url"] = input.LogoURL
	}
	if input.Country != "" {
		updates["country"] = input.Country
	}
	if input.Currency != "" {
		updates["currency"] = input.Currency
	}
	if input.Timezone != "" {
		updates["timezone"] = input.Timezone
	}
	if input.ContactName != "" {
		updates["contact_name"] = input.ContactName
	}
	if input.ContactEmail != "" {
		updates["contact_email"] = input.ContactEmail
	}
	if input.ContactPhone != "" {
		updates["contact_phone"] = input.ContactPhone
	}
	if input.OnboardingState != "" {
		updates["onboarding_state"] = jsonObject(input.OnboardingState)
	}
	if input.Status != "" {
		updates["status"] = input.Status
	}
	if input.Metadata != "" {
		updates["metadata"] = jsonObject(input.Metadata)
	}
	if err := s.db.WithContext(ctx).Model(&org).Updates(updates).Error; err != nil {
		return organization.Organization{}, err
	}
	_ = s.auditTx(s.db.WithContext(ctx), &id, &actor.ID, "organization", &id, "organization_updated", "{}")
	return s.GetOrganization(ctx, actor, id)
}

func (s *Service) CreateStore(ctx context.Context, actor auth.CurrentUser, input StoreInput) (Store, error) {
	if !actor.Role.CanManageOrganization() {
		return Store{}, httperror.Forbidden("You cannot create stores")
	}
	input.normalize()
	if input.Name == "" || input.Code == "" {
		return Store{}, httperror.BadRequest("Store name and code are required")
	}
	store := Store{
		ID:             uuid.New(),
		OrganizationID: actor.OrganizationID,
		Name:           input.Name,
		Code:           input.Code,
		Status:         defaultString(input.Status, StatusActive),
		Address:        input.Address,
		City:           input.City,
		Country:        input.Country,
		Latitude:       input.Latitude,
		Longitude:      input.Longitude,
		Metadata:       jsonObject(input.Metadata),
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&store).Error; err != nil {
			return err
		}
		return s.replaceStoreConfig(tx, actor.OrganizationID, store.ID, input.Hours, input.FulfilmentModes)
	})
	if err != nil {
		return Store{}, err
	}
	return s.GetStore(ctx, actor, store.ID)
}

func (s *Service) ListStores(ctx context.Context, actor auth.CurrentUser) ([]Store, error) {
	var stores []Store
	query := s.storeQuery(s.db.WithContext(ctx), actor).Preload("Hours").Preload("FulfilmentModes").Order("stores.created_at DESC")
	return stores, query.Find(&stores).Error
}

func (s *Service) GetStore(ctx context.Context, actor auth.CurrentUser, storeID uuid.UUID) (Store, error) {
	var store Store
	err := s.storeQuery(s.db.WithContext(ctx), actor).
		Preload("Hours").
		Preload("FulfilmentModes").
		Where("stores.id = ?", storeID).
		First(&store).Error
	return store, mapNotFound(err, "Store not found")
}

func (s *Service) UpdateStore(ctx context.Context, actor auth.CurrentUser, storeID uuid.UUID, input StoreInput) (Store, error) {
	if !actor.Role.CanManageOrganization() && actor.Role != authz.StoreManager {
		return Store{}, httperror.Forbidden("You cannot update stores")
	}
	store, err := s.GetStore(ctx, actor, storeID)
	if err != nil {
		return Store{}, err
	}
	input.normalize()
	updates := map[string]any{"updated_at": s.now()}
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.Code != "" {
		updates["code"] = input.Code
	}
	if input.Status != "" {
		updates["status"] = input.Status
	}
	updates["address"] = input.Address
	updates["city"] = input.City
	updates["country"] = input.Country
	updates["latitude"] = input.Latitude
	updates["longitude"] = input.Longitude
	if input.Metadata != "" {
		updates["metadata"] = jsonObject(input.Metadata)
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&store).Updates(updates).Error; err != nil {
			return err
		}
		if input.Hours != nil || input.FulfilmentModes != nil {
			return s.replaceStoreConfig(tx, actor.OrganizationID, store.ID, input.Hours, input.FulfilmentModes)
		}
		return nil
	})
	if err != nil {
		return Store{}, err
	}
	return s.GetStore(ctx, actor, storeID)
}

func (s *Service) SetStoreStatus(ctx context.Context, actor auth.CurrentUser, storeID uuid.UUID, status string) (Store, error) {
	return s.UpdateStore(ctx, actor, storeID, StoreInput{Status: status})
}

func (s *Service) CreateCategory(ctx context.Context, actor auth.CurrentUser, input CategoryInput) (Category, error) {
	if !actor.Role.CanManageOrganization() {
		return Category{}, httperror.Forbidden("You cannot manage catalogue")
	}
	input.normalize()
	if input.Name == "" || input.Slug == "" {
		return Category{}, httperror.BadRequest("Category name and slug are required")
	}
	category := Category{ID: uuid.New(), OrganizationID: actor.OrganizationID, Name: input.Name, Slug: input.Slug, SortOrder: input.SortOrder, Status: defaultString(input.Status, StatusActive)}
	return category, s.db.WithContext(ctx).Create(&category).Error
}

func (s *Service) ListCategories(ctx context.Context, actor auth.CurrentUser) ([]Category, error) {
	var categories []Category
	err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("sort_order ASC, name ASC").Find(&categories).Error
	return categories, err
}

func (s *Service) CreateProduct(ctx context.Context, actor auth.CurrentUser, input ProductInput) (Product, error) {
	if !actor.Role.CanManageOrganization() {
		return Product{}, httperror.Forbidden("You cannot manage catalogue")
	}
	input.normalize()
	if input.Name == "" || input.Slug == "" {
		return Product{}, httperror.BadRequest("Product name and slug are required")
	}
	product := Product{
		ID:             uuid.New(),
		OrganizationID: actor.OrganizationID,
		CategoryID:     input.CategoryID,
		Name:           input.Name,
		Slug:           input.Slug,
		Description:    input.Description,
		Status:         defaultString(input.Status, "draft"),
		Metadata:       jsonObject(input.Metadata),
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&product).Error; err != nil {
			return err
		}
		for _, variantInput := range input.Variants {
			variantInput.normalize()
			if variantInput.SKU == "" || variantInput.Name == "" || variantInput.PriceMinor < 0 {
				return httperror.BadRequest("Each variant requires sku, name, and a non-negative price")
			}
			variant := Variant{
				ID:             uuid.New(),
				OrganizationID: actor.OrganizationID,
				ProductID:      product.ID,
				SKU:            variantInput.SKU,
				Name:           variantInput.Name,
				PriceMinor:     variantInput.PriceMinor,
				Currency:       defaultString(variantInput.Currency, "NGN"),
				Status:         defaultString(variantInput.Status, StatusActive),
				Metadata:       jsonObject(variantInput.Metadata),
			}
			if err := tx.Create(&variant).Error; err != nil {
				return err
			}
		}
		for _, imageInput := range input.Images {
			imageInput.normalize()
			if imageInput.URL == "" {
				return httperror.BadRequest("Product image URL is required")
			}
			image := ProductImage{ID: uuid.New(), OrganizationID: actor.OrganizationID, ProductID: product.ID, URL: imageInput.URL, AltText: imageInput.AltText, SortOrder: imageInput.SortOrder}
			if err := tx.Create(&image).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Product{}, err
	}
	return s.GetProduct(ctx, actor, product.ID)
}

func (s *Service) ListProducts(ctx context.Context, actor auth.CurrentUser) ([]Product, error) {
	var products []Product
	err := s.db.WithContext(ctx).
		Where("organization_id = ?", actor.OrganizationID).
		Preload("Variants").
		Preload("Images").
		Order("created_at DESC").
		Find(&products).Error
	return products, err
}

func (s *Service) GetProduct(ctx context.Context, actor auth.CurrentUser, productID uuid.UUID) (Product, error) {
	var product Product
	err := s.db.WithContext(ctx).
		Where("organization_id = ? AND id = ?", actor.OrganizationID, productID).
		Preload("Variants").
		Preload("Images").
		First(&product).Error
	return product, mapNotFound(err, "Product not found")
}

func (s *Service) GetVariant(ctx context.Context, actor auth.CurrentUser, variantID uuid.UUID) (Variant, error) {
	var variant Variant
	err := s.db.WithContext(ctx).
		Where("product_variants.organization_id = ? AND product_variants.id = ?", actor.OrganizationID, variantID).
		Preload("Product").
		First(&variant).Error
	return variant, mapNotFound(err, "Variant not found")
}

func (s *Service) UpdateProduct(ctx context.Context, actor auth.CurrentUser, productID uuid.UUID, input ProductInput) (Product, error) {
	if !actor.Role.CanManageOrganization() {
		return Product{}, httperror.Forbidden("You cannot manage catalogue")
	}
	product, err := s.GetProduct(ctx, actor, productID)
	if err != nil {
		return Product{}, err
	}
	input.normalize()
	updates := map[string]any{"updated_at": s.now()}
	if input.Name != "" {
		updates["name"] = input.Name
	}
	if input.Slug != "" {
		updates["slug"] = input.Slug
	}
	if input.CategoryID != nil {
		updates["category_id"] = input.CategoryID
	}
	if input.Description != "" {
		updates["description"] = input.Description
	}
	if input.Status != "" {
		updates["status"] = input.Status
	}
	if input.Metadata != "" {
		updates["metadata"] = jsonObject(input.Metadata)
	}
	if err := s.db.WithContext(ctx).Model(&product).Updates(updates).Error; err != nil {
		return Product{}, err
	}
	return s.GetProduct(ctx, actor, productID)
}

func (s *Service) CreateVariant(ctx context.Context, actor auth.CurrentUser, productID uuid.UUID, input VariantInput) (Variant, error) {
	if !actor.Role.CanManageOrganization() {
		return Variant{}, httperror.Forbidden("You cannot manage catalogue")
	}
	if _, err := s.GetProduct(ctx, actor, productID); err != nil {
		return Variant{}, err
	}
	input.normalize()
	if input.SKU == "" || input.Name == "" || input.PriceMinor < 0 {
		return Variant{}, httperror.BadRequest("Variant sku, name, and non-negative price are required")
	}
	variant := Variant{ID: uuid.New(), OrganizationID: actor.OrganizationID, ProductID: productID, SKU: input.SKU, Name: input.Name, PriceMinor: input.PriceMinor, Currency: defaultString(input.Currency, "NGN"), Status: defaultString(input.Status, StatusActive), Metadata: jsonObject(input.Metadata)}
	return variant, s.db.WithContext(ctx).Create(&variant).Error
}

func (s *Service) UpdateVariant(ctx context.Context, actor auth.CurrentUser, variantID uuid.UUID, input VariantUpdateInput) (Variant, error) {
	if !actor.Role.CanManageOrganization() {
		return Variant{}, httperror.Forbidden("You cannot manage catalogue")
	}
	var variant Variant
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, variantID).First(&variant).Error; err != nil {
		return Variant{}, mapNotFound(err, "Variant not found")
	}
	updates := map[string]any{"updated_at": s.now()}
	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return Variant{}, httperror.BadRequest("Variant name is required")
		}
		updates["name"] = name
	}
	if input.PriceMinor != nil {
		if *input.PriceMinor < 0 {
			return Variant{}, httperror.BadRequest("Variant price cannot be negative")
		}
		updates["price_minor"] = *input.PriceMinor
	}
	if input.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*input.Status))
		if status != StatusActive && status != StatusInactive {
			return Variant{}, httperror.BadRequest("Variant status is not valid")
		}
		updates["status"] = status
	}
	if err := s.db.WithContext(ctx).Model(&variant).Updates(updates).Error; err != nil {
		return Variant{}, err
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, variantID).First(&variant).Error; err != nil {
		return Variant{}, err
	}
	return variant, nil
}

func (s *Service) CreateProductImage(ctx context.Context, actor auth.CurrentUser, productID uuid.UUID, input ProductImageInput) (ProductImage, error) {
	if !actor.Role.CanManageOrganization() {
		return ProductImage{}, httperror.Forbidden("You cannot manage catalogue")
	}
	if _, err := s.GetProduct(ctx, actor, productID); err != nil {
		return ProductImage{}, err
	}
	input.normalize()
	if input.URL == "" {
		return ProductImage{}, httperror.BadRequest("Image URL is required")
	}
	image := ProductImage{ID: uuid.New(), OrganizationID: actor.OrganizationID, ProductID: productID, URL: input.URL, AltText: input.AltText, SortOrder: input.SortOrder}
	return image, s.db.WithContext(ctx).Create(&image).Error
}

func (s *Service) ListInventory(ctx context.Context, actor auth.CurrentUser, storeID *uuid.UUID) ([]InventoryLevel, error) {
	var rows []InventoryLevel
	query := s.db.WithContext(ctx).Where("inventory_levels.organization_id = ?", actor.OrganizationID).Preload("Variant").Preload("Variant.Product")
	if storeID != nil {
		if _, err := s.GetStore(ctx, actor, *storeID); err != nil {
			return nil, err
		}
		query = query.Where("store_id = ?", *storeID)
	} else if actor.Role == authz.StoreStaff || actor.Role == authz.StoreManager {
		query = query.Joins("JOIN store_user_assignments sua ON sua.organization_id = inventory_levels.organization_id AND sua.store_id = inventory_levels.store_id AND sua.user_id = ?", actor.ID)
	}
	err := query.Order("updated_at DESC").Find(&rows).Error
	return rows, err
}

func (s *Service) CheckInventory(ctx context.Context, actor auth.CurrentUser, storeID, variantID uuid.UUID, quantity int) (InventoryLevel, error) {
	if quantity <= 0 {
		return InventoryLevel{}, httperror.BadRequest("Quantity must be greater than zero")
	}
	if _, err := s.GetStore(ctx, actor, storeID); err != nil {
		return InventoryLevel{}, err
	}
	var level InventoryLevel
	err := s.db.WithContext(ctx).
		Where("organization_id = ? AND store_id = ? AND variant_id = ?", actor.OrganizationID, storeID, variantID).
		Preload("Variant").
		Preload("Variant.Product").
		First(&level).Error
	if err != nil {
		return InventoryLevel{}, mapNotFound(err, "Inventory level not found")
	}
	if level.Available() < quantity {
		return InventoryLevel{}, httperror.BadRequest("Insufficient inventory")
	}
	return level, nil
}

func (s *Service) UpdateInventory(ctx context.Context, actor auth.CurrentUser, inventoryID uuid.UUID, input InventoryInput) (InventoryLevel, error) {
	if !actor.Role.CanManageOrganization() && actor.Role != authz.StoreManager {
		return InventoryLevel{}, httperror.Forbidden("You cannot update inventory")
	}
	var level InventoryLevel
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, inventoryID).First(&level).Error; err != nil {
			return mapNotFound(err, "Inventory level not found")
		}
		if _, err := s.GetStore(ctx, actor, level.StoreID); err != nil {
			return err
		}
		updates := map[string]any{"updated_at": s.now()}
		if input.SetOnHand != nil {
			if *input.SetOnHand < level.Reserved {
				return httperror.BadRequest("On-hand stock cannot be lower than reserved quantity")
			}
			updates["on_hand"] = *input.SetOnHand
		}
		if input.Adjustment != nil {
			next := level.OnHand + *input.Adjustment
			if next < level.Reserved {
				return httperror.BadRequest("Inventory adjustment would make stock unavailable")
			}
			updates["on_hand"] = next
		}
		if input.ReorderThreshold != nil {
			if *input.ReorderThreshold < 0 {
				return httperror.BadRequest("Reorder threshold cannot be negative")
			}
			updates["reorder_threshold"] = *input.ReorderThreshold
		}
		if err := tx.Model(&level).Updates(updates).Error; err != nil {
			return err
		}
		if err := s.auditTx(tx, &actor.OrganizationID, &actor.ID, "inventory", &level.ID, "inventory_updated", fmt.Sprintf(`{"store_id":%q,"variant_id":%q}`, level.StoreID, level.VariantID)); err != nil {
			return err
		}
		return tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, inventoryID).First(&level).Error
	})
	return level, err
}

func (s *Service) UpsertInventory(ctx context.Context, actor auth.CurrentUser, input InventoryCreateInput) (InventoryLevel, error) {
	if !actor.Role.CanManageOrganization() && actor.Role != authz.StoreManager {
		return InventoryLevel{}, httperror.Forbidden("You cannot update inventory")
	}
	if input.StoreID == uuid.Nil || input.VariantID == uuid.Nil || input.OnHand < 0 || input.ReorderThreshold < 0 {
		return InventoryLevel{}, httperror.BadRequest("Store, variant, on-hand stock, and reorder threshold are required")
	}
	if _, err := s.GetStore(ctx, actor, input.StoreID); err != nil {
		return InventoryLevel{}, err
	}
	var variant Variant
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, input.VariantID).First(&variant).Error; err != nil {
		return InventoryLevel{}, mapNotFound(err, "Variant not found")
	}
	var level InventoryLevel
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND store_id = ? AND variant_id = ?", actor.OrganizationID, input.StoreID, input.VariantID).First(&level).Error
		if err == nil {
			if err := tx.Model(&level).Updates(map[string]any{"on_hand": input.OnHand, "reorder_threshold": input.ReorderThreshold, "updated_at": s.now()}).Error; err != nil {
				return err
			}
			return s.auditTx(tx, &actor.OrganizationID, &actor.ID, "inventory", &level.ID, "inventory_upserted", fmt.Sprintf(`{"store_id":%q,"variant_id":%q}`, level.StoreID, level.VariantID))
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		level = InventoryLevel{ID: uuid.New(), OrganizationID: actor.OrganizationID, StoreID: input.StoreID, VariantID: input.VariantID, OnHand: input.OnHand, ReorderThreshold: input.ReorderThreshold}
		if err := tx.Create(&level).Error; err != nil {
			return err
		}
		return s.auditTx(tx, &actor.OrganizationID, &actor.ID, "inventory", &level.ID, "inventory_created", fmt.Sprintf(`{"store_id":%q,"variant_id":%q}`, level.StoreID, level.VariantID))
	})
	return level, err
}

func (s *Service) CreateCustomer(ctx context.Context, actor auth.CurrentUser, input CustomerInput) (Customer, error) {
	input.normalize()
	if input.Name == "" && input.Phone == "" && input.Email == "" {
		return Customer{}, httperror.BadRequest("Customer name, phone, or email is required")
	}
	customer := Customer{ID: uuid.New(), OrganizationID: actor.OrganizationID, Name: input.Name, Phone: input.Phone, Email: input.Email, DefaultAddress: input.DefaultAddress, Metadata: jsonObject(input.Metadata)}
	return customer, s.db.WithContext(ctx).Create(&customer).Error
}

func (s *Service) FindOrCreateCustomer(ctx context.Context, actor auth.CurrentUser, input CustomerInput) (Customer, error) {
	input.normalize()
	if input.Phone == "" && input.Email == "" && input.Name == "" {
		return Customer{}, httperror.BadRequest("Customer name, phone, or email is required")
	}
	var customer Customer
	query := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID)
	if input.Phone != "" {
		err := query.Where("phone = ?", input.Phone).First(&customer).Error
		if err == nil {
			return s.updateCustomerMissingFields(ctx, customer, input)
		}
		if err != gorm.ErrRecordNotFound {
			return Customer{}, err
		}
	}
	if input.Email != "" {
		err := s.db.WithContext(ctx).Where("organization_id = ? AND email = ?", actor.OrganizationID, input.Email).First(&customer).Error
		if err == nil {
			return s.updateCustomerMissingFields(ctx, customer, input)
		}
		if err != gorm.ErrRecordNotFound {
			return Customer{}, err
		}
	}
	return s.CreateCustomer(ctx, actor, input)
}

func (s *Service) updateCustomerMissingFields(ctx context.Context, customer Customer, input CustomerInput) (Customer, error) {
	updates := map[string]any{"updated_at": s.now()}
	if customer.Name == "" && input.Name != "" {
		updates["name"] = input.Name
	}
	if customer.Phone == "" && input.Phone != "" {
		updates["phone"] = input.Phone
	}
	if customer.Email == "" && input.Email != "" {
		updates["email"] = input.Email
	}
	if customer.DefaultAddress == "" && input.DefaultAddress != "" {
		updates["default_address"] = input.DefaultAddress
	}
	if len(updates) > 1 {
		if err := s.db.WithContext(ctx).Model(&customer).Updates(updates).Error; err != nil {
			return Customer{}, err
		}
		if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", customer.OrganizationID, customer.ID).First(&customer).Error; err != nil {
			return Customer{}, err
		}
	}
	return customer, nil
}

func (s *Service) ListCustomers(ctx context.Context, actor auth.CurrentUser) ([]Customer, error) {
	var customers []Customer
	err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("updated_at DESC").Find(&customers).Error
	return customers, err
}

func (s *Service) CreateCart(ctx context.Context, actor auth.CurrentUser, input CartInput) (Cart, error) {
	if input.CustomerID == uuid.Nil || input.StoreID == uuid.Nil {
		return Cart{}, httperror.BadRequest("Customer and store are required")
	}
	if _, err := s.GetStore(ctx, actor, input.StoreID); err != nil {
		return Cart{}, err
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, input.CustomerID).First(&Customer{}).Error; err != nil {
		return Cart{}, mapNotFound(err, "Customer not found")
	}
	cart := Cart{ID: uuid.New(), OrganizationID: actor.OrganizationID, CustomerID: input.CustomerID, StoreID: input.StoreID, Status: "active", Currency: defaultString(input.Currency, "NGN")}
	return cart, s.db.WithContext(ctx).Create(&cart).Error
}

func (s *Service) GetOrCreateActiveCart(ctx context.Context, actor auth.CurrentUser, input CartInput) (Cart, error) {
	if input.CustomerID == uuid.Nil || input.StoreID == uuid.Nil {
		return Cart{}, httperror.BadRequest("Customer and store are required")
	}
	var cart Cart
	err := s.db.WithContext(ctx).Where("organization_id = ? AND customer_id = ? AND store_id = ? AND status = ?", actor.OrganizationID, input.CustomerID, input.StoreID, "active").Order("created_at DESC").First(&cart).Error
	if err == nil {
		return cart, nil
	}
	if err != gorm.ErrRecordNotFound {
		return Cart{}, err
	}
	return s.CreateCart(ctx, actor, input)
}

func (s *Service) GetCart(ctx context.Context, actor auth.CurrentUser, cartID uuid.UUID) (CartSummary, error) {
	cart, err := s.loadCart(ctx, actor, cartID)
	if err != nil {
		return CartSummary{}, err
	}
	return s.calculateCart(cart)
}

func (s *Service) AddCartItem(ctx context.Context, actor auth.CurrentUser, cartID uuid.UUID, input CartItemInput) (CartSummary, error) {
	if input.VariantID == uuid.Nil || input.Quantity <= 0 || input.Quantity > 100 {
		return CartSummary{}, httperror.BadRequest("Variant and quantity from 1 to 100 are required")
	}
	cart, err := s.loadCart(ctx, actor, cartID)
	if err != nil {
		return CartSummary{}, err
	}
	if cart.Status != "active" {
		return CartSummary{}, httperror.BadRequest("Cart is not active")
	}
	if err := s.ensureVariantAvailable(ctx, actor.OrganizationID, cart.StoreID, input.VariantID, input.Quantity); err != nil {
		return CartSummary{}, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item CartItem
		err := tx.Where("organization_id = ? AND cart_id = ? AND variant_id = ?", actor.OrganizationID, cartID, input.VariantID).First(&item).Error
		if err == nil {
			next := item.Quantity + input.Quantity
			if err := s.ensureVariantAvailableTx(tx, actor.OrganizationID, cart.StoreID, input.VariantID, next); err != nil {
				return err
			}
			return tx.Model(&item).Updates(map[string]any{"quantity": next, "updated_at": s.now()}).Error
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		item = CartItem{ID: uuid.New(), OrganizationID: actor.OrganizationID, CartID: cartID, VariantID: input.VariantID, Quantity: input.Quantity}
		return tx.Create(&item).Error
	})
	if err != nil {
		return CartSummary{}, err
	}
	return s.GetCart(ctx, actor, cartID)
}

func (s *Service) UpdateCartItem(ctx context.Context, actor auth.CurrentUser, cartID, itemID uuid.UUID, input CartItemInput) (CartSummary, error) {
	if input.Quantity <= 0 || input.Quantity > 100 {
		return CartSummary{}, httperror.BadRequest("Quantity from 1 to 100 is required")
	}
	cart, err := s.loadCart(ctx, actor, cartID)
	if err != nil {
		return CartSummary{}, err
	}
	var item CartItem
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND cart_id = ? AND id = ?", actor.OrganizationID, cartID, itemID).First(&item).Error; err != nil {
		return CartSummary{}, mapNotFound(err, "Cart item not found")
	}
	if err := s.ensureVariantAvailable(ctx, actor.OrganizationID, cart.StoreID, item.VariantID, input.Quantity); err != nil {
		return CartSummary{}, err
	}
	if err := s.db.WithContext(ctx).Model(&item).Updates(map[string]any{"quantity": input.Quantity, "updated_at": s.now()}).Error; err != nil {
		return CartSummary{}, err
	}
	return s.GetCart(ctx, actor, cartID)
}

func (s *Service) RemoveCartItem(ctx context.Context, actor auth.CurrentUser, cartID, itemID uuid.UUID) (CartSummary, error) {
	if _, err := s.loadCart(ctx, actor, cartID); err != nil {
		return CartSummary{}, err
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND cart_id = ? AND id = ?", actor.OrganizationID, cartID, itemID).Delete(&CartItem{}).Error; err != nil {
		return CartSummary{}, err
	}
	return s.GetCart(ctx, actor, cartID)
}

func (s *Service) ClearCart(ctx context.Context, actor auth.CurrentUser, cartID uuid.UUID) (CartSummary, error) {
	if _, err := s.loadCart(ctx, actor, cartID); err != nil {
		return CartSummary{}, err
	}
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND cart_id = ?", actor.OrganizationID, cartID).Delete(&CartItem{}).Error; err != nil {
		return CartSummary{}, err
	}
	return s.GetCart(ctx, actor, cartID)
}

func (s *Service) CreateOrder(ctx context.Context, actor auth.CurrentUser, input OrderInput) (Order, error) {
	if !actor.Role.CanViewCommerce() {
		return Order{}, httperror.Forbidden("You cannot create orders")
	}
	input.normalize()
	if input.StoreID == uuid.Nil || input.CustomerID == uuid.Nil {
		return Order{}, httperror.BadRequest("Store and customer are required")
	}
	if !isFulfilmentMode(input.FulfilmentType) {
		return Order{}, httperror.BadRequest("Unsupported fulfilment mode")
	}
	var created Order
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if input.IdempotencyKey != "" {
			var existing Order
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND idempotency_key = ?", actor.OrganizationID, input.IdempotencyKey).First(&existing).Error
			if err == nil {
				created = existing
				return nil
			}
			if err != gorm.ErrRecordNotFound {
				return err
			}
		}
		if err := s.ensureStoreAccessibleTx(tx, actor, input.StoreID); err != nil {
			return err
		}
		if err := tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, input.CustomerID).First(&Customer{}).Error; err != nil {
			return mapNotFound(err, "Customer not found")
		}
		items := input.Items
		if input.CartID != nil {
			var cart Cart
			if err := tx.Where("organization_id = ? AND id = ? AND status = ?", actor.OrganizationID, *input.CartID, "active").First(&cart).Error; err != nil {
				return mapNotFound(err, "Active cart not found")
			}
			if cart.CustomerID != input.CustomerID || cart.StoreID != input.StoreID {
				return httperror.BadRequest("Cart customer/store does not match order")
			}
			var cartItems []CartItem
			if err := tx.Where("organization_id = ? AND cart_id = ?", actor.OrganizationID, cart.ID).Find(&cartItems).Error; err != nil {
				return err
			}
			for _, item := range cartItems {
				items = append(items, OrderItemInput{VariantID: item.VariantID, Quantity: item.Quantity})
			}
		}
		if len(items) == 0 {
			return httperror.BadRequest("Order requires at least one item")
		}
		mode, err := s.resolveStoreFulfilmentModeTx(tx, actor.OrganizationID, input.StoreID, input.FulfilmentType)
		if err != nil {
			return err
		}
		order := Order{
			ID:               uuid.New(),
			OrganizationID:   actor.OrganizationID,
			StoreID:          input.StoreID,
			CustomerID:       input.CustomerID,
			OrderNumber:      orderNumber(s.now()),
			Status:           OrderAwaitingPayment,
			FulfilmentType:   input.FulfilmentType,
			IdempotencyKey:   input.IdempotencyKey,
			DeliveryFeeMinor: mode.DeliveryFeeMinor,
			Currency:         defaultString(input.Currency, "NGN"),
			Metadata:         jsonObject(input.Metadata),
		}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}
		for _, item := range items {
			if item.VariantID == uuid.Nil || item.Quantity <= 0 || item.Quantity > 100 {
				return httperror.BadRequest("Each order item requires variant and quantity from 1 to 100")
			}
			var variant Variant
			if err := tx.Preload("Product").Where("product_variants.organization_id = ? AND product_variants.id = ? AND product_variants.status = ?", actor.OrganizationID, item.VariantID, StatusActive).First(&variant).Error; err != nil {
				return mapNotFound(err, "Variant not found")
			}
			if variant.Product.Status != StatusActive {
				return httperror.BadRequest("Product is not active")
			}
			var inv InventoryLevel
			err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("organization_id = ? AND store_id = ? AND variant_id = ?", actor.OrganizationID, input.StoreID, item.VariantID).
				First(&inv).Error
			if err != nil {
				return mapNotFound(err, "Inventory level not found")
			}
			if inv.Available() < item.Quantity {
				return httperror.BadRequest("Insufficient inventory")
			}
			result := tx.Model(&InventoryLevel{}).
				Where("organization_id = ? AND store_id = ? AND variant_id = ? AND on_hand - reserved >= ?", actor.OrganizationID, input.StoreID, item.VariantID, item.Quantity).
				Updates(map[string]any{"on_hand": gorm.Expr("on_hand - ?", item.Quantity), "updated_at": s.now()})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return httperror.BadRequest("Insufficient inventory")
			}
			if err := tx.Where("organization_id = ? AND store_id = ? AND variant_id = ?", actor.OrganizationID, input.StoreID, item.VariantID).First(&inv).Error; err != nil {
				return err
			}
			price := variant.PriceMinor
			if item.UnitPriceMinor != nil && *item.UnitPriceMinor >= 0 {
				price = *item.UnitPriceMinor
			}
			lineTotal, err := multiplyPrice(price, item.Quantity)
			if err != nil {
				return err
			}
			order.SubtotalMinor += lineTotal
			if order.SubtotalMinor < 0 || order.SubtotalMinor > math.MaxInt64-order.DeliveryFeeMinor {
				return httperror.BadRequest("Order total overflow")
			}
			orderItem := OrderItem{ID: uuid.New(), OrganizationID: actor.OrganizationID, OrderID: order.ID, VariantID: variant.ID, ProductName: variant.Product.Name, VariantName: variant.Name, Quantity: item.Quantity, UnitPriceMinor: price, TotalMinor: lineTotal}
			if err := tx.Create(&orderItem).Error; err != nil {
				return err
			}
		}
		order.TotalMinor = order.SubtotalMinor + order.DeliveryFeeMinor
		if err := tx.Model(&order).Updates(map[string]any{"subtotal_minor": order.SubtotalMinor, "total_minor": order.TotalMinor}).Error; err != nil {
			return err
		}
		fulfilment := Fulfilment{ID: uuid.New(), OrganizationID: actor.OrganizationID, OrderID: order.ID, Type: order.FulfilmentType, Status: "pending", RecipientName: input.RecipientName, RecipientPhone: input.RecipientPhone, DeliveryAddress: input.DeliveryAddress, Metadata: jsonObject(input.FulfilmentMetadata)}
		if err := tx.Create(&fulfilment).Error; err != nil {
			return err
		}
		if input.CartID != nil {
			if err := tx.Model(&Cart{}).Where("organization_id = ? AND id = ?", actor.OrganizationID, *input.CartID).Updates(map[string]any{"status": "converted", "updated_at": s.now()}).Error; err != nil {
				return err
			}
		}
		if err := s.recordOrderEventTx(tx, actor, order.ID, "", order.Status, "order_created", input.IdempotencyKey, ""); err != nil {
			return err
		}
		created = order
		return nil
	})
	if err != nil {
		if input.IdempotencyKey != "" {
			var existing Order
			if findErr := s.db.WithContext(ctx).Where("organization_id = ? AND idempotency_key = ?", actor.OrganizationID, input.IdempotencyKey).First(&existing).Error; findErr == nil {
				return s.GetOrder(ctx, actor, existing.ID)
			}
		}
		return Order{}, err
	}
	return s.GetOrder(ctx, actor, created.ID)
}

func (s *Service) ListOrders(ctx context.Context, actor auth.CurrentUser, filter OrderFilter) ([]Order, error) {
	var orders []Order
	query := s.db.WithContext(ctx).Where("orders.organization_id = ?", actor.OrganizationID).Preload("Items").Preload("Customer").Preload("Store").Order("orders.created_at DESC")
	if filter.StoreID != nil {
		if _, err := s.GetStore(ctx, actor, *filter.StoreID); err != nil {
			return nil, err
		}
		query = query.Where("orders.store_id = ?", *filter.StoreID)
	} else if actor.Role == authz.StoreStaff || actor.Role == authz.StoreManager {
		query = query.Joins("JOIN store_user_assignments sua ON sua.organization_id = orders.organization_id AND sua.store_id = orders.store_id AND sua.user_id = ?", actor.ID)
	}
	if filter.Status != "" {
		query = query.Where("orders.status = ?", strings.TrimSpace(filter.Status))
	}
	return orders, query.Find(&orders).Error
}

func (s *Service) ListCustomerOrders(ctx context.Context, actor auth.CurrentUser, customerID uuid.UUID) ([]Order, error) {
	if customerID == uuid.Nil {
		return nil, httperror.BadRequest("Customer is required")
	}
	var orders []Order
	err := s.db.WithContext(ctx).
		Where("orders.organization_id = ? AND orders.customer_id = ?", actor.OrganizationID, customerID).
		Preload("Items").
		Preload("Customer").
		Preload("Store").
		Order("orders.created_at DESC").
		Limit(20).
		Find(&orders).Error
	return orders, err
}

func (s *Service) GetOrder(ctx context.Context, actor auth.CurrentUser, orderID uuid.UUID) (Order, error) {
	var order Order
	query := s.db.WithContext(ctx).Where("orders.organization_id = ? AND orders.id = ?", actor.OrganizationID, orderID).Preload("Items").Preload("Customer").Preload("Store")
	if actor.Role == authz.StoreStaff || actor.Role == authz.StoreManager {
		query = query.Joins("JOIN store_user_assignments sua ON sua.organization_id = orders.organization_id AND sua.store_id = orders.store_id AND sua.user_id = ?", actor.ID)
	}
	err := query.First(&order).Error
	return order, mapNotFound(err, "Order not found")
}

func (s *Service) TransitionOrder(ctx context.Context, actor auth.CurrentUser, orderID uuid.UUID, input TransitionInput) (Order, error) {
	input.Status = strings.TrimSpace(input.Status)
	if input.Status == "" {
		return Order{}, httperror.BadRequest("Target status is required")
	}
	var order Order
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("orders.organization_id = ? AND orders.id = ?", actor.OrganizationID, orderID)
		if actor.Role == authz.StoreStaff || actor.Role == authz.StoreManager {
			query = query.Joins("JOIN store_user_assignments sua ON sua.organization_id = orders.organization_id AND sua.store_id = orders.store_id AND sua.user_id = ?", actor.ID)
		}
		if err := query.First(&order).Error; err != nil {
			return mapNotFound(err, "Order not found")
		}
		if input.IdempotencyKey != "" {
			var existing OrderEvent
			err := tx.Where("organization_id = ? AND order_id = ? AND idempotency_key = ?", actor.OrganizationID, order.ID, input.IdempotencyKey).First(&existing).Error
			if err == nil {
				return nil
			}
			if err != gorm.ErrRecordNotFound {
				return err
			}
		}
		if order.Status == input.Status {
			return nil
		}
		if !canTransition(order.Status, input.Status) {
			return httperror.BadRequest("Invalid order status transition")
		}
		from := order.Status
		if err := tx.Model(&order).Updates(map[string]any{"status": input.Status, "updated_at": s.now()}).Error; err != nil {
			return err
		}
		if input.Status == OrderCancelled {
			if err := s.restoreInventoryForOrderTx(tx, actor.OrganizationID, order.ID); err != nil {
				return err
			}
		}
		if err := s.recordOrderEventTx(tx, actor, order.ID, from, input.Status, "order_transition", input.IdempotencyKey, input.Reason); err != nil {
			return err
		}
		if err := s.auditTx(tx, &actor.OrganizationID, &actor.ID, "order", &order.ID, "order_transition", fmt.Sprintf(`{"from":%q,"to":%q}`, from, input.Status)); err != nil {
			return err
		}
		return s.recordOrderNotificationTx(tx, actor.OrganizationID, order.ID, "order_"+input.Status, "Order "+order.OrderNumber+" is now "+input.Status+".")
	})
	if err != nil {
		return Order{}, err
	}
	return s.GetOrder(ctx, actor, orderID)
}

func (s *Service) InitializePayment(ctx context.Context, actor auth.CurrentUser, input PaymentInput) (Payment, error) {
	if input.OrderID == uuid.Nil || input.IdempotencyKey == "" {
		return Payment{}, httperror.BadRequest("Order and idempotency key are required")
	}
	order, err := s.GetOrder(ctx, actor, input.OrderID)
	if err != nil {
		return Payment{}, err
	}
	providerName := defaultString(strings.ToLower(strings.TrimSpace(input.Provider)), s.paymentProvider.Name())
	provider, err := s.resolveUsablePaymentProvider(ctx, actor.OrganizationID, providerName)
	if err != nil {
		return Payment{}, err
	}
	if provider == nil {
		return Payment{}, httperror.BadRequest("Payment provider is not configured")
	}
	var existing Payment
	err = s.db.WithContext(ctx).Where("organization_id = ? AND order_id = ? AND provider = ? AND idempotency_key = ?", actor.OrganizationID, input.OrderID, providerName, input.IdempotencyKey).First(&existing).Error
	if err == nil {
		return existing, nil
	}
	if err != gorm.ErrRecordNotFound {
		return Payment{}, err
	}
	reference := fmt.Sprintf("zc_%s", uuid.NewString())
	init, err := provider.Initialize(ctx, PaymentInitializeRequest{Reference: reference, Email: input.Email, AmountMinor: order.TotalMinor, Currency: order.Currency, CallbackURL: input.CallbackURL})
	if err != nil {
		return Payment{}, err
	}
	payment := Payment{ID: uuid.New(), OrganizationID: actor.OrganizationID, OrderID: order.ID, Provider: providerName, Reference: init.Reference, Status: PaymentPending, AmountMinor: order.TotalMinor, Currency: order.Currency, AuthorizationURL: init.AuthorizationURL, IdempotencyKey: input.IdempotencyKey, Metadata: jsonObject(init.ProviderMetadata)}
	return payment, s.db.WithContext(ctx).Create(&payment).Error
}

func (s *Service) VerifyPayment(ctx context.Context, actor auth.CurrentUser, input PaymentVerifyInput) (Payment, error) {
	if input.Reference == "" {
		return Payment{}, httperror.BadRequest("Payment reference is required")
	}
	var payment Payment
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND reference = ?", actor.OrganizationID, input.Reference).First(&payment).Error; err != nil {
			return mapNotFound(err, "Payment not found")
		}
		if payment.Status == PaymentPaid {
			return nil
		}
		provider, err := s.resolveUsablePaymentProvider(ctx, actor.OrganizationID, payment.Provider)
		if err != nil {
			return err
		}
		if provider == nil {
			return httperror.BadRequest("Payment provider is not configured")
		}
		verification, err := provider.Verify(ctx, payment.Reference)
		if err != nil {
			return err
		}
		if !verification.Paid {
			return httperror.BadRequest("Payment is not paid")
		}
		if err := verifyPaymentMatches(payment, verification); err != nil {
			return err
		}
		now := s.now()
		if err := tx.Model(&payment).Updates(map[string]any{"status": PaymentPaid, "verified_at": &now, "updated_at": now}).Error; err != nil {
			return err
		}
		var order Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, payment.OrderID).First(&order).Error; err != nil {
			return err
		}
		if order.Status == OrderAwaitingPayment {
			if err := tx.Model(&order).Updates(map[string]any{"status": OrderPaid, "updated_at": now}).Error; err != nil {
				return err
			}
			if err := s.recordOrderEventTx(tx, actor, order.ID, OrderAwaitingPayment, OrderPaid, "payment_verified", "payment:"+payment.Reference, ""); err != nil {
				return err
			}
			if err := s.recordOrderNotificationTx(tx, actor.OrganizationID, order.ID, "payment_confirmed", "Payment confirmed for order "+order.OrderNumber+"."); err != nil {
				return err
			}
		}
		return tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, payment.ID).First(&payment).Error
	})
	if err == nil {
		s.fireAfterPaymentPaid(ctx, actor.OrganizationID, payment.OrderID)
	}
	return payment, err
}

func (s *Service) ReconcilePayment(ctx context.Context, actor auth.CurrentUser, paymentID uuid.UUID) (PaymentReconciliation, error) {
	if paymentID == uuid.Nil {
		return PaymentReconciliation{}, httperror.BadRequest("Payment is required")
	}
	var reconciliation PaymentReconciliation
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var payment Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, paymentID).First(&payment).Error; err != nil {
			return mapNotFound(err, "Payment not found")
		}
		provider, err := s.resolveUsablePaymentProvider(ctx, actor.OrganizationID, payment.Provider)
		if err != nil {
			return err
		}
		if provider == nil {
			return httperror.BadRequest("Payment provider is not configured")
		}
		verification, err := provider.Verify(ctx, payment.Reference)
		if err != nil {
			return err
		}
		status := "matched"
		action := "none"
		discrepancy := ""
		if err := verifyPaymentMatches(payment, verification); err != nil {
			status = "review_required"
			discrepancy = err.Error()
		} else if payment.Status != PaymentPaid && verification.Paid {
			now := s.now()
			if err := tx.Model(&payment).Updates(map[string]any{"status": PaymentPaid, "verified_at": &now, "updated_at": now}).Error; err != nil {
				return err
			}
			var order Order
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", payment.OrganizationID, payment.OrderID).First(&order).Error; err != nil {
				return err
			}
			if order.Status == OrderAwaitingPayment {
				if err := tx.Model(&order).Updates(map[string]any{"status": OrderPaid, "updated_at": now}).Error; err != nil {
					return err
				}
				if err := s.recordOrderEventTx(tx, actor, order.ID, OrderAwaitingPayment, OrderPaid, "payment_reconciled", "payment-reconcile:"+payment.Reference, ""); err != nil {
					return err
				}
				if err := s.recordOrderNotificationTx(tx, actor.OrganizationID, order.ID, "payment_confirmed", "Payment confirmed for order "+order.OrderNumber+"."); err != nil {
					return err
				}
			}
			status = "auto_reconciled"
			action = "marked_paid"
		} else if payment.Status == PaymentPaid && !verification.Paid {
			status = "review_required"
			discrepancy = "internal payment is paid but provider is not paid"
		}
		reconciliation = PaymentReconciliation{ID: uuid.New(), OrganizationID: actor.OrganizationID, PaymentID: payment.ID, Provider: payment.Provider, Reference: payment.Reference, InternalStatus: payment.Status, ProviderStatus: defaultString(verification.Status, boolPaymentStatus(verification.Paid)), Status: status, ActionTaken: action, Discrepancy: discrepancy, Metadata: jsonObject(verification.ProviderMetadata)}
		if err := tx.Create(&reconciliation).Error; err != nil {
			return err
		}
		return s.auditTx(tx, &actor.OrganizationID, &actor.ID, "payment", &payment.ID, "payment_reconciled", fmt.Sprintf(`{"status":%q,"action":%q}`, status, action))
	})
	return reconciliation, err
}

func (s *Service) ListPayments(ctx context.Context, actor auth.CurrentUser) ([]Payment, error) {
	var payments []Payment
	err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("created_at DESC").Find(&payments).Error
	return payments, err
}

func (s *Service) GetPaymentByReference(ctx context.Context, actor auth.CurrentUser, reference string) (Payment, error) {
	var payment Payment
	err := s.db.WithContext(ctx).Where("organization_id = ? AND reference = ?", actor.OrganizationID, strings.TrimSpace(reference)).First(&payment).Error
	return payment, mapNotFound(err, "Payment not found")
}

func (s *Service) resolvePaymentProvider(ctx context.Context, organizationID uuid.UUID, providerName string) (PaymentProvider, error) {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	if providerName == "paystack" && s.paymentSecretStore != nil {
		secret, err := s.paymentSecretStore.GetSecret(ctx, organizationID, "paystack", "secret_key")
		if err == nil && strings.TrimSpace(secret) != "" {
			return s.paystackProviderFactory(secret), nil
		}
		if err != nil && err != gorm.ErrRecordNotFound {
			return nil, err
		}
	}
	if s.paymentProvider != nil && providerName == s.paymentProvider.Name() {
		return s.paymentProvider, nil
	}
	return nil, nil
}

func (s *Service) resolveUsablePaymentProvider(ctx context.Context, organizationID uuid.UUID, providerName string) (PaymentProvider, error) {
	provider, err := s.resolvePaymentProvider(ctx, organizationID, providerName)
	if err != nil || provider == nil {
		return provider, err
	}
	if paystack, ok := provider.(*PaystackProvider); ok && strings.TrimSpace(paystack.secretKey) == "" {
		return nil, nil
	}
	return provider, nil
}

func (s *Service) GetFulfilment(ctx context.Context, actor auth.CurrentUser, orderID uuid.UUID) (Fulfilment, error) {
	if _, err := s.GetOrder(ctx, actor, orderID); err != nil {
		return Fulfilment{}, err
	}
	var fulfilment Fulfilment
	err := s.db.WithContext(ctx).Where("organization_id = ? AND order_id = ?", actor.OrganizationID, orderID).First(&fulfilment).Error
	return fulfilment, mapNotFound(err, "Fulfilment not found")
}

func (s *Service) UpdateFulfilment(ctx context.Context, actor auth.CurrentUser, orderID uuid.UUID, input FulfilmentInput) (Fulfilment, error) {
	fulfilment, err := s.GetFulfilment(ctx, actor, orderID)
	if err != nil {
		return Fulfilment{}, err
	}
	if input.Status != "" && input.Status != fulfilment.Status && !canFulfilmentTransition(fulfilment.Type, fulfilment.Status, input.Status) {
		return Fulfilment{}, httperror.BadRequest("Invalid fulfilment status transition")
	}
	updates := map[string]any{"updated_at": s.now()}
	if input.Status != "" {
		updates["status"] = input.Status
	}
	if input.RecipientName != "" {
		updates["recipient_name"] = input.RecipientName
	}
	if input.RecipientPhone != "" {
		updates["recipient_phone"] = input.RecipientPhone
	}
	if input.DeliveryAddress != "" {
		updates["delivery_address"] = input.DeliveryAddress
	}
	if input.Metadata != "" {
		updates["metadata"] = jsonObject(input.Metadata)
	}
	if err := s.db.WithContext(ctx).Model(&fulfilment).Updates(updates).Error; err != nil {
		return Fulfilment{}, err
	}
	if input.Status != "" && input.Status != fulfilment.Status {
		_ = s.auditTx(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, "fulfilment", &fulfilment.ID, "fulfilment_transition", fmt.Sprintf(`{"from":%q,"to":%q}`, fulfilment.Status, input.Status))
	}
	return s.GetFulfilment(ctx, actor, orderID)
}

func (s *Service) CreateChannel(ctx context.Context, actor auth.CurrentUser, input ChannelInput) (Channel, error) {
	if !actor.Role.CanManageOrganization() {
		return Channel{}, httperror.Forbidden("You cannot manage channels")
	}
	input.normalize()
	if input.Provider == "" || input.DisplayName == "" {
		return Channel{}, httperror.BadRequest("Channel provider and display name are required")
	}
	channel := Channel{ID: uuid.New(), OrganizationID: actor.OrganizationID, Provider: input.Provider, DisplayName: input.DisplayName, PhoneNumberID: input.PhoneNumberID, DisplayNumber: input.DisplayNumber, Status: defaultString(input.Status, "draft"), Config: jsonObject(input.Config), SecretConfig: jsonObject(input.SecretConfig)}
	err := s.db.WithContext(ctx).Create(&channel).Error
	if err == nil {
		_ = s.auditTx(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, "channel", &channel.ID, "channel_created", fmt.Sprintf(`{"provider":%q}`, channel.Provider))
	}
	return channel, err
}

func (s *Service) ListChannels(ctx context.Context, actor auth.CurrentUser) ([]Channel, error) {
	var channels []Channel
	err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("created_at DESC").Find(&channels).Error
	return channels, err
}

func (s *Service) UpdateChannel(ctx context.Context, actor auth.CurrentUser, channelID uuid.UUID, input ChannelInput) (Channel, error) {
	if !actor.Role.CanManageOrganization() {
		return Channel{}, httperror.Forbidden("You cannot manage channels")
	}
	var channel Channel
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, channelID).First(&channel).Error; err != nil {
		return Channel{}, mapNotFound(err, "Channel not found")
	}
	input.normalize()
	updates := map[string]any{"updated_at": s.now()}
	if input.DisplayName != "" {
		updates["display_name"] = input.DisplayName
	}
	if input.PhoneNumberID != "" {
		updates["phone_number_id"] = input.PhoneNumberID
	}
	if input.DisplayNumber != "" {
		updates["display_number"] = input.DisplayNumber
	}
	if input.Status != "" {
		updates["status"] = input.Status
	}
	if input.Config != "" {
		updates["config"] = jsonObject(input.Config)
	}
	if err := s.db.WithContext(ctx).Model(&channel).Updates(updates).Error; err != nil {
		return Channel{}, err
	}
	_ = s.auditTx(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, "channel", &channel.ID, "channel_updated", fmt.Sprintf(`{"provider":%q}`, channel.Provider))
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, channelID).First(&channel).Error; err != nil {
		return Channel{}, err
	}
	return channel, nil
}

func (s *Service) TestChannel(ctx context.Context, actor auth.CurrentUser, channelID uuid.UUID) (map[string]any, error) {
	if !actor.Role.CanManageOrganization() {
		return nil, httperror.Forbidden("You cannot test channels")
	}
	var channel Channel
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, channelID).First(&channel).Error; err != nil {
		return nil, mapNotFound(err, "Channel not found")
	}
	issues := []string{}
	if channel.Provider != "whatsapp" {
		issues = append(issues, "Only WhatsApp channel testing is currently supported.")
	}
	if strings.TrimSpace(channel.PhoneNumberID) == "" {
		issues = append(issues, "Phone number ID is missing.")
	}
	if strings.TrimSpace(channel.DisplayNumber) == "" {
		issues = append(issues, "Display number is missing.")
	}
	status := "ok"
	if len(issues) > 0 {
		status = "needs_attention"
	}
	_ = s.auditTx(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, "channel", &channel.ID, "channel_tested", fmt.Sprintf(`{"status":%q}`, status))
	return map[string]any{"status": status, "issues": issues, "provider": channel.Provider, "display_number": channel.DisplayNumber}, nil
}

func (s *Service) DisconnectChannel(ctx context.Context, actor auth.CurrentUser, channelID uuid.UUID) (Channel, error) {
	return s.UpdateChannel(ctx, actor, channelID, ChannelInput{Status: StatusInactive})
}

func (s *Service) ListPaymentConfigurations(ctx context.Context, actor auth.CurrentUser) ([]PaymentConfiguration, error) {
	if !actor.Role.CanManageOrganization() {
		return nil, httperror.Forbidden("You cannot view payment configuration")
	}
	var configs []PaymentConfiguration
	if err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("provider ASC").Find(&configs).Error; err != nil {
		return nil, err
	}
	return s.annotatePaymentConfigurations(ctx, configs), nil
}

func (s *Service) UpsertPaymentConfiguration(ctx context.Context, actor auth.CurrentUser, input PaymentConfigurationInput) (PaymentConfiguration, error) {
	if !actor.Role.CanManageOrganization() {
		return PaymentConfiguration{}, httperror.Forbidden("You cannot manage payment configuration")
	}
	input.normalize()
	if input.Provider == "" {
		return PaymentConfiguration{}, httperror.BadRequest("Payment provider is required")
	}
	secretConfig := jsonMap(input.SecretConfig)
	if rawSecret := stringFromAny(secretConfig["secret_key"]); rawSecret != "" {
		if s.paymentSecretStore == nil {
			return PaymentConfiguration{}, httperror.BadRequest("Secure payment secret storage is not configured")
		}
		if err := s.paymentSecretStore.SaveSecret(ctx, actor.OrganizationID, input.Provider, "secret_key", rawSecret); err != nil {
			return PaymentConfiguration{}, err
		}
		if input.SecretSource == "" {
			input.SecretSource = "merchant_secret"
		}
	}
	config := PaymentConfiguration{ID: uuid.New(), OrganizationID: actor.OrganizationID, Provider: input.Provider, DisplayName: defaultString(input.DisplayName, input.Provider), Status: defaultString(input.Status, "draft"), Enabled: input.Enabled, PublicConfig: jsonObject(input.PublicConfig), SecretSource: defaultString(input.SecretSource, "environment"), Metadata: jsonObject(input.Metadata)}
	err := s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "organization_id"}, {Name: "provider"}}, DoUpdates: clause.Assignments(map[string]any{"display_name": config.DisplayName, "status": config.Status, "enabled": config.Enabled, "public_config": config.PublicConfig, "secret_source": config.SecretSource, "metadata": config.Metadata, "updated_at": s.now()})}).Create(&config).Error
	if err != nil {
		return PaymentConfiguration{}, err
	}
	var saved PaymentConfiguration
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND provider = ?", actor.OrganizationID, input.Provider).First(&saved).Error; err != nil {
		return PaymentConfiguration{}, err
	}
	s.annotatePaymentConfiguration(ctx, &saved)
	_ = s.auditTx(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, "payment_configuration", &saved.ID, "payment_configuration_saved", fmt.Sprintf(`{"provider":%q,"enabled":%t}`, input.Provider, input.Enabled))
	return saved, nil
}

func (s *Service) TestPaymentConfiguration(ctx context.Context, actor auth.CurrentUser, provider string) (PaymentConfiguration, error) {
	if !actor.Role.CanManageOrganization() {
		return PaymentConfiguration{}, httperror.Forbidden("You cannot test payment configuration")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	var config PaymentConfiguration
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND provider = ?", actor.OrganizationID, provider).First(&config).Error; err != nil {
		return PaymentConfiguration{}, mapNotFound(err, "Payment configuration not found")
	}
	s.annotatePaymentConfiguration(ctx, &config)
	status := "needs_attention"
	resolved, err := s.resolveUsablePaymentProvider(ctx, actor.OrganizationID, provider)
	if err != nil {
		return PaymentConfiguration{}, err
	}
	if resolved != nil {
		status = StatusActive
	}
	now := s.now()
	if err := s.db.WithContext(ctx).Model(&config).Updates(map[string]any{"status": status, "tested_at": &now, "updated_at": now}).Error; err != nil {
		return PaymentConfiguration{}, err
	}
	_ = s.auditTx(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, "payment_configuration", &config.ID, "payment_configuration_tested", fmt.Sprintf(`{"provider":%q,"status":%q}`, provider, status))
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND provider = ?", actor.OrganizationID, provider).First(&config).Error; err != nil {
		return PaymentConfiguration{}, err
	}
	s.annotatePaymentConfiguration(ctx, &config)
	return config, nil
}

func (s *Service) annotatePaymentConfigurations(ctx context.Context, configs []PaymentConfiguration) []PaymentConfiguration {
	for index := range configs {
		s.annotatePaymentConfiguration(ctx, &configs[index])
	}
	return configs
}

func (s *Service) annotatePaymentConfiguration(ctx context.Context, config *PaymentConfiguration) {
	if config == nil || s.paymentSecretStore == nil {
		return
	}
	hasSecret, err := s.paymentSecretStore.HasSecret(ctx, config.OrganizationID, config.Provider, "secret_key")
	if err == nil {
		config.HasSecret = hasSecret
	}
}

func (s *Service) RecordNotification(ctx context.Context, actor auth.CurrentUser, input NotificationInput) (CommerceNotification, error) {
	if strings.TrimSpace(input.Type) == "" {
		return CommerceNotification{}, httperror.BadRequest("Notification type is required")
	}
	notification := CommerceNotification{
		ID:               uuid.New(),
		OrganizationID:   actor.OrganizationID,
		OrderID:          input.OrderID,
		CustomerID:       input.CustomerID,
		ChannelID:        input.ChannelID,
		NotificationType: strings.TrimSpace(input.Type),
		Recipient:        strings.TrimSpace(input.Recipient),
		Status:           defaultString(strings.TrimSpace(input.Status), "queued"),
		Payload:          jsonObject(input.Payload),
	}
	if err := s.db.WithContext(ctx).Create(&notification).Error; err != nil {
		return CommerceNotification{}, err
	}
	if err := s.enqueueNotificationJob(ctx, notification); err != nil {
		return CommerceNotification{}, err
	}
	return notification, nil
}

func (s *Service) storeQuery(db *gorm.DB, actor auth.CurrentUser) *gorm.DB {
	query := db.Model(&Store{}).Where("stores.organization_id = ?", actor.OrganizationID)
	if actor.Role == authz.StoreStaff || actor.Role == authz.StoreManager {
		query = query.Joins("JOIN store_user_assignments sua ON sua.organization_id = stores.organization_id AND sua.store_id = stores.id AND sua.user_id = ?", actor.ID)
	}
	return query
}

func (s *Service) replaceStoreConfig(tx *gorm.DB, organizationID, storeID uuid.UUID, hours []StoreHourInput, modes []StoreFulfilmentModeInput) error {
	if hours != nil {
		if err := tx.Where("organization_id = ? AND store_id = ?", organizationID, storeID).Delete(&StoreHour{}).Error; err != nil {
			return err
		}
		for _, input := range hours {
			hour := StoreHour{ID: uuid.New(), OrganizationID: organizationID, StoreID: storeID, DayOfWeek: input.DayOfWeek, OpensAt: input.OpensAt, ClosesAt: input.ClosesAt, IsClosed: input.IsClosed}
			if err := tx.Create(&hour).Error; err != nil {
				return err
			}
		}
	}
	if modes != nil {
		if err := tx.Where("organization_id = ? AND store_id = ?", organizationID, storeID).Delete(&StoreFulfilmentMode{}).Error; err != nil {
			return err
		}
		for _, input := range modes {
			if !isFulfilmentMode(input.Mode) {
				return httperror.BadRequest("Unsupported fulfilment mode")
			}
			mode := StoreFulfilmentMode{ID: uuid.New(), OrganizationID: organizationID, StoreID: storeID, Mode: input.Mode, Enabled: input.Enabled, DeliveryFeeMinor: input.DeliveryFeeMinor, Metadata: jsonObject(input.Metadata)}
			if err := tx.Create(&mode).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) loadCart(ctx context.Context, actor auth.CurrentUser, cartID uuid.UUID) (Cart, error) {
	var cart Cart
	err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", actor.OrganizationID, cartID).Preload("Items").Preload("Items.Variant").Preload("Items.Variant.Product").First(&cart).Error
	if err != nil {
		return Cart{}, mapNotFound(err, "Cart not found")
	}
	if _, err := s.GetStore(ctx, actor, cart.StoreID); err != nil {
		return Cart{}, err
	}
	return cart, nil
}

func (s *Service) calculateCart(cart Cart) (CartSummary, error) {
	summary := CartSummary{Cart: cart, Items: make([]CartSummaryItem, 0, len(cart.Items)), Currency: cart.Currency}
	for _, item := range cart.Items {
		lineTotal, err := multiplyPrice(item.Variant.PriceMinor, item.Quantity)
		if err != nil {
			return CartSummary{}, err
		}
		summary.SubtotalMinor += lineTotal
		summary.Items = append(summary.Items, CartSummaryItem{Item: item, ProductName: item.Variant.Product.Name, VariantName: item.Variant.Name, UnitPriceMinor: item.Variant.PriceMinor, TotalMinor: lineTotal})
	}
	summary.TotalMinor = summary.SubtotalMinor
	return summary, nil
}

func (s *Service) ensureVariantAvailable(ctx context.Context, organizationID, storeID, variantID uuid.UUID, quantity int) error {
	return s.ensureVariantAvailableTx(s.db.WithContext(ctx), organizationID, storeID, variantID, quantity)
}

func (s *Service) ensureVariantAvailableTx(tx *gorm.DB, organizationID, storeID, variantID uuid.UUID, quantity int) error {
	var inv InventoryLevel
	if err := tx.Where("organization_id = ? AND store_id = ? AND variant_id = ?", organizationID, storeID, variantID).First(&inv).Error; err != nil {
		return mapNotFound(err, "Inventory level not found")
	}
	if inv.Available() < quantity {
		return httperror.BadRequest("Insufficient inventory")
	}
	return nil
}

func (s *Service) ensureStoreAccessibleTx(tx *gorm.DB, actor auth.CurrentUser, storeID uuid.UUID) error {
	var count int64
	query := tx.Table("stores").Where("stores.organization_id = ? AND stores.id = ? AND stores.status = ?", actor.OrganizationID, storeID, StatusActive)
	if actor.Role == authz.StoreStaff || actor.Role == authz.StoreManager {
		query = query.Joins("JOIN store_user_assignments sua ON sua.organization_id = stores.organization_id AND sua.store_id = stores.id AND sua.user_id = ?", actor.ID)
	}
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return httperror.NotFound("Store not found")
	}
	return nil
}

func (s *Service) resolveStoreFulfilmentModeTx(tx *gorm.DB, organizationID, storeID uuid.UUID, mode string) (StoreFulfilmentMode, error) {
	var fulfilmentMode StoreFulfilmentMode
	err := tx.Where("organization_id = ? AND store_id = ? AND mode = ? AND enabled = ?", organizationID, storeID, mode, true).First(&fulfilmentMode).Error
	return fulfilmentMode, mapNotFound(err, "Fulfilment mode is not enabled for this store")
}

func (s *Service) recordOrderEventTx(tx *gorm.DB, actor auth.CurrentUser, orderID uuid.UUID, from, to, eventType, idempotencyKey, reason string) error {
	if idempotencyKey == "" {
		idempotencyKey = fmt.Sprintf("%s:%s:%d", eventType, to, time.Now().UnixNano())
	}
	var actorUserID *uuid.UUID
	if actor.ID != uuid.Nil {
		actorUserID = &actor.ID
	}
	event := OrderEvent{ID: uuid.New(), OrganizationID: actor.OrganizationID, OrderID: orderID, FromStatus: from, ToStatus: to, EventType: eventType, ActorUserID: actorUserID, IdempotencyKey: idempotencyKey, Reason: reason, Metadata: "{}"}
	return tx.Create(&event).Error
}

func (s *Service) recordOrderNotificationTx(tx *gorm.DB, organizationID, orderID uuid.UUID, notificationType, message string) error {
	var order Order
	if err := tx.Where("organization_id = ? AND id = ?", organizationID, orderID).Preload("Customer").First(&order).Error; err != nil {
		return err
	}
	var channel Channel
	var channelID *uuid.UUID
	if err := tx.Where("organization_id = ? AND provider = ? AND status = ?", organizationID, "whatsapp", StatusActive).Order("created_at ASC").First(&channel).Error; err == nil {
		channelID = &channel.ID
	}
	customerID := order.CustomerID
	notification := CommerceNotification{ID: uuid.New(), OrganizationID: organizationID, OrderID: &order.ID, CustomerID: &customerID, ChannelID: channelID, NotificationType: notificationType, Recipient: order.Customer.Phone, Status: "queued", Payload: jsonValue(map[string]any{"message": message, "order_id": order.ID.String(), "order_number": order.OrderNumber, "status": order.Status})}
	if err := tx.Create(&notification).Error; err != nil {
		return err
	}
	return s.enqueueNotificationJobTx(tx, notification)
}

func (s *Service) restoreInventoryForOrderTx(tx *gorm.DB, organizationID, orderID uuid.UUID) error {
	var order Order
	if err := tx.Where("organization_id = ? AND id = ?", organizationID, orderID).First(&order).Error; err != nil {
		return err
	}
	var items []OrderItem
	if err := tx.Where("organization_id = ? AND order_id = ?", organizationID, orderID).Find(&items).Error; err != nil {
		return err
	}
	for _, item := range items {
		var level InventoryLevel
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("organization_id = ? AND store_id = ? AND variant_id = ?", organizationID, order.StoreID, item.VariantID).
			First(&level).Error; err != nil {
			return err
		}
		if err := tx.Model(&level).Updates(map[string]any{"on_hand": gorm.Expr("on_hand + ?", item.Quantity), "updated_at": s.now()}).Error; err != nil {
			return err
		}
	}
	return nil
}

func mapNotFound(err error, message string) error {
	if err == nil {
		return nil
	}
	if err == gorm.ErrRecordNotFound {
		return httperror.NotFound(message)
	}
	return err
}

func canTransition(from, to string) bool {
	allowed := map[string][]string{
		OrderAwaitingPayment: {OrderPaid, OrderCancelled},
		OrderPaid:            {OrderProcessing, OrderCancelled},
		OrderProcessing:      {OrderReady, OrderCancelled},
		OrderReady:           {OrderOutForDelivery, OrderCompleted, OrderCancelled},
		OrderOutForDelivery:  {OrderCompleted, OrderCancelled},
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

func canFulfilmentTransition(fulfilmentType, from, to string) bool {
	allowed := map[string][]string{
		"pending":          {"ready", "cancelled"},
		"ready":            {"completed", "cancelled"},
		"out_for_delivery": {"completed", "cancelled"},
	}
	if fulfilmentType == FulfilmentMerchantRider {
		allowed["ready"] = []string{"out_for_delivery", "completed", "cancelled"}
	}
	for _, candidate := range allowed[from] {
		if candidate == to {
			return true
		}
	}
	return false
}

func isFulfilmentMode(mode string) bool {
	return mode == FulfilmentPickup || mode == FulfilmentCustomerRider || mode == FulfilmentMerchantRider
}

func verifyPaymentMatches(payment Payment, verification PaymentVerification) error {
	if verification.Reference != "" && verification.Reference != payment.Reference {
		return httperror.BadRequest("Payment reference mismatch")
	}
	if verification.AmountMinor > 0 && verification.AmountMinor != payment.AmountMinor {
		return httperror.BadRequest("Payment amount mismatch")
	}
	if verification.Currency != "" && strings.ToUpper(verification.Currency) != strings.ToUpper(payment.Currency) {
		return httperror.BadRequest("Payment currency mismatch")
	}
	return nil
}

func boolPaymentStatus(paid bool) string {
	if paid {
		return "success"
	}
	return "not_paid"
}

func (s *Service) enqueueNotificationJob(ctx context.Context, notification CommerceNotification) error {
	if s.jobs == nil {
		return nil
	}
	_, err := s.jobs.Enqueue(ctx, jobs.EnqueueInput{
		OrganizationID: &notification.OrganizationID,
		JobType:        jobs.JobTypeNotificationDelivery,
		Payload:        map[string]any{"notification_id": notification.ID.String()},
		IdempotencyKey: "notification:" + notification.ID.String(),
		CorrelationID:  notification.ID.String(),
		MaxAttempts:    5,
	})
	return err
}

func (s *Service) enqueueNotificationJobTx(tx *gorm.DB, notification CommerceNotification) error {
	if s.jobs == nil {
		return nil
	}
	now := s.now()
	organizationID := notification.OrganizationID
	job := jobs.Job{ID: uuid.New(), OrganizationID: &organizationID, JobType: jobs.JobTypeNotificationDelivery, Status: jobs.StatusQueued, Payload: jsonValue(map[string]any{"notification_id": notification.ID.String()}), IdempotencyKey: "notification:" + notification.ID.String(), CorrelationID: notification.ID.String(), MaxAttempts: 5, AvailableAt: now}
	return tx.Create(&job).Error
}

func multiplyPrice(price int64, quantity int) (int64, error) {
	if price < 0 || quantity <= 0 {
		return 0, httperror.BadRequest("Invalid price or quantity")
	}
	if price > math.MaxInt64/int64(quantity) {
		return 0, httperror.BadRequest("Line total overflow")
	}
	return price * int64(quantity), nil
}

func orderNumber(now time.Time) string {
	return fmt.Sprintf("ZC-%s-%s", now.UTC().Format("20060102"), strings.ToUpper(uuid.NewString()[:8]))
}

func jsonObject(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	return raw
}

func jsonValue(value any) string {
	if value == nil {
		return "{}"
	}
	body, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(body)
}

func jsonMap(raw string) map[string]any {
	result := map[string]any{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return map[string]any{}
	}
	return result
}

func stringFromAny(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
