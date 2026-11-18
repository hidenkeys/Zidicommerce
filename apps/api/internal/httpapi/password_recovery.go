package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/email"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"golang.org/x/crypto/bcrypt"
)

func registerRecovery(router fiber.Router, deps Dependencies) {
	limit := limiter.New(limiter.Config{Max: 12, Expiration: time.Minute})
	router.Post("/auth/forgot-password", limit, func(c *fiber.Ctx) error {
		var input struct{ Email string }
		if c.BodyParser(&input) != nil {
			return httperror.BadRequest("Enter a valid email address")
		}
		address := strings.ToLower(strings.TrimSpace(input.Email))
		parsed, err := mail.ParseAddress(address)
		if err != nil || parsed.Address != address {
			return httperror.BadRequest("Enter a valid email address")
		}
		if deps.Config.Email.Mode != "smtp" || deps.Mailer == nil {
			return httperror.Unavailable("Password recovery email is unavailable. Contact your workspace administrator.")
		}
		secret := make([]byte, 32)
		if _, err = rand.Read(secret); err != nil {
			return httperror.Internal("Could not start password recovery")
		}
		token := base64.RawURLEncoding.EncodeToString(secret)
		digest := sha256.Sum256([]byte(token))
		// Refuse ambiguous legacy identities. Throttle resends atomically across replicas.
		var recipient string
		err = deps.DB.QueryRowContext(c.UserContext(), `WITH candidates AS (
    SELECT id,email FROM users WHERE lower(email)=$1 AND status='active'
  ), issued AS (
    INSERT INTO password_recovery(user_id,token_digest)
    SELECT id,$2 FROM candidates WHERE (SELECT count(*) FROM candidates)=1
    ON CONFLICT(user_id) DO UPDATE SET token_digest=excluded.token_digest, issued_at=now()
    WHERE password_recovery.issued_at < now()-interval '2 minutes' RETURNING user_id
  ) SELECT email FROM candidates JOIN issued ON user_id=id`, address, digest[:]).Scan(&recipient)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return httperror.Internal("Could not start password recovery")
		}
		if err == nil {
			link := strings.TrimRight(deps.Config.Email.AppBaseURL, "/") + "/reset-password#" + token
			if err := deps.Mailer.Send(email.Message{To: recipient, Subject: "Reset your Zidi Commerce password", Text: "Open this link within 20 minutes to choose a new password. If you did not request this, you can ignore this email.\n\n" + link}); err != nil {
				deps.Logger.Warn("password recovery email delivery failed")
			}
		}
		return c.JSON(fiber.Map{"data": true})
	})
	router.Post("/auth/reset-password", limit, func(c *fiber.Ctx) error {
		var input struct{ Token, Password string }
		if c.BodyParser(&input) != nil || len(input.Token) != 43 || len(input.Password) < 8 || len(input.Password) > 72 {
			return httperror.BadRequest("Use a valid reset link and a password between 8 and 72 bytes")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return httperror.Internal("Could not update password")
		}
		digest := sha256.Sum256([]byte(input.Token))
		res, err := deps.DB.ExecContext(c.UserContext(), `WITH consumed AS (
    DELETE FROM password_recovery WHERE token_digest=$1 AND issued_at > now()-interval '20 minutes' RETURNING user_id
  ) UPDATE users SET password_hash=$2,updated_at=now() FROM consumed WHERE users.id=consumed.user_id AND users.status='active'`, digest[:], string(hash))
		if err != nil {
			return httperror.Internal("Could not update password")
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return httperror.BadRequest("This reset link is invalid or has expired")
		}
		return c.JSON(fiber.Map{"data": true})
	})
}
