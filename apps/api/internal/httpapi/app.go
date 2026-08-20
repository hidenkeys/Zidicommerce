package httpapi

import (
	"database/sql"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/organization"
	runtimeengine "github.com/hidenkeys/zidicommerce/apps/api/internal/runtime"
)

type Dependencies struct {
	Config       config.Config
	Logger       *slog.Logger
	DB           *sql.DB
	TokenManager *auth.TokenManager
	AuthService  *auth.Service
	OrgHandler   *organization.Handler
	Commerce     *core.Handler
	Bot          *bot.Handler
	Runtime      *runtimeengine.Handler
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
	v1.Post("/auth/register", registerHandler(deps.AuthService))
	deps.Commerce.RegisterPublic(v1)
	deps.Runtime.RegisterPublic(v1)

	protected := v1.Group("", auth.Middleware(deps.TokenManager))
	protected.Get("/auth/me", meHandler)

	protected.Get("/organizations/current", deps.OrgHandler.Current)
	deps.Commerce.Register(protected)
	deps.Bot.Register(protected)
	deps.Runtime.Register(protected)

	return app
}
