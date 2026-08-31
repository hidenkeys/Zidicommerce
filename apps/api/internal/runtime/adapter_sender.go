package runtime

import (
	"context"

	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
)

type AdapterChannelSender struct {
	sender channelplatform.OutboundSender
}

func NewAdapterChannelSender(sender channelplatform.OutboundSender) *AdapterChannelSender {
	return &AdapterChannelSender{sender: sender}
}

func (s *AdapterChannelSender) Send(ctx context.Context, channel core.Channel, recipient string, message OutboundMessage, _ map[string]any, idempotencyKey string) (ProviderSendResult, error) {
	command := ChannelOutboundCommand(channel.Provider, channel.ID, nil, recipient, recipient, message, idempotencyKey)
	result, err := s.sender.Send(ctx, command)
	return ProviderSendResult{ProviderMessageID: result.ProviderMessageID, Response: result.ResponseMetadata}, err
}
