package core

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	"gorm.io/gorm"
)

// AdoptChannelInput moves an already-configured WhatsApp number from whichever
// workspace currently holds it to another workspace, carrying its credentials
// across inside the database.
//
// A provider phone number is globally unique across workspaces, so a pilot
// tenant cannot simply be handed a number that another tenant already owns.
// Doing this through the API would mean re-entering the Meta credentials by
// hand; copying them row to row keeps them inside the database, where they
// already live.
type AdoptChannelInput struct {
	PhoneNumberID string
	TargetOrgSlug string
	VerifyToken   string
	BotID         string
}

// AdoptWhatsAppNumber is idempotent: once the target workspace owns the number
// and is active, re-running it changes nothing.
func (s *Service) AdoptWhatsAppNumber(ctx context.Context, input AdoptChannelInput, log *slog.Logger) error {
	phone := strings.TrimSpace(input.PhoneNumberID)
	slug := strings.TrimSpace(input.TargetOrgSlug)
	if phone == "" || slug == "" {
		return nil
	}
	if log == nil {
		log = slog.Default()
	}

	var target organization.Organization
	if err := s.db.WithContext(ctx).Where("slug = ?", slug).First(&target).Error; err != nil {
		return fmt.Errorf("target organization %q not found: %w", slug, err)
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source Channel
		err := tx.Where("provider = ? AND phone_number_id = ?", "whatsapp", phone).First(&source).Error
		switch {
		case err == gorm.ErrRecordNotFound:
			source = Channel{}
		case err != nil:
			return err
		}

		if source.ID != uuid.Nil && source.OrganizationID == target.ID {
			log.Info("whatsapp number already owned by the target workspace",
				"organization_id", target.ID, "channel_id", source.ID, "status", source.Status)
			if source.Status == StatusActive {
				return nil
			}
		}

		// The credentials travel with the number so they never have to be
		// re-entered (or pass through configuration) to move it. The webhook
		// verify token belongs to the number's Meta app rather than to a
		// workspace, so it travels too.
		carriedSecrets := source.SecretConfig
		carriedDisplay := source.DisplayNumber
		carriedConfig := jsonMap(source.Config)

		var destination Channel
		err = tx.Where("organization_id = ? AND provider = ?", target.ID, "whatsapp").
			Order("updated_at DESC").First(&destination).Error
		if err == gorm.ErrRecordNotFound {
			destination = Channel{ID: uuid.New(), OrganizationID: target.ID, Provider: "whatsapp",
				DisplayName: "WhatsApp", Status: "draft", Config: "{}", SecretConfig: "{}"}
			if err := tx.Create(&destination).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		// Release the number first: the unique index ignores status, so the old
		// holder must let go before the new one can claim it.
		if source.ID != uuid.Nil && source.ID != destination.ID {
			if err := tx.Model(&Channel{}).Where("id = ?", source.ID).Updates(map[string]any{
				"phone_number_id": "", "status": StatusInactive, "updated_at": s.now(),
			}).Error; err != nil {
				return err
			}
			log.Info("released whatsapp number from previous workspace",
				"from_organization_id", source.OrganizationID, "channel_id", source.ID,
				"to_organization_id", target.ID)
		}

		config := jsonMap(destination.Config)
		if token := strings.TrimSpace(input.VerifyToken); token != "" {
			config["verify_token"] = token
		}
		if stringFromAny(config["verify_token"]) == "" {
			if carried := stringFromAny(carriedConfig["verify_token"]); carried != "" {
				config["verify_token"] = carried
			}
		}
		if botID := strings.TrimSpace(input.BotID); botID != "" {
			config["bot_id"] = botID
		}
		if stringFromAny(config["bot_id"]) == "" {
			var publishedBotID string
			if err := tx.Raw(
				`SELECT id::text FROM bots WHERE organization_id = ? AND published_version_id IS NOT NULL ORDER BY updated_at DESC LIMIT 1`,
				target.ID).Scan(&publishedBotID).Error; err == nil && publishedBotID != "" {
				config["bot_id"] = publishedBotID
			}
		}

		updates := map[string]any{
			"phone_number_id": phone,
			"status":          StatusActive,
			"config":          jsonValue(config),
			"updated_at":      s.now(),
		}
		if strings.TrimSpace(destination.DisplayNumber) == "" && strings.TrimSpace(carriedDisplay) != "" {
			updates["display_number"] = carriedDisplay
		}
		if merged := mergeSecrets(destination.SecretConfig, carriedSecrets); merged != "" {
			updates["secret_config"] = merged
		}
		if err := tx.Model(&Channel{}).Where("id = ?", destination.ID).Updates(updates).Error; err != nil {
			return mapChannelNumberConflict(err)
		}

		log.Info("whatsapp number adopted",
			"organization_id", target.ID, "channel_id", destination.ID,
			"has_bot", stringFromAny(config["bot_id"]) != "",
			"has_verify_token", stringFromAny(config["verify_token"]) != "",
			"has_access_token", channelSecretPresent(mergeSecrets(destination.SecretConfig, carriedSecrets), "access_token"),
			"has_app_secret", channelSecretPresent(mergeSecrets(destination.SecretConfig, carriedSecrets), "app_secret"))
		return nil
	})
}

// mergeSecrets keeps whatever the destination already holds and fills the gaps
// from the source, so adopting a number never erases a credential.
func mergeSecrets(destination, source string) string {
	merged := jsonMap(destination)
	for key, value := range jsonMap(source) {
		if stringFromAny(merged[key]) == "" && stringFromAny(value) != "" {
			merged[key] = value
		}
	}
	if len(merged) == 0 {
		return ""
	}
	return jsonValue(merged)
}

// channelSecretPresent reports whether a credential exists without revealing it.
func channelSecretPresent(secretConfig, key string) bool {
	return stringFromAny(jsonMap(secretConfig)[key]) != ""
}
