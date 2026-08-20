package httpapi

import (
	"database/sql"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
)

type Dependencies struct {
	Config       config.Config
	Logger       *slog.Logger
	DB           *sql.DB
	TokenManager *auth.TokenManager
	AuthService  *auth.Service
	OrgHandler   *organization.Handler
}

func New(deps Dependencies) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      deps.Config.AppName,
		ErrorHandler: httperror.Handler,
	})

	app.Use(requestLogger(deps.Logger))
	app.Get("/health", healthHandler(deps.DB))

	v1 := app.Group("/v1")
	v1.Post("/auth/login", loginHandler(deps.AuthService))

	protected := v1.Group("", auth.Middleware(deps.TokenManager))
	protected.Get("/auth/me", meHandler)

	protected.Get("/organizations/current", deps.OrgHandler.Current)

	protected.Get("/organizations", auth.RequireRole(authz.PlatformAdmin), notImplemented("organizations.list"))
	protected.Get("/stores", auth.RequireRole(authz.MerchantAdmin, authz.StoreManager, authz.StoreStaff), notImplemented("stores.list"))
	protected.Get("/catalogue", auth.RequireRole(authz.MerchantAdmin, authz.StoreManager, authz.Viewer), notImplemented("catalogue.list"))
	protected.Get("/inventory", auth.RequireRole(authz.MerchantAdmin, authz.StoreManager, authz.StoreStaff), notImplemented("inventory.list"))
	protected.Get("/customers", auth.RequireRole(authz.MerchantAdmin, authz.SupportAgent, authz.Viewer), notImplemented("customers.list"))
	protected.Get("/orders", auth.RequireRole(authz.MerchantAdmin, authz.StoreManager, authz.StoreStaff, authz.SupportAgent), notImplemented("orders.list"))
	protected.Get("/payments", auth.RequireRole(authz.MerchantAdmin), notImplemented("payments.list"))
	protected.Get("/fulfilment", auth.RequireRole(authz.MerchantAdmin, authz.StoreManager, authz.StoreStaff), notImplemented("fulfilment.list"))
	protected.Get("/channels", auth.RequireRole(authz.MerchantAdmin), notImplemented("channels.list"))
	protected.Get("/bots", auth.RequireRole(authz.MerchantAdmin), notImplemented("bots.list"))

	return app
}

func notImplemented(resource string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
			"data": fiber.Map{
				"resource": resource,
				"status":   "planned_for_later_phase",
			},
		})
	}
}
