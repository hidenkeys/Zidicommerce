package core

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
)

type MerchantImportInput struct {
	Stores     []StoreImportInput     `json:"stores"`
	Categories []CategoryImportInput  `json:"categories"`
	Products   []ProductImportInput   `json:"products"`
	Inventory  []InventoryImportInput `json:"inventory"`
	Channels   []ChannelImportInput   `json:"channels"`
}

type StoreImportInput struct {
	ExternalKey     string                     `json:"external_key"`
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

type CategoryImportInput struct {
	ExternalKey string `json:"external_key"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	SortOrder   int    `json:"sort_order"`
	Status      string `json:"status"`
}

type ProductImportInput struct {
	ExternalKey         string               `json:"external_key"`
	CategoryExternalKey string               `json:"category_external_key"`
	Name                string               `json:"name"`
	Slug                string               `json:"slug"`
	Description         string               `json:"description"`
	Status              string               `json:"status"`
	Metadata            string               `json:"metadata"`
	Variants            []VariantImportInput `json:"variants"`
	Images              []ProductImageInput  `json:"images"`
}

type VariantImportInput struct {
	ExternalKey string `json:"external_key"`
	SKU         string `json:"sku"`
	Name        string `json:"name"`
	PriceMinor  int64  `json:"price_minor"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
	Metadata    string `json:"metadata"`
}

type InventoryImportInput struct {
	StoreExternalKey   string `json:"store_external_key"`
	VariantExternalKey string `json:"variant_external_key"`
	OnHand             int    `json:"on_hand"`
	ReorderThreshold   int    `json:"reorder_threshold"`
}

type ChannelImportInput struct {
	Provider      string `json:"provider"`
	DisplayName   string `json:"display_name"`
	PhoneNumberID string `json:"phone_number_id"`
	DisplayNumber string `json:"display_number"`
	Status        string `json:"status"`
	Config        string `json:"config"`
	SecretConfig  string `json:"secret_config"`
}

type MerchantImportResult struct {
	JobID      uuid.UUID            `json:"job_id"`
	Stores     map[string]uuid.UUID `json:"stores"`
	Categories map[string]uuid.UUID `json:"categories"`
	Products   map[string]uuid.UUID `json:"products"`
	Variants   map[string]uuid.UUID `json:"variants"`
	Channels   []uuid.UUID          `json:"channels"`
}

func (s *Service) ImportMerchantConfiguration(ctx context.Context, actor auth.CurrentUser, input MerchantImportInput) (MerchantImportResult, error) {
	if !actor.Role.CanManageOrganization() {
		return MerchantImportResult{}, httperror.Forbidden("You cannot import merchant configuration")
	}
	if err := validateMerchantImport(input); err != nil {
		job := MerchantImportJob{ID: uuid.New(), OrganizationID: actor.OrganizationID, ActorUserID: &actor.ID, Status: "failed", Source: "json", Summary: "{}", Errors: jsonValue([]string{err.Error()})}
		_ = s.db.WithContext(ctx).Create(&job).Error
		return MerchantImportResult{}, err
	}
	result := MerchantImportResult{Stores: map[string]uuid.UUID{}, Categories: map[string]uuid.UUID{}, Products: map[string]uuid.UUID{}, Variants: map[string]uuid.UUID{}}
	job := MerchantImportJob{ID: uuid.New(), OrganizationID: actor.OrganizationID, ActorUserID: &actor.ID, Status: "processing", Source: "json", Summary: "{}", Errors: "[]"}
	if err := s.db.WithContext(ctx).Create(&job).Error; err != nil {
		return MerchantImportResult{}, err
	}
	result.JobID = job.ID
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, input := range input.Stores {
			store, err := s.upsertImportedStore(tx, actor.OrganizationID, input)
			if err != nil {
				return err
			}
			result.Stores[input.ExternalKey] = store.ID
			if err := s.replaceStoreConfig(tx, actor.OrganizationID, store.ID, input.Hours, input.FulfilmentModes); err != nil {
				return err
			}
		}
		for _, input := range input.Categories {
			category, err := s.upsertImportedCategory(tx, actor.OrganizationID, input)
			if err != nil {
				return err
			}
			result.Categories[input.ExternalKey] = category.ID
		}
		for _, input := range input.Products {
			categoryID := result.Categories[input.CategoryExternalKey]
			product, err := s.upsertImportedProduct(tx, actor.OrganizationID, categoryID, input)
			if err != nil {
				return err
			}
			result.Products[input.ExternalKey] = product.ID
			for _, variantInput := range input.Variants {
				variant, err := s.upsertImportedVariant(tx, actor.OrganizationID, product.ID, variantInput)
				if err != nil {
					return err
				}
				result.Variants[variantInput.ExternalKey] = variant.ID
			}
			if len(input.Images) > 0 {
				if err := s.replaceImportedProductImages(tx, actor.OrganizationID, product.ID, input.Images); err != nil {
					return err
				}
			}
		}
		for _, input := range input.Inventory {
			storeID := result.Stores[input.StoreExternalKey]
			variantID := result.Variants[input.VariantExternalKey]
			if _, err := s.upsertImportedInventory(tx, actor.OrganizationID, storeID, variantID, input); err != nil {
				return err
			}
		}
		for _, input := range input.Channels {
			channel, err := s.upsertImportedChannel(tx, actor.OrganizationID, input)
			if err != nil {
				return err
			}
			result.Channels = append(result.Channels, channel.ID)
		}
		return tx.Model(&job).Updates(map[string]any{"status": "completed", "summary": jsonValue(result), "errors": "[]"}).Error
	})
	if err != nil {
		_ = s.db.WithContext(ctx).Model(&job).Updates(map[string]any{"status": "failed", "errors": jsonValue([]string{err.Error()})}).Error
	}
	return result, err
}

func (s *Service) upsertImportedStore(tx *gorm.DB, organizationID uuid.UUID, input StoreImportInput) (Store, error) {
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	store := Store{}
	err := tx.Where("organization_id = ? AND code = ?", organizationID, code).First(&store).Error
	if err == nil {
		updates := map[string]any{"name": strings.TrimSpace(input.Name), "status": defaultString(strings.ToLower(strings.TrimSpace(input.Status)), StatusActive), "address": strings.TrimSpace(input.Address), "city": strings.TrimSpace(input.City), "country": strings.TrimSpace(input.Country), "latitude": input.Latitude, "longitude": input.Longitude, "metadata": jsonObject(input.Metadata)}
		if err := tx.Model(&store).Updates(updates).Error; err != nil {
			return Store{}, err
		}
		return store, tx.Where("organization_id = ? AND code = ?", organizationID, code).First(&store).Error
	}
	if err != gorm.ErrRecordNotFound {
		return Store{}, err
	}
	store = Store{ID: uuid.New(), OrganizationID: organizationID, Name: strings.TrimSpace(input.Name), Code: code, Status: defaultString(strings.ToLower(strings.TrimSpace(input.Status)), StatusActive), Address: strings.TrimSpace(input.Address), City: strings.TrimSpace(input.City), Country: strings.TrimSpace(input.Country), Latitude: input.Latitude, Longitude: input.Longitude, Metadata: jsonObject(input.Metadata)}
	return store, tx.Create(&store).Error
}

func (s *Service) upsertImportedCategory(tx *gorm.DB, organizationID uuid.UUID, input CategoryImportInput) (Category, error) {
	slug := strings.ToLower(strings.TrimSpace(input.Slug))
	category := Category{}
	err := tx.Where("organization_id = ? AND slug = ?", organizationID, slug).First(&category).Error
	if err == nil {
		updates := map[string]any{"name": strings.TrimSpace(input.Name), "sort_order": input.SortOrder, "status": defaultString(strings.ToLower(strings.TrimSpace(input.Status)), StatusActive)}
		if err := tx.Model(&category).Updates(updates).Error; err != nil {
			return Category{}, err
		}
		return category, tx.Where("organization_id = ? AND slug = ?", organizationID, slug).First(&category).Error
	}
	if err != gorm.ErrRecordNotFound {
		return Category{}, err
	}
	category = Category{ID: uuid.New(), OrganizationID: organizationID, Name: strings.TrimSpace(input.Name), Slug: slug, SortOrder: input.SortOrder, Status: defaultString(strings.ToLower(strings.TrimSpace(input.Status)), StatusActive)}
	return category, tx.Create(&category).Error
}

func (s *Service) upsertImportedProduct(tx *gorm.DB, organizationID, categoryID uuid.UUID, input ProductImportInput) (Product, error) {
	slug := strings.ToLower(strings.TrimSpace(input.Slug))
	product := Product{}
	err := tx.Where("organization_id = ? AND slug = ?", organizationID, slug).First(&product).Error
	if err == nil {
		updates := map[string]any{"category_id": categoryID, "name": strings.TrimSpace(input.Name), "description": strings.TrimSpace(input.Description), "status": defaultString(strings.ToLower(strings.TrimSpace(input.Status)), StatusActive), "metadata": jsonObject(input.Metadata)}
		if err := tx.Model(&product).Updates(updates).Error; err != nil {
			return Product{}, err
		}
		return product, tx.Where("organization_id = ? AND slug = ?", organizationID, slug).First(&product).Error
	}
	if err != gorm.ErrRecordNotFound {
		return Product{}, err
	}
	product = Product{ID: uuid.New(), OrganizationID: organizationID, CategoryID: &categoryID, Name: strings.TrimSpace(input.Name), Slug: slug, Description: strings.TrimSpace(input.Description), Status: defaultString(strings.ToLower(strings.TrimSpace(input.Status)), StatusActive), Metadata: jsonObject(input.Metadata)}
	return product, tx.Create(&product).Error
}

func (s *Service) upsertImportedVariant(tx *gorm.DB, organizationID, productID uuid.UUID, input VariantImportInput) (Variant, error) {
	sku := strings.ToUpper(strings.TrimSpace(input.SKU))
	variant := Variant{}
	err := tx.Where("organization_id = ? AND sku = ?", organizationID, sku).First(&variant).Error
	if err == nil {
		updates := map[string]any{"product_id": productID, "name": strings.TrimSpace(input.Name), "price_minor": input.PriceMinor, "currency": defaultString(strings.ToUpper(strings.TrimSpace(input.Currency)), "NGN"), "status": defaultString(strings.ToLower(strings.TrimSpace(input.Status)), StatusActive), "metadata": jsonObject(input.Metadata)}
		if err := tx.Model(&variant).Updates(updates).Error; err != nil {
			return Variant{}, err
		}
		return variant, tx.Where("organization_id = ? AND sku = ?", organizationID, sku).First(&variant).Error
	}
	if err != gorm.ErrRecordNotFound {
		return Variant{}, err
	}
	variant = Variant{ID: uuid.New(), OrganizationID: organizationID, ProductID: productID, SKU: sku, Name: strings.TrimSpace(input.Name), PriceMinor: input.PriceMinor, Currency: defaultString(strings.ToUpper(strings.TrimSpace(input.Currency)), "NGN"), Status: defaultString(strings.ToLower(strings.TrimSpace(input.Status)), StatusActive), Metadata: jsonObject(input.Metadata)}
	return variant, tx.Create(&variant).Error
}

func (s *Service) replaceImportedProductImages(tx *gorm.DB, organizationID, productID uuid.UUID, images []ProductImageInput) error {
	if err := tx.Where("organization_id = ? AND product_id = ?", organizationID, productID).Delete(&ProductImage{}).Error; err != nil {
		return err
	}
	for _, imageInput := range images {
		image := ProductImage{ID: uuid.New(), OrganizationID: organizationID, ProductID: productID, URL: strings.TrimSpace(imageInput.URL), AltText: strings.TrimSpace(imageInput.AltText), SortOrder: imageInput.SortOrder}
		if err := tx.Create(&image).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) upsertImportedInventory(tx *gorm.DB, organizationID, storeID, variantID uuid.UUID, input InventoryImportInput) (InventoryLevel, error) {
	level := InventoryLevel{}
	err := tx.Where("organization_id = ? AND store_id = ? AND variant_id = ?", organizationID, storeID, variantID).First(&level).Error
	if err == nil {
		updates := map[string]any{"on_hand": input.OnHand, "reorder_threshold": input.ReorderThreshold}
		if err := tx.Model(&level).Updates(updates).Error; err != nil {
			return InventoryLevel{}, err
		}
		return level, tx.Where("organization_id = ? AND store_id = ? AND variant_id = ?", organizationID, storeID, variantID).First(&level).Error
	}
	if err != gorm.ErrRecordNotFound {
		return InventoryLevel{}, err
	}
	level = InventoryLevel{ID: uuid.New(), OrganizationID: organizationID, StoreID: storeID, VariantID: variantID, OnHand: input.OnHand, ReorderThreshold: input.ReorderThreshold}
	return level, tx.Create(&level).Error
}

func (s *Service) upsertImportedChannel(tx *gorm.DB, organizationID uuid.UUID, input ChannelImportInput) (Channel, error) {
	provider := strings.ToLower(strings.TrimSpace(input.Provider))
	phoneNumberID := strings.TrimSpace(input.PhoneNumberID)
	displayName := strings.TrimSpace(input.DisplayName)
	channel := Channel{}
	query := tx.Where("organization_id = ? AND provider = ?", organizationID, provider)
	if phoneNumberID != "" {
		query = query.Where("phone_number_id = ?", phoneNumberID)
	} else {
		query = query.Where("display_name = ?", displayName)
	}
	err := query.First(&channel).Error
	if err == nil {
		updates := map[string]any{"display_name": displayName, "phone_number_id": phoneNumberID, "display_number": strings.TrimSpace(input.DisplayNumber), "status": defaultString(strings.ToLower(strings.TrimSpace(input.Status)), "draft"), "config": jsonObject(input.Config)}
		if strings.TrimSpace(input.SecretConfig) != "" {
			updates["secret_config"] = jsonObject(input.SecretConfig)
		}
		if err := tx.Model(&channel).Updates(updates).Error; err != nil {
			return Channel{}, err
		}
		return channel, tx.Where("organization_id = ? AND id = ?", organizationID, channel.ID).First(&channel).Error
	}
	if err != gorm.ErrRecordNotFound {
		return Channel{}, err
	}
	if phoneNumberID != "" {
		var existing Channel
		if err := tx.Where("provider = ? AND phone_number_id = ? AND organization_id <> ?", provider, phoneNumberID, organizationID).First(&existing).Error; err == nil {
			return Channel{}, httperror.BadRequest("Channel phone_number_id is already connected to another organization")
		} else if err != gorm.ErrRecordNotFound {
			return Channel{}, err
		}
	}
	channel = Channel{ID: uuid.New(), OrganizationID: organizationID, Provider: provider, DisplayName: displayName, PhoneNumberID: phoneNumberID, DisplayNumber: strings.TrimSpace(input.DisplayNumber), Status: defaultString(strings.ToLower(strings.TrimSpace(input.Status)), "draft"), Config: jsonObject(input.Config), SecretConfig: jsonObject(input.SecretConfig)}
	return channel, tx.Create(&channel).Error
}

func (s *Service) ListMerchantImportJobs(ctx context.Context, actor auth.CurrentUser) ([]MerchantImportJob, error) {
	if !actor.Role.CanManageOrganization() {
		return nil, httperror.Forbidden("You cannot view merchant imports")
	}
	var jobs []MerchantImportJob
	err := s.db.WithContext(ctx).Where("organization_id = ?", actor.OrganizationID).Order("created_at DESC").Limit(50).Find(&jobs).Error
	return jobs, err
}

func validateMerchantImport(input MerchantImportInput) error {
	stores := map[string]bool{}
	for _, store := range input.Stores {
		if strings.TrimSpace(store.ExternalKey) == "" || strings.TrimSpace(store.Name) == "" || strings.TrimSpace(store.Code) == "" {
			return httperror.BadRequest("Each store requires external_key, name, and code")
		}
		if stores[store.ExternalKey] {
			return httperror.BadRequest("Duplicate store external_key")
		}
		stores[store.ExternalKey] = true
	}
	categories := map[string]bool{}
	for _, category := range input.Categories {
		if strings.TrimSpace(category.ExternalKey) == "" || strings.TrimSpace(category.Name) == "" || strings.TrimSpace(category.Slug) == "" {
			return httperror.BadRequest("Each category requires external_key, name, and slug")
		}
		categories[category.ExternalKey] = true
	}
	variants := map[string]bool{}
	skus := map[string]bool{}
	for _, product := range input.Products {
		if strings.TrimSpace(product.ExternalKey) == "" || strings.TrimSpace(product.Name) == "" || strings.TrimSpace(product.Slug) == "" || !categories[product.CategoryExternalKey] {
			return httperror.BadRequest("Each product requires external_key, name, slug, and a valid category_external_key")
		}
		for _, variant := range product.Variants {
			if strings.TrimSpace(variant.ExternalKey) == "" || strings.TrimSpace(variant.SKU) == "" || strings.TrimSpace(variant.Name) == "" || variant.PriceMinor < 0 {
				return httperror.BadRequest("Each variant requires external_key, sku, name, and a non-negative price")
			}
			if variants[variant.ExternalKey] || skus[strings.ToUpper(strings.TrimSpace(variant.SKU))] {
				return httperror.BadRequest("Duplicate variant external_key or sku")
			}
			variants[variant.ExternalKey] = true
			skus[strings.ToUpper(strings.TrimSpace(variant.SKU))] = true
		}
	}
	for _, inventory := range input.Inventory {
		if !stores[inventory.StoreExternalKey] || !variants[inventory.VariantExternalKey] || inventory.OnHand < 0 || inventory.ReorderThreshold < 0 {
			return httperror.BadRequest("Each inventory row requires valid store/variant references and non-negative quantities")
		}
	}
	return nil
}
