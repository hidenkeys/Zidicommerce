package runtime

import (
	"time"

	"github.com/google/uuid"
)

type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Name      string  `json:"name"`
	Address   string  `json:"address"`
}

type MessageOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type InboundMessage struct {
	ChannelID              *uuid.UUID     `json:"channel_id,omitempty"`
	Provider               string         `json:"provider"`
	ProviderChannelID      string         `json:"provider_channel_id"`
	ExternalMessageID      string         `json:"external_message_id"`
	ExternalConversationID string         `json:"external_conversation_id"`
	Sender                 string         `json:"sender"`
	CustomerID             *uuid.UUID     `json:"customer_id,omitempty"`
	Text                   string         `json:"text"`
	Location               *Location      `json:"location,omitempty"`
	Attachments            []Attachment   `json:"attachments,omitempty"`
	Metadata               map[string]any `json:"metadata,omitempty"`
	Timestamp              time.Time      `json:"timestamp"`
	AuthenticatedActor     *uuid.UUID     `json:"-"`
	TrustedOrganizationID  *uuid.UUID     `json:"-"`
	SimulatorBotID         *uuid.UUID     `json:"-"`
	SimulatorChannelID     *uuid.UUID     `json:"-"`
	SimulatorStart         bool           `json:"-"`
}

type Attachment struct {
	Type string `json:"type"`
	URL  string `json:"url"`
	ID   string `json:"id"`
}

type OutboundMessage struct {
	Type            string          `json:"type"`
	Text            string          `json:"text,omitempty"`
	Options         []MessageOption `json:"options,omitempty"`
	MediaURL        string          `json:"media_url,omitempty"`
	LocationRequest bool            `json:"location_request,omitempty"`
	Metadata        map[string]any  `json:"metadata,omitempty"`
}

type RuntimeResult struct {
	ConversationID uuid.UUID         `json:"conversation_id"`
	SessionStatus  string            `json:"status"`
	Messages       []OutboundMessage `json:"messages"`
	Handoff        bool              `json:"handoff"`
	Error          *RuntimeError     `json:"error,omitempty"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
}

type RuntimeStartInput struct {
	BotID                  uuid.UUID `json:"bot_id"`
	ChannelID              uuid.UUID `json:"channel_id"`
	ExternalConversationID string    `json:"external_conversation_id"`
	Sender                 string    `json:"sender"`
}

type RuntimeMessageInput struct {
	SessionID         uuid.UUID      `json:"session_id"`
	ExternalMessageID string         `json:"external_message_id"`
	Text              string         `json:"text"`
	Location          *Location      `json:"location,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
}

type SupportHandoffResolveInput struct {
	ResolutionNote string `json:"resolution_note"`
	ResumeBot      bool   `json:"resume_bot"`
}

type SupportHandoffClaimInput struct {
	Note string `json:"note"`
}

type SupportHandoffNoteInput struct {
	Note     string `json:"note"`
	Internal bool   `json:"internal"`
}

type ConversationSummary struct {
	ID                     uuid.UUID  `json:"id"`
	OrganizationID         uuid.UUID  `json:"organization_id"`
	BotID                  uuid.UUID  `json:"bot_id"`
	BotVersionID           uuid.UUID  `json:"bot_version_id"`
	ChannelID              uuid.UUID  `json:"channel_id"`
	CustomerID             *uuid.UUID `json:"customer_id,omitempty"`
	CustomerName           string     `json:"customer_name"`
	CustomerPhone          string     `json:"customer_phone"`
	ExternalConversationID string     `json:"external_conversation_id"`
	CurrentStepKey         string     `json:"current_step_key"`
	ExpectedInput          string     `json:"expected_input"`
	Status                 string     `json:"status"`
	CurrentModule          string     `json:"current_module"`
	LastMessage            string     `json:"last_message"`
	LastMessageDirection   string     `json:"last_message_direction"`
	HandoffStatus          string     `json:"handoff_status"`
	UpdatedAt              time.Time  `json:"updated_at"`
	CreatedAt              time.Time  `json:"created_at"`
}

type WhatsAppWebhookPayload struct {
	Object string `json:"object"`
	Entry  []struct {
		Changes []struct {
			Value struct {
				Metadata struct {
					PhoneNumberID string `json:"phone_number_id"`
				} `json:"metadata"`
				Messages []struct {
					ID        string `json:"id"`
					From      string `json:"from"`
					Timestamp string `json:"timestamp"`
					Type      string `json:"type"`
					Text      struct {
						Body string `json:"body"`
					} `json:"text"`
					Button struct {
						Payload string `json:"payload"`
						Text    string `json:"text"`
					} `json:"button"`
					Interactive struct {
						Type        string `json:"type"`
						ButtonReply struct {
							ID    string `json:"id"`
							Title string `json:"title"`
						} `json:"button_reply"`
						ListReply struct {
							ID    string `json:"id"`
							Title string `json:"title"`
						} `json:"list_reply"`
					} `json:"interactive"`
					Location struct {
						Latitude  float64 `json:"latitude"`
						Longitude float64 `json:"longitude"`
						Name      string  `json:"name"`
						Address   string  `json:"address"`
					} `json:"location"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}
