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
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, input := range input.Stores {
			store := Store{ID: uuid.New(), OrganizationID: actor.OrganizationID, Name: strings.TrimSpace(input.Name), Code: strings.ToUpper(strings.TrimSpace(input.Code)), Status: defaultString(strings.ToLower(strings.TrimSpace(input.Status)), StatusActive), Address: strings.TrimSpace(input.Address), City: strings.TrimSpace(input.City), Country: strings.TrimSpace(input.Country), Latitude: input.Latitude, Longitude: input.Longitude, Metadata: jsonObject(input.Metadata)}
			if err := tx.Create(&store).Error; err != nil {
				return err
			}
			result.Stores[input.ExternalKey] = store.ID
			if err := s.replaceStoreConfig(tx, actor.OrganizationID, store.ID, input.Hours, input.FulfilmentModes); err != nil {
				return err
			}
		}
		for _, input := range input.Categories {
			category := Category{ID: uuid.New(), OrganizationID: actor.OrganizationID, Name: strings.TrimSpace(input.Name), Slug: strings.ToLower(strings.TrimSpace(input.Slug)), SortOrder: input.SortOrder, Status: defaultString(strings.ToLower(strings.TrimSpace(input.Status)), StatusActive)}
			if err := tx.Create(&category).Error; err != nil {
				return err
			}
			result.Categories[input.ExternalKey] = category.ID
		}
		for _, input := range input.Products {
			categoryID := result.Categories[input.CategoryExternalKey]
			product := Product{ID: uuid.New(), OrganizationID: actor.OrganizationID, CategoryID: &categoryID, Name: strings.TrimSpace(input.Name), Slug: strings.ToLower(strings.TrimSpace(input.Slug)), Description: strings.TrimSpace(input.Description), Status: defaultString(strings.ToLower(strings.TrimSpace(input.Status)), StatusActive), Metadata: jsonObject(input.Metadata)}
			if err := tx.Create(&product).Error; err != nil {
				return err
			}
			result.Products[input.ExternalKey] = product.ID
			for _, variantInput := range input.Variants {
				variant := Variant{ID: uuid.New(), OrganizationID: actor.OrganizationID, ProductID: product.ID, SKU: strings.ToUpper(strings.TrimSpace(variantInput.SKU)), Name: strings.TrimSpace(variantInput.Name), PriceMinor: variantInput.PriceMinor, Currency: defaultString(strings.ToUpper(strings.TrimSpace(variantInput.Currency)), "NGN"), Status: defaultString(strings.ToLower(strings.TrimSpace(variantInput.Status)), StatusActive), Metadata: jsonObject(variantInput.Metadata)}
				if err := tx.Create(&variant).Error; err != nil {
					return err
				}
				result.Variants[variantInput.ExternalKey] = variant.ID
			}
			for _, imageInput := range input.Images {
				image := ProductImage{ID: uuid.New(), OrganizationID: actor.OrganizationID, ProductID: product.ID, URL: strings.TrimSpace(imageInput.URL), AltText: strings.TrimSpace(imageInput.AltText), SortOrder: imageInput.SortOrder}
				if err := tx.Create(&image).Error; err != nil {
					return err
				}
			}
		}
		for _, input := range input.Inventory {
			storeID := result.Stores[input.StoreExternalKey]
			variantID := result.Variants[input.VariantExternalKey]
			level := InventoryLevel{ID: uuid.New(), OrganizationID: actor.OrganizationID, StoreID: storeID, VariantID: variantID, OnHand: input.OnHand, ReorderThreshold: input.ReorderThreshold}
			if err := tx.Create(&level).Error; err != nil {
				return err
			}
		}
		for _, input := range input.Channels {
			channel := Channel{ID: uuid.New(), OrganizationID: actor.OrganizationID, Provider: strings.ToLower(strings.TrimSpace(input.Provider)), DisplayName: strings.TrimSpace(input.DisplayName), PhoneNumberID: strings.TrimSpace(input.PhoneNumberID), DisplayNumber: strings.TrimSpace(input.DisplayNumber), Status: defaultString(strings.ToLower(strings.TrimSpace(input.Status)), "draft"), Config: jsonObject(input.Config), SecretConfig: jsonObject(input.SecretConfig)}
			if err := tx.Create(&channel).Error; err != nil {
				return err
			}
			result.Channels = append(result.Channels, channel.ID)
		}
		job := MerchantImportJob{ID: uuid.New(), OrganizationID: actor.OrganizationID, ActorUserID: &actor.ID, Status: "completed", Source: "json", Summary: jsonValue(result), Errors: "[]"}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		result.JobID = job.ID
		return nil
	})
	return result, err
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
