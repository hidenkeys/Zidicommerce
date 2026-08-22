package fieldservice

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Register(router fiber.Router) {
	router.Get("/field/settings", h.getSettings)
	router.Put("/field/settings", auth.RequireRole(authz.PlatformAdmin, authz.MerchantAdmin), h.updateSettings)
	router.Get("/field/overview", auth.RequireRole(authz.PlatformAdmin, authz.MerchantAdmin), h.overview)
	router.Get("/field/pools", h.listPools)
	router.Post("/field/pools", auth.RequireRole(authz.PlatformAdmin, authz.MerchantAdmin), h.upsertPool)
	router.Get("/field/providers", h.listProviders)
	router.Post("/field/providers", auth.RequireRole(authz.PlatformAdmin, authz.MerchantAdmin), h.upsertProvider)
	router.Post("/field/providers/:id/availability", h.setAvailability)
	router.Get("/field/requests", h.listRequests)
	router.Get("/field/requests/:id", h.getRequest)
	router.Get("/field/requests/:id/matches", auth.RequireRole(authz.PlatformAdmin, authz.MerchantAdmin), h.listMatches)
	router.Get("/field/requests/:id/messages", h.listMessages)
	router.Post("/field/requests/:id/messages", h.postMessage)
	router.Post("/field/requests/:id/status", h.transition)
	router.Post("/field/requests/:id/quotes", h.createQuote)
	router.Post("/field/requests/:id/take-over", auth.RequireRole(authz.PlatformAdmin, authz.MerchantAdmin), h.takeOver)
	router.Post("/field/requests/:id/release", auth.RequireRole(authz.PlatformAdmin, authz.MerchantAdmin), h.release)
	router.Post("/field/requests/:id/close", auth.RequireRole(authz.PlatformAdmin, authz.MerchantAdmin), h.closeConversation)
	router.Get("/field/inbox", h.inbox)
	router.Get("/field/home", h.home)
	router.Post("/field/dispatch/:id/accept", h.accept)
	router.Post("/field/dispatch/:id/decline", h.decline)
	router.Get("/field/quotes", h.listQuotes)
	router.Post("/field/quotes/:id/approve", h.approveQuote)
	router.Post("/field/quotes/:id/decline", h.declineQuote)
	router.Get("/field/conversations", auth.RequireRole(authz.PlatformAdmin, authz.MerchantAdmin), h.conversations)
	router.Get("/field/transactions", auth.RequireRole(authz.PlatformAdmin, authz.MerchantAdmin), h.transactions)
}

func (h *Handler) getSettings(c *fiber.Ctx) error {
	data, err := h.service.GetSettings(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) updateSettings(c *fiber.Ctx) error {
	var input Settings
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateSettings(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) overview(c *fiber.Ctx) error {
	data, err := h.service.Overview(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) listPools(c *fiber.Ctx) error {
	actor := mustUser(c)
	if actor.Role.CanManageOrganization() {
		data, err := h.service.ListManageablePools(c.UserContext(), actor)
		return respond(c, data, err)
	}
	data, err := h.service.ListPools(c.UserContext(), actor)
	return respond(c, data, err)
}

func (h *Handler) upsertPool(c *fiber.Ctx) error {
	var input Pool
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpsertPool(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) listProviders(c *fiber.Ctx) error {
	data, err := h.service.ListProviderViews(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) upsertProvider(c *fiber.Ctx) error {
	var input struct {
		Provider
		PoolIDs  []uuid.UUID `json:"pool_ids"`
		Password string      `json:"password"`
	}
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpsertProvider(c.UserContext(), mustUser(c), input.Provider, input.PoolIDs, input.Password)
	return respond(c, data, err)
}

func (h *Handler) setAvailability(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input struct {
		Availability string `json:"availability"`
	}
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.SetProviderAvailability(c.UserContext(), mustUser(c), id, input.Availability)
	return respond(c, data, err)
}

func (h *Handler) listRequests(c *fiber.Ctx) error {
	data, err := h.service.ListRequests(c.UserContext(), mustUser(c), c.Query("status"))
	return respond(c, data, err)
}

func (h *Handler) getRequest(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetRequest(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) listMatches(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListMatches(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) listMessages(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListMessages(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) postMessage(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input struct {
		Body string `json:"body"`
	}
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.PostMessage(c.UserContext(), mustUser(c), id, input.Body)
	return respond(c, data, err)
}

func (h *Handler) transition(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input struct {
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.TransitionJob(c.UserContext(), mustUser(c), id, input.Status, input.Notes)
	return respond(c, data, err)
}

func (h *Handler) createQuote(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input QuoteInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateQuote(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) takeOver(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	return respond(c, fiber.Map{"ok": true}, h.service.TakeOverConversation(c.UserContext(), mustUser(c), id))
}

func (h *Handler) release(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	return respond(c, fiber.Map{"ok": true}, h.service.ReleaseToProvider(c.UserContext(), mustUser(c), id))
}

func (h *Handler) closeConversation(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	return respond(c, fiber.Map{"ok": true}, h.service.CloseConversation(c.UserContext(), mustUser(c), id))
}

func (h *Handler) inbox(c *fiber.Ctx) error {
	data, err := h.service.ListProviderInbox(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) home(c *fiber.Ctx) error {
	data, err := h.service.ProviderHome(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) accept(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.AcceptDispatch(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) decline(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	return respond(c, fiber.Map{"ok": true}, h.service.DeclineDispatch(c.UserContext(), mustUser(c), id))
}

func (h *Handler) listQuotes(c *fiber.Ctx) error {
	data, err := h.service.ListQuotes(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) approveQuote(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ApproveQuote(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) declineQuote(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	return respond(c, fiber.Map{"ok": true}, h.service.DeclineQuote(c.UserContext(), mustUser(c), id))
}

func (h *Handler) conversations(c *fiber.Ctx) error {
	data, err := h.service.ListActiveConversations(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) transactions(c *fiber.Ctx) error {
	data, err := h.service.ListTransactions(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func mustUser(c *fiber.Ctx) auth.CurrentUser {
	user, _ := auth.GetCurrentUser(c)
	return user
}

func paramID(c *fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil {
		return uuid.Nil, httperror.BadRequest("Invalid identifier")
	}
	return id, nil
}

func bind(c *fiber.Ctx, dest any) error {
	if err := c.BodyParser(dest); err != nil {
		return httperror.BadRequest("Invalid JSON body")
	}
	return nil
}

func respond(c *fiber.Ctx, data any, err error) error {
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": data})
}
