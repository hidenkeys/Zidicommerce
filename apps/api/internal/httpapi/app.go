package httpapi

import (
	"database/sql"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/ai"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	whatsappadapter "github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform/whatsapp"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/config"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/fieldservice"
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
	Channels     *channelplatform.Handler
	WhatsApp     *whatsappadapter.Handler
	Bot          *bot.Handler
	Runtime      *runtimeengine.Handler
	Field        *fieldservice.Handler
	AI           *ai.Handler
}

func New(deps Dependencies) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      deps.Config.AppName,
		ErrorHandler: httperror.Handler,
	})

	app.Use(requestLogger(deps.Logger))
	app.Use(cors.New(cors.Config{
		AllowOrigins: deps.Config.CORSOrigins,
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, X-Request-ID",
		AllowMethods: "GET, POST, PUT, PATCH, DELETE, OPTIONS",
		MaxAge:       86400,
	}))
	app.Get("/health", healthHandler(deps.DB))

	v1 := app.Group("/v1")
	v1.Post("/auth/login", loginHandler(deps.AuthService))
	v1.Post("/auth/register", registerHandler(deps.AuthService))
	deps.Commerce.RegisterPublic(v1)
	if deps.WhatsApp != nil {
		deps.WhatsApp.RegisterPublic(v1)
	} else {
		deps.Runtime.RegisterPublic(v1)
	}

	protected := v1.Group("", auth.Middleware(deps.TokenManager))
	protected.Get("/auth/me", meHandler)

	protected.Get("/organizations/current", deps.OrgHandler.Current)
	deps.Commerce.Register(protected)
	if deps.Channels != nil {
		deps.Channels.Register(protected)
	}
	if deps.WhatsApp != nil {
		deps.WhatsApp.RegisterProtected(protected)
	}
	deps.Bot.Register(protected)
	deps.Runtime.Register(protected)
	if deps.AI != nil {
		deps.AI.Register(protected)
	}
	if deps.Field != nil {
		deps.Field.Register(protected)
	}

	return app
}
