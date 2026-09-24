package meta

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
)

type webhookPayload struct {
	Object string         `json:"object"`
	Entry  []webhookEntry `json:"entry"`
}

type webhookEntry struct {
	ID        string             `json:"id"`
	Time      int64              `json:"time"`
	Messaging []webhookMessaging `json:"messaging"`
}

type webhookMessaging struct {
	Sender    webhookParty     `json:"sender"`
	Recipient webhookParty     `json:"recipient"`
	Timestamp int64            `json:"timestamp"`
	Message   *webhookMessage  `json:"message"`
	Postback  *webhookPostback `json:"postback"`
	Reaction  *webhookReaction `json:"reaction"`
	Delivery  *webhookDelivery `json:"delivery"`
	Read      *webhookRead     `json:"read"`
}

type webhookParty struct {
	ID string `json:"id"`
}

type webhookMessage struct {
	ID          string              `json:"mid"`
	Text        string              `json:"text"`
	IsEcho      bool                `json:"is_echo"`
	Attachments []webhookAttachment `json:"attachments"`
	QuickReply  *struct {
		Payload string `json:"payload"`
	} `json:"quick_reply"`
}

type webhookAttachment struct {
	Type    string `json:"type"`
	Payload struct {
		URL string `json:"url"`
	} `json:"payload"`
}

type webhookPostback struct {
	ID      string `json:"mid"`
	Title   string `json:"title"`
	Payload string `json:"payload"`
}

type webhookReaction struct {
	MessageID string `json:"mid"`
	Action    string `json:"action"`
	Reaction  string `json:"reaction"`
	Emoji     string `json:"emoji"`
}

type webhookDelivery struct {
	MessageIDs []string `json:"mids"`
	Watermark  int64    `json:"watermark"`
}

type webhookRead struct {
	Watermark int64 `json:"watermark"`
}

func decodeWebhook(body []byte) (webhookPayload, error) {
	var payload webhookPayload
	if len(body) == 0 || json.Unmarshal(body, &payload) != nil || len(payload.Entry) == 0 {
		return webhookPayload{}, errors.New("invalid Meta webhook payload")
	}
	return payload, nil
}

func providerForObject(object string) string {
	switch normalize(object) {
	case "instagram":
		return ProviderInstagram
	case "page":
		return ProviderFacebook
	default:
		return ""
	}
}

func normalizeInboundEvent(provider string, connectionID uuid.UUID, entry webhookEntry, messaging webhookMessaging, fallback time.Time) (channelplatform.InboundEvent, bool) {
	if messaging.Message != nil && !messaging.Message.IsEcho && messaging.Sender.ID != "" {
		message := messaging.Message
		messageType := "text"
		media := make([]channelplatform.MediaReference, 0, len(message.Attachments))
		for index, attachment := range message.Attachments {
			media = append(media, channelplatform.MediaReference{Type: normalize(attachment.Type), Reference: attachment.Payload.URL, ContentType: ""})
			if index == 0 && strings.TrimSpace(message.Text) == "" {
				messageType = normalize(attachment.Type)
			}
		}
		metadata := map[string]any{"provider_account_id": entry.ID}
		if message.QuickReply != nil {
			messageType = "postback"
			metadata["action_id"] = message.QuickReply.Payload
		}
		return channelplatform.InboundEvent{Provider: provider, ChannelConnectionID: connectionID, ProviderEventID: message.ID, ExternalCustomerID: messaging.Sender.ID, ExternalMessageID: message.ID, ExternalConversationID: messaging.Sender.ID, MessageType: messageType, Body: message.Text, Media: media, Timestamp: metaTimestamp(messaging.Timestamp, entry.Time, fallback), Metadata: metadata}, message.ID != ""
	}
	if messaging.Postback != nil && messaging.Sender.ID != "" {
		id := messaging.Postback.ID
		if id == "" {
			id = messagingEventID(entry, messaging, "postback")
		}
		return channelplatform.InboundEvent{Provider: provider, ChannelConnectionID: connectionID, ProviderEventID: id, ExternalCustomerID: messaging.Sender.ID, ExternalMessageID: id, ExternalConversationID: messaging.Sender.ID, MessageType: "postback", Body: first(messaging.Postback.Title, messaging.Postback.Payload), Timestamp: metaTimestamp(messaging.Timestamp, entry.Time, fallback), Metadata: map[string]any{"provider_account_id": entry.ID, "action_id": messaging.Postback.Payload}}, true
	}
	if messaging.Reaction != nil && messaging.Sender.ID != "" {
		id := messagingEventID(entry, messaging, "reaction")
		body := strings.TrimSpace(strings.Join([]string{messaging.Reaction.Action, first(messaging.Reaction.Emoji, messaging.Reaction.Reaction)}, " "))
		return channelplatform.InboundEvent{Provider: provider, ChannelConnectionID: connectionID, ProviderEventID: id, ExternalCustomerID: messaging.Sender.ID, ExternalMessageID: id, ExternalConversationID: messaging.Sender.ID, MessageType: "reaction", Body: body, Timestamp: metaTimestamp(messaging.Timestamp, entry.Time, fallback), Metadata: map[string]any{"provider_account_id": entry.ID, "reacted_to_message_id": messaging.Reaction.MessageID, "reaction_action": messaging.Reaction.Action}}, true
	}
	return channelplatform.InboundEvent{}, false
}

func messagingEventID(entry webhookEntry, messaging webhookMessaging, eventType string) string {
	body := strings.Join([]string{entry.ID, messaging.Sender.ID, messaging.Recipient.ID, fmt.Sprint(messaging.Timestamp), eventType}, ":")
	if messaging.Postback != nil {
		body += ":" + messaging.Postback.Payload
	}
	if messaging.Reaction != nil {
		body += ":" + messaging.Reaction.MessageID + ":" + messaging.Reaction.Action + ":" + messaging.Reaction.Reaction
	}
	sum := sha256.Sum256([]byte(body))
	return eventType + ":" + hex.EncodeToString(sum[:12])
}

func metaTimestamp(messageMS, entrySeconds int64, fallback time.Time) time.Time {
	if messageMS > 0 {
		return time.UnixMilli(messageMS).UTC()
	}
	if entrySeconds > 0 {
		return time.Unix(entrySeconds, 0).UTC()
	}
	if fallback.IsZero() {
		return time.Now().UTC()
	}
	return fallback.UTC()
}

func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return ""
}
