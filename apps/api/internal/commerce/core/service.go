package core

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/email"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
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
	Reference string
	Paid      bool
}

type Service struct {
	db              *gorm.DB
	paymentProvider PaymentProvider
	mailer          email.Sender
	appBaseURL      string
	now             func() time.Time
}

func NewService(db *gorm.DB, provider PaymentProvider) *Service {
	return &Service{db: db, paymentProvider: provider, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) ConfigureNotifications(mailer email.Sender, appBaseURL string) {
	s.mailer = mailer
	s.appBaseURL = strings.TrimRight(appBaseURL, "/")
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
			return tx.Model(&level).Updates(map[string]any{"on_hand": input.OnHand, "reorder_threshold": input.ReorderThreshold, "updated_at": s.now()}).Error
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		level = InventoryLevel{ID: uuid.New(), OrganizationID: actor.OrganizationID, StoreID: input.StoreID, VariantID: input.VariantID, OnHand: input.OnHand, ReorderThreshold: input.ReorderThreshold}
		return tx.Create(&level).Error
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
			if err := tx.Model(&inv).Updates(map[string]any{"on_hand": gorm.Expr("on_hand - ?", item.Quantity), "updated_at": s.now()}).Error; err != nil {
				return err
			}
			lineTotal, err := multiplyPrice(variant.PriceMinor, item.Quantity)
			if err != nil {
				return err
			}
			order.SubtotalMinor += lineTotal
			if order.SubtotalMinor < 0 || order.SubtotalMinor > math.MaxInt64-order.DeliveryFeeMinor {
				return httperror.BadRequest("Order total overflow")
			}
			orderItem := OrderItem{ID: uuid.New(), OrganizationID: actor.OrganizationID, OrderID: order.ID, VariantID: variant.ID, ProductName: variant.Product.Name, VariantName: variant.Name, Quantity: item.Quantity, UnitPriceMinor: variant.PriceMinor, TotalMinor: lineTotal}
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
		if !canTransition(order.Status, input.Status) {
			return httperror.BadRequest("Invalid order status transition")
		}
		from := order.Status
		if err := tx.Model(&order).Updates(map[string]any{"status": input.Status, "updated_at": s.now()}).Error; err != nil {
			return err
		}
		return s.recordOrderEventTx(tx, actor, order.ID, from, input.Status, "order_transition", input.IdempotencyKey, input.Reason)
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
	if providerName != s.paymentProvider.Name() {
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
	init, err := s.paymentProvider.Initialize(ctx, PaymentInitializeRequest{Reference: reference, Email: input.Email, AmountMinor: order.TotalMinor, Currency: order.Currency, CallbackURL: input.CallbackURL})
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
		verification, err := s.paymentProvider.Verify(ctx, payment.Reference)
		if err != nil {
			return err
		}
		if !verification.Paid {
			return httperror.BadRequest("Payment is not paid")
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
		}
		return tx.Where("organization_id = ? AND id = ?", actor.OrganizationID, payment.ID).First(&payment).Error
	})
	return payment, err
}

func (s *Service) ListPayments(ctx context.Context, actor auth.CurrentUser) ([]Payment, error) {
	var payments []Payment
	err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("created_at DESC").Find(&payments).Error
	return payments, err
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
	return channel, s.db.WithContext(ctx).Create(&channel).Error
}

func (s *Service) ListChannels(ctx context.Context, actor auth.CurrentUser) ([]Channel, error) {
	var channels []Channel
	err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("created_at DESC").Find(&channels).Error
	return channels, err
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
	event := OrderEvent{ID: uuid.New(), OrganizationID: actor.OrganizationID, OrderID: orderID, FromStatus: from, ToStatus: to, EventType: eventType, ActorUserID: actor.ID, IdempotencyKey: idempotencyKey, Reason: reason, Metadata: "{}"}
	return tx.Create(&event).Error
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

func isFulfilmentMode(mode string) bool {
	return mode == FulfilmentPickup || mode == FulfilmentCustomerRider || mode == FulfilmentMerchantRider
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

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
