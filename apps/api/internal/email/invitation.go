package email

import (
	"fmt"
	"html"
	"strings"
	"time"
)

type InvitationContent struct {
	BusinessName   string
	InviterName    string
	RoleLabel      string
	StoreNames     []string
	AcceptURL      string
	ExpiresAt      time.Time
	RecipientEmail string
}

func InvitationURL(appBaseURL, token string) string {
	base := strings.TrimRight(strings.TrimSpace(appBaseURL), "/")
	if base == "" {
		base = "http://localhost:3000"
	}
	return base + "/invitations/accept?token=" + token
}

func BuildInvitation(content InvitationContent) Message {
	business := strings.TrimSpace(content.BusinessName)
	if business == "" {
		business = "a ZidiCommerce workspace"
	}
	subject := "You've been invited to join " + business + " on ZidiCommerce"
	expiry := content.ExpiresAt.UTC().Format("2 January 2006")
	inviter := strings.TrimSpace(content.InviterName)
	if inviter == "" {
		inviter = "A teammate"
	}
	stores := "All assigned stores for this role"
	if len(content.StoreNames) > 0 {
		stores = strings.Join(content.StoreNames, ", ")
	}
	text := fmt.Sprintf(
		"%s invited you to join %s on ZidiCommerce.\n\nZidiCommerce is the workspace your team uses to run orders, catalogue, inventory, and the WhatsApp assistant.\n\nRole: %s\nStore access: %s\nExpires: %s\n\nAccept your invitation:\n%s\n\nIf you were not expecting this email, you can ignore it. This link is unique to you and expires in 7 days.",
		inviter,
		business,
		content.RoleLabel,
		stores,
		expiry,
		content.AcceptURL,
	)
	safeBusiness := html.EscapeString(business)
	safeInviter := html.EscapeString(inviter)
	safeRole := html.EscapeString(content.RoleLabel)
	safeStores := html.EscapeString(stores)
	safeURL := html.EscapeString(content.AcceptURL)
	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="margin:0;padding:0;background:#f6f8fb;font-family:Inter,Arial,sans-serif;color:#101828;">
  <table role="presentation" width="100%%" cellspacing="0" cellpadding="0" style="padding:32px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="560" cellspacing="0" cellpadding="0" style="background:#ffffff;border:1px solid #eaecf0;border-radius:12px;padding:32px;">
          <tr><td style="font-size:13px;font-weight:800;letter-spacing:.08em;text-transform:uppercase;color:#667085;">ZidiCommerce</td></tr>
          <tr><td style="padding-top:16px;font-size:22px;font-weight:700;">You've been invited to join %s</td></tr>
          <tr><td style="padding-top:12px;line-height:1.6;color:#475467;">%s invited you to the %s workspace. ZidiCommerce is where the team manages orders, catalogue, inventory, and the WhatsApp assistant.</td></tr>
          <tr><td style="padding-top:18px;line-height:1.7;"><strong>Role</strong><br>%s<br><br><strong>Store access</strong><br>%s<br><br><strong>Expires</strong><br>%s</td></tr>
          <tr><td style="padding-top:28px;"><a href="%s" style="display:inline-block;background:#101828;color:#ffffff;text-decoration:none;padding:12px 18px;border-radius:8px;font-weight:700;">Accept invitation</a></td></tr>
          <tr><td style="padding-top:20px;font-size:13px;color:#667085;line-height:1.6;">If the button does not work, copy this link:<br>%s</td></tr>
          <tr><td style="padding-top:18px;font-size:12px;color:#98a2b3;">If you were not expecting this email, ignore it. This link is unique to you and expires in 7 days.</td></tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`, safeBusiness, safeInviter, safeBusiness, safeRole, safeStores, html.EscapeString(expiry), safeURL, safeURL)
	return Message{To: content.RecipientEmail, Subject: subject, Text: text, HTML: htmlBody}
}
