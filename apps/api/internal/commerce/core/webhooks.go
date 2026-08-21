package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	paymentWebhookProcessed = "processed"
	paymentWebhookIgnored   = "ignored"
	paymentWebhookFailed    = "failed"
)

type PaymentWebhookResult struct {
	EventID   uuid.UUID  `json:"event_id"`
	Provider  string     `json:"provider"`
	Reference string     `json:"reference"`
	Status    string     `json:"status"`
	PaymentID *uuid.UUID `json:"payment_id,omitempty"`
	OrderID   *uuid.UUID `json:"order_id,omitempty"`
}

type paystackWebhookPayload struct {
	Event string `json:"event"`
	Data  struct {
		ID        any    `json:"id"`
		Reference string `json:"reference"`
		Status    string `json:"status"`
		Amount    int64  `json:"amount"`
		Currency  string `json:"currency"`
	} `json:"data"`
}

func (s *Service) HandlePaystackWebhook(ctx context.Context, body []byte, signature string) (PaymentWebhookResult, error) {
	var payload paystackWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return PaymentWebhookResult{}, httperror.BadRequest("Invalid Paystack webhook payload")
	}
	payload.Data.Reference = strings.TrimSpace(payload.Data.Reference)
	if payload.Event == "" || payload.Data.Reference == "" {
		return PaymentWebhookResult{}, httperror.BadRequest("Paystack webhook event and reference are required")
	}
	var webhookPayment Payment
	paymentFound := s.db.WithContext(ctx).Where("provider = ? AND reference = ?", "paystack", payload.Data.Reference).First(&webhookPayment).Error == nil
	signatureSecret := s.paystackSecret
	if paymentFound && s.paymentSecretStore != nil {
		if merchantSecret, err := s.paymentSecretStore.GetSecret(ctx, webhookPayment.OrganizationID, "paystack", "secret_key"); err == nil && strings.TrimSpace(merchantSecret) != "" {
			signatureSecret = merchantSecret
		} else if err != nil && err != gorm.ErrRecordNotFound {
			return PaymentWebhookResult{}, err
		}
	}
	if !verifyPaystackSignature(signatureSecret, signature, body) {
		return PaymentWebhookResult{}, httperror.Forbidden("Invalid Paystack webhook signature")
	}
	externalID := paystackEventID(payload)
	rawPayload := jsonValue(payload)
	var result PaymentWebhookResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing PaymentWebhookEvent
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider = ? AND external_event_id = ?", "paystack", externalID).First(&existing).Error
		if err == nil {
			result = PaymentWebhookResult{EventID: existing.ID, Provider: existing.Provider, Reference: existing.Reference, Status: existing.Status}
			return nil
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}

		event := PaymentWebhookEvent{ID: uuid.New(), Provider: "paystack", ExternalEventID: externalID, Reference: payload.Data.Reference, EventType: payload.Event, Status: "processing", Payload: rawPayload}
		if err := tx.Create(&event).Error; err != nil {
			return err
		}
		result = PaymentWebhookResult{EventID: event.ID, Provider: event.Provider, Reference: event.Reference}

		var payment Payment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("provider = ? AND reference = ?", "paystack", payload.Data.Reference).First(&payment).Error; err != nil {
			status := paymentWebhookFailed
			if err == gorm.ErrRecordNotFound {
				status = paymentWebhookIgnored
			}
			now := s.now()
			_ = tx.Model(&event).Updates(map[string]any{"status": status, "error_message": "payment reference not found", "processed_at": &now, "updated_at": now}).Error
			result.Status = status
			return nil
		}
		event.OrganizationID = &payment.OrganizationID
		result.PaymentID = &payment.ID
		result.OrderID = &payment.OrderID

		if payload.Event != "charge.success" || strings.ToLower(payload.Data.Status) != "success" {
			now := s.now()
			if err := tx.Model(&event).Updates(map[string]any{"organization_id": payment.OrganizationID, "status": paymentWebhookIgnored, "processed_at": &now, "updated_at": now}).Error; err != nil {
				return err
			}
			result.Status = paymentWebhookIgnored
			return nil
		}
		if payload.Data.Amount > 0 && payload.Data.Amount != payment.AmountMinor {
			now := s.now()
			if err := tx.Model(&event).Updates(map[string]any{"organization_id": payment.OrganizationID, "status": paymentWebhookFailed, "error_message": "amount mismatch", "processed_at": &now, "updated_at": now}).Error; err != nil {
				return err
			}
			result.Status = paymentWebhookFailed
			return nil
		}
		if payload.Data.Currency != "" && strings.ToUpper(payload.Data.Currency) != strings.ToUpper(payment.Currency) {
			now := s.now()
			if err := tx.Model(&event).Updates(map[string]any{"organization_id": payment.OrganizationID, "status": paymentWebhookFailed, "error_message": "currency mismatch", "processed_at": &now, "updated_at": now}).Error; err != nil {
				return err
			}
			result.Status = paymentWebhookFailed
			return nil
		}
		provider, err := s.resolveUsablePaymentProvider(ctx, payment.OrganizationID, payment.Provider)
		if err != nil {
			return err
		}
		if provider == nil {
			return httperror.BadRequest("Payment provider is not configured")
		}
		verification, err := provider.Verify(ctx, payment.Reference)
		if err != nil {
			now := s.now()
			if updateErr := tx.Model(&event).Updates(map[string]any{"organization_id": payment.OrganizationID, "status": paymentWebhookFailed, "error_message": publicWebhookError(err), "processed_at": &now, "updated_at": now}).Error; updateErr != nil {
				return updateErr
			}
			result.Status = paymentWebhookFailed
			return nil
		}
		if !verification.Paid {
			now := s.now()
			if err := tx.Model(&event).Updates(map[string]any{"organization_id": payment.OrganizationID, "status": paymentWebhookFailed, "error_message": "provider verification is not paid", "processed_at": &now, "updated_at": now}).Error; err != nil {
				return err
			}
			result.Status = paymentWebhookFailed
			return nil
		}
		if err := verifyPaymentMatches(payment, verification); err != nil {
			now := s.now()
			if updateErr := tx.Model(&event).Updates(map[string]any{"organization_id": payment.OrganizationID, "status": paymentWebhookFailed, "error_message": publicWebhookError(err), "processed_at": &now, "updated_at": now}).Error; updateErr != nil {
				return updateErr
			}
			result.Status = paymentWebhookFailed
			return nil
		}
		now := s.now()
		if payment.Status != PaymentPaid {
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
				actor := auth.CurrentUser{ID: uuid.Nil, OrganizationID: payment.OrganizationID, Role: authz.PlatformAdmin}
				if err := s.recordOrderEventTx(tx, actor, order.ID, OrderAwaitingPayment, OrderPaid, "payment_webhook_verified", "paystack:"+externalID, ""); err != nil {
					return err
				}
				if err := s.recordOrderNotificationTx(tx, payment.OrganizationID, order.ID, "payment_confirmed", "Payment confirmed for order "+order.OrderNumber+"."); err != nil {
					return err
				}
			}
		}
		if err := tx.Model(&event).Updates(map[string]any{"organization_id": payment.OrganizationID, "status": paymentWebhookProcessed, "processed_at": &now, "updated_at": now}).Error; err != nil {
			return err
		}
		result.Status = paymentWebhookProcessed
		return nil
	})
	if err == nil && result.Status == paymentWebhookProcessed && result.OrderID != nil {
		orgID := uuid.Nil
		if result.PaymentID != nil {
			var payment Payment
			if s.db.WithContext(ctx).Select("organization_id, order_id").Where("id = ?", *result.PaymentID).First(&payment).Error == nil {
				orgID = payment.OrganizationID
				s.fireAfterPaymentPaid(ctx, orgID, payment.OrderID)
			}
		}
	}
	return result, err
}

func verifyPaystackSignature(secret, signature string, body []byte) bool {
	secret = strings.TrimSpace(secret)
	signature = strings.TrimSpace(signature)
	if secret == "" || signature == "" {
		return false
	}
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func paystackEventID(payload paystackWebhookPayload) string {
	rawID := strings.TrimSpace(fmt.Sprint(payload.Data.ID))
	if rawID == "" || rawID == "<nil>" {
		rawID = payload.Event + ":" + payload.Data.Reference + ":" + strings.ToLower(payload.Data.Status)
	}
	return rawID
}

func publicWebhookError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 240 {
		return message[:240]
	}
	return message
}
