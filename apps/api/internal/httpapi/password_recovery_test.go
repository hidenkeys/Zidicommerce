package httpapi

import (
	"database/sql"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/email"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

type recoveryMailbox struct{ messages []email.Message }

func (m *recoveryMailbox) Send(msg email.Message) error {
	m.messages = append(m.messages, msg)
	return nil
}

func TestRecoveryIssuesSingleUseLink(t *testing.T) {
	dsn := os.Getenv("ZIDI_RECOVERY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires a migrated local test database")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	address := uuid.NewString() + "@example.invalid"
	if _, err = db.Exec("INSERT INTO users(email,password_hash,role,status) VALUES ($1,'unused','merchant_admin','active')", address); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DELETE FROM users WHERE email=$1", address)
	mailbox := new(recoveryMailbox)
	deps := Dependencies{DB: db, Mailer: mailbox}
	deps.Config.Email.Mode = "smtp"
	deps.Config.Email.AppBaseURL = "https://admin.example.invalid"
	app := fiber.New(fiber.Config{ErrorHandler: httperror.Handler})
	registerRecovery(app, deps)
	post := func(path, body string, want int) {
		t.Helper()
		req := httptest.NewRequest("POST", "/auth/"+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != want {
			t.Fatalf("%s status %d, want %d", path, resp.StatusCode, want)
		}
	}
	post("forgot-password", `{"email":"nobody@example.invalid"}`, 200)
	if len(mailbox.messages) != 0 {
		t.Fatal("unexpected mail")
	}
	for i := 0; i < 2; i++ {
		post("forgot-password", `{"email":"`+address+`"}`, 200)
	}
	if len(mailbox.messages) != 1 || mailbox.messages[0].To != address {
		t.Fatal("expected one recovery email")
	}
	_, token, ok := strings.Cut(mailbox.messages[0].Text, "https://admin.example.invalid/reset-password#")
	if !ok {
		t.Fatal("invalid reset URL")
	}
	body := `{"password":"test-recovered-password","token":"` + token + `"}`
	post("reset-password", body, 200)
	post("reset-password", body, 400)
	var hash string
	if err = db.QueryRow("SELECT password_hash FROM users WHERE email=$1", address).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("test-recovered-password")) != nil {
		t.Fatal("password did not change")
	}
}
