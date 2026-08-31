package runtime

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

// InboundMessageFromChannelEvent is the provider-neutral handoff into the
// conversation runtime. Provider adapters normalize first; the runtime only
// receives stable channel and customer identities.
func InboundMessageFromChannelEvent(event channelplatform.InboundEvent) InboundMessage {
	attachments := make([]Attachment, 0, len(event.Media))
	for _, media := range event.Media {
		attachments = append(attachments, Attachment{Type: media.Type, URL: media.Reference})
	}
	channelID := event.ChannelConnectionID
	return InboundMessage{
		ChannelID:              &channelID,
		Provider:               strings.ToLower(strings.TrimSpace(event.Provider)),
		ExternalMessageID:      strings.TrimSpace(event.ExternalMessageID),
		ExternalConversationID: strings.TrimSpace(event.ExternalConversationID),
		Sender:                 strings.TrimSpace(event.ExternalCustomerID),
		Text:                   event.Body,
		Attachments:            attachments,
		Metadata:               event.Metadata,
		Timestamp:              event.Timestamp,
	}
}

// ChannelOutboundCommand converts a runtime response into the command an
// adapter will eventually send. It deliberately has no phone-number, page, or
// other provider-specific requirement.
func ChannelOutboundCommand(provider string, channelID uuid.UUID, conversationID *uuid.UUID, externalCustomerID, externalConversationID string, message OutboundMessage, idempotencyKey string) channelplatform.OutboundCommand {
	media := []channelplatform.MediaReference{}
	if strings.TrimSpace(message.MediaURL) != "" {
		media = append(media, channelplatform.MediaReference{Type: message.Type, Reference: message.MediaURL})
	}
	return channelplatform.OutboundCommand{
		Provider:               strings.ToLower(strings.TrimSpace(provider)),
		ChannelConnectionID:    channelID,
		ConversationID:         conversationID,
		ExternalCustomerID:     strings.TrimSpace(externalCustomerID),
		ExternalConversationID: strings.TrimSpace(externalConversationID),
		MessageType:            message.Type,
		Body:                   message.Text,
		Media:                  media,
		Actions:                outboundActions(message.Options),
		IdempotencyKey:         strings.TrimSpace(idempotencyKey),
		Metadata:               message.Metadata,
	}
}

func outboundActions(options []MessageOption) []channelplatform.OutboundAction {
	actions := make([]channelplatform.OutboundAction, 0, len(options))
	for _, option := range options {
		actions = append(actions, channelplatform.OutboundAction{ID: option.ID, Label: option.Label, Description: option.Description})
	}
	return actions
}

func (s *Service) ProcessChannelInbound(ctx context.Context, organizationID uuid.UUID, event channelplatform.InboundEvent) error {
	if organizationID == uuid.Nil || event.ChannelConnectionID == uuid.Nil {
		return httperror.Forbidden("Channel organization could not be resolved")
	}
	var channel core.Channel
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ? AND provider = ? AND status IN ?", organizationID, event.ChannelConnectionID, strings.ToLower(strings.TrimSpace(event.Provider)), runtimeUsableChannelStatuses).First(&channel).Error; err != nil {
		return httperror.NotFound("Channel connection not found")
	}
	input := InboundMessageFromChannelEvent(event)
	input.TrustedOrganizationID = &organizationID
	result, err := s.ProcessMessage(ctx, input)
	if err != nil {
		return err
	}
	if _, dispatchErr := s.DispatchOutbound(ctx, channel, input, result); dispatchErr != nil {
		s.log.Warn("channel outbound dispatch failed after inbound processing", "organization_id", organizationID, "channel_id", channel.ID, "provider", channel.Provider, "error", publicSendError(dispatchErr))
	}
	return nil
}
