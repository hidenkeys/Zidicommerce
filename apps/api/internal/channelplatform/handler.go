package channelplatform

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

type Handler struct {
	service *Service
	oauth   *OAuthService
}

func NewHandler(service *Service, oauth ...*OAuthService) *Handler {
	handler := &Handler{service: service}
	if len(oauth) > 0 {
		handler.oauth = oauth[0]
	}
	return handler
}

func (h *Handler) Register(router fiber.Router) {
	router.Get("/channel-platform/providers", auth.RequirePermission(authz.PermissionChannelsView), h.listProviders)
	router.Get("/channel-platform/connections", auth.RequirePermission(authz.PermissionChannelsView), h.listConnections)
	router.Post("/channel-platform/connections", auth.RequirePermission(authz.PermissionChannelsManage), h.createConnection)
	router.Get("/channel-platform/connections/:id", auth.RequirePermission(authz.PermissionChannelsView), h.getConnection)
	router.Patch("/channel-platform/connections/:id", auth.RequirePermission(authz.PermissionChannelsManage), h.updateConnection)
	router.Delete("/channel-platform/connections/:id", auth.RequirePermission(authz.PermissionChannelsManage), h.archiveConnection)
	router.Get("/channel-platform/connections/:id/health", auth.RequirePermission(authz.PermissionChannelsView), h.getHealth)
	router.Get("/channel-platform/connections/:id/metrics", auth.RequirePermission(authz.PermissionChannelsView), h.getMetrics)
	router.Get("/channel-platform/connections/:id/events", auth.RequirePermission(authz.PermissionChannelsView), h.listEvents)
	router.Get("/channel-platform/connections/:id/credentials", auth.RequirePermission(authz.PermissionChannelsView), h.listCredentials)
	router.Post("/channel-platform/connections/:id/credentials", auth.RequirePermission(authz.PermissionChannelsManage), h.upsertCredential)
	if h.oauth != nil {
		router.Post("/channel-platform/connections/:id/oauth/start", auth.RequirePermission(authz.PermissionChannelsManage), h.startOAuth)
		router.Post("/channel-platform/connections/:id/oauth/callback", auth.RequirePermission(authz.PermissionChannelsManage), h.completeOAuthCallback)
		router.Get("/channel-platform/connections/:id/oauth/assets", auth.RequirePermission(authz.PermissionChannelsView), h.listOAuthAssets)
		router.Post("/channel-platform/connections/:id/oauth/assets/:asset_id/select", auth.RequirePermission(authz.PermissionChannelsManage), h.selectOAuthAsset)
		router.Post("/channel-platform/connections/:id/oauth/cancel", auth.RequirePermission(authz.PermissionChannelsManage), h.cancelOAuth)
		router.Post("/channel-platform/connections/:id/oauth/refresh", auth.RequirePermission(authz.PermissionChannelsManage), h.refreshOAuth)
		router.Post("/channel-platform/connections/:id/oauth/disconnect", auth.RequirePermission(authz.PermissionChannelsManage), h.disconnectOAuth)
	}
}

func (h *Handler) listProviders(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"data": ProviderCatalog()})
}

func (h *Handler) listConnections(c *fiber.Ctx) error {
	data, err := h.service.ListConnections(c.UserContext(), currentUser(c))
	return respond(c, data, err)
}

func (h *Handler) getConnection(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.GetConnection(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) createConnection(c *fiber.Ctx) error {
	var input CreateConnectionInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateConnection(c.UserContext(), currentUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) updateConnection(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	var input UpdateConnectionInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateConnection(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) archiveConnection(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.ArchiveConnection(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) getHealth(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.GetHealthSummary(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) getMetrics(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	from, err := dateQuery(c.Query("from"))
	if err != nil {
		return err
	}
	to, err := dateQuery(c.Query("to"))
	if err != nil {
		return err
	}
	data, err := h.service.GetMetricsSummary(c.UserContext(), currentUser(c), id, from, to)
	return respond(c, data, err)
}

func (h *Handler) listEvents(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	limit, _ := strconv.Atoi(c.Query("limit", "100"))
	data, err := h.service.ListProviderEvents(c.UserContext(), currentUser(c), id, limit)
	return respond(c, data, err)
}

func (h *Handler) listCredentials(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.service.ListCredentials(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) upsertCredential(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	var input CredentialReferenceInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpsertCredentialReference(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) startOAuth(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	var input OAuthStartInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.oauth.Start(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) completeOAuthCallback(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	var input OAuthCallbackInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.oauth.CompleteCallback(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) listOAuthAssets(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.oauth.ListAssets(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) selectOAuthAsset(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	assetID, err := uuid.Parse(c.Params("asset_id"))
	if err != nil {
		return httperror.BadRequest("Invalid provider asset ID")
	}
	var input OAuthAssetSelectionInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.oauth.SelectAsset(c.UserContext(), currentUser(c), id, assetID, input)
	return respond(c, data, err)
}

func (h *Handler) cancelOAuth(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	var input OAuthCancelInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.oauth.Cancel(c.UserContext(), currentUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) refreshOAuth(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.oauth.Refresh(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) disconnectOAuth(c *fiber.Ctx) error {
	id, err := connectionID(c)
	if err != nil {
		return err
	}
	data, err := h.oauth.Disconnect(c.UserContext(), currentUser(c), id)
	return respond(c, data, err)
}

func currentUser(c *fiber.Ctx) auth.CurrentUser {
	user, err := auth.GetCurrentUser(c)
	if err != nil {
		panic(err)
	}
	return user
}

func connectionID(c *fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, httperror.BadRequest("Invalid channel connection ID")
	}
	return id, nil
}

func bind(c *fiber.Ctx, target any) error {
	if err := c.BodyParser(target); err != nil {
		return httperror.BadRequest("Invalid JSON payload")
	}
	return nil
}

func dateQuery(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, httperror.BadRequest("Dates must use YYYY-MM-DD")
	}
	return parsed, nil
}

func respond(c *fiber.Ctx, data any, err error) error {
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": data})
}
