package whatsapp

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/channelplatform"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const flowPrivateKey = "flow_private_key"

type FlowConfig struct {
	Enabled                               bool
	TestWABAID, TestPhoneID, GraphBaseURL string
}

func (s *Service) ConfigureFlows(config FlowConfig, client *http.Client) {
	s.flowConfig = config
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	s.flowHTTP = client
}

func (s *Service) flowConnection(ctx context.Context, actor auth.CurrentUser, id uuid.UUID) (Configuration, error) {
	if !actor.Role.HasPermission(authz.PermissionChannelsManage) || !actor.Role.HasPermission(authz.PermissionBotPublish) {
		return Configuration{}, httperror.Forbidden("Publishing Flows requires channel management and journey publication permissions")
	}
	if !s.flowConfig.Enabled || s.flowConfig.TestWABAID == "" || s.flowConfig.TestPhoneID == "" {
		return Configuration{}, httperror.BadRequest("Flows are disabled: configure a designated test WABA and phone first")
	}
	config, err := s.findOrDefaultConfiguration(ctx, actor.OrganizationID, id)
	if err != nil {
		return config, err
	}
	if config.WhatsAppBusinessAccountID != s.flowConfig.TestWABAID || config.PhoneNumberID != s.flowConfig.TestPhoneID {
		return config, httperror.Forbidden("This connection is not the designated Flow test account")
	}
	detail, err := s.platform.GetConnection(ctx, actor, id)
	if err != nil {
		return config, err
	}
	if detail.Connection.Provider != Provider || detail.Connection.Environment != channelplatform.EnvironmentSandbox {
		return config, httperror.BadRequest("A designated sandbox WhatsApp connection is required")
	}
	return config, nil
}
func (s *Service) flowGraph(ctx context.Context, config Configuration, method, path, contentType string, body io.Reader) (map[string]any, error) {
	token, err := s.ResolveCredential(ctx, config.OrganizationID, config.ConnectionID, CredentialAccessToken)
	if err != nil {
		return nil, httperror.BadRequest("The connection access token is unavailable")
	}
	base := strings.TrimRight(s.flowConfig.GraphBaseURL, "/")
	if base == "" {
		base = "https://graph.facebook.com"
	}
	request, err := http.NewRequestWithContext(ctx, method, base+"/"+config.GraphAPIVersion+"/"+path, body)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := s.flowHTTP.Do(request)
	if err != nil {
		return nil, httperror.BadRequest("Meta Flow request did not complete; reconcile the provider record before retrying creation")
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
	if err != nil {
		return nil, httperror.BadRequest("Meta Flow response could not be read")
	}
	var result map[string]any
	if json.Unmarshal(data, &result) != nil {
		return nil, httperror.BadRequest("Meta returned an invalid Flow response")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, httperror.BadRequest(fmt.Sprintf("Meta rejected this Flow operation (HTTP %d). Check test-account permissions and provider validation.", response.StatusCode))
	}
	return result, nil
}
func (s *Service) PrepareJourneyFlow(ctx context.Context, actor auth.CurrentUser, connectionID, versionID uuid.UUID) (bot.JourneyFlowDeployment, error) {
	config, err := s.flowConnection(ctx, actor, connectionID)
	if err != nil {
		return bot.JourneyFlowDeployment{}, err
	}
	service := bot.NewService(s.db)
	graph, err := service.GetConfiguration(ctx, actor, versionID)
	if err != nil {
		return bot.JourneyFlowDeployment{}, err
	}
	d := bot.JourneyFromConfiguration(graph)
	if !bot.IsJourney(graph.Version) || d.ConnectionID == nil || *d.ConnectionID != connectionID {
		return bot.JourneyFlowDeployment{}, httperror.BadRequest("Bind this journey draft to the selected connection")
	}
	compiled := bot.CompileJourneyFlow(d)
	if len(compiled.Errors) > 0 {
		return bot.JourneyFlowDeployment{}, httperror.BadRequest("Resolve Flow compilation errors before provider deployment")
	}
	if !strings.HasPrefix(s.publicURL, "https://") {
		return bot.JourneyFlowDeployment{}, httperror.BadRequest("A public HTTPS Flow data endpoint is required")
	}
	raw, _ := json.Marshal(compiled.JSON)
	hash := sha256.Sum256(raw)
	revisionHash := hex.EncodeToString(hash[:])
	row := bot.JourneyFlowDeployment{ID: uuid.New(), OrganizationID: actor.OrganizationID, BotID: graph.Version.BotID, VersionID: versionID, ConnectionID: connectionID, Revision: graph.Version.VersionNumber, Status: "draft", Errors: "[]", Warnings: marshal(compiled.Warnings), CompiledHash: revisionHash}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			row = bot.JourneyFlowDeployment{}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND version_id = ? AND connection_id = ?", actor.OrganizationID, versionID, connectionID).First(&row).Error; err != nil {
				return err
			}
			if row.Status == "published" || row.Status == "deprecated" {
				return httperror.BadRequest("Published provider Flows are immutable; clone a journey draft")
			}
			if row.ProviderFlowID == "" {
				return httperror.BadRequest("A prior creation attempt requires provider reconciliation; it will not be repeated automatically")
			}
		}
		return nil
	})
	if err != nil {
		return row, err
	}
	failed := func(err error) (bot.JourneyFlowDeployment, error) {
		s.db.WithContext(ctx).Model(&row).Updates(map[string]any{"status": "failed", "errors": `["Provider operation failed; see the safe API error and Meta test account."]`})
		return row, err
	}
	if row.ProviderFlowID == "" {
		form := url.Values{"name": {d.Name + " v" + fmt.Sprint(row.Revision)}, "categories": {`["OTHER"]`}, "endpoint_uri": {s.publicURL + "/v1/journey-flows/" + connectionID.String() + "/data"}}
		var previous bot.JourneyFlowDeployment
		if s.db.WithContext(ctx).Where("organization_id = ? AND bot_id = ? AND connection_id = ? AND status = ?", actor.OrganizationID, row.BotID, connectionID, "published").Order("revision DESC").First(&previous).Error == nil {
			form.Set("clone_flow_id", previous.ProviderFlowID)
		}
		result, err := s.flowGraph(ctx, config, "POST", config.WhatsAppBusinessAccountID+"/flows", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		if err != nil {
			return failed(err)
		}
		row.ProviderFlowID = valueString(result["id"])
		if row.ProviderFlowID == "" {
			return failed(httperror.BadRequest("Meta did not return a Flow ID"))
		}
		if err := s.db.WithContext(ctx).Model(&row).Update("provider_flow_id", row.ProviderFlowID).Error; err != nil {
			return row, err
		}
	}
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	part, err := writer.CreateFormFile("file", "flow.json")
	if err != nil {
		return failed(err)
	}
	part.Write(raw)
	writer.WriteField("name", "flow.json")
	writer.WriteField("asset_type", "FLOW_JSON")
	writer.Close()
	if _, err := s.flowGraph(ctx, config, "POST", row.ProviderFlowID+"/assets", writer.FormDataContentType(), &upload); err != nil {
		return failed(err)
	}
	row.CompiledHash = revisionHash
	row.Status = "draft"
	row.Errors = "[]"
	row.Warnings = marshal(compiled.Warnings)
	if err := s.db.WithContext(ctx).Model(&row).Updates(map[string]any{"compiled_hash": row.CompiledHash, "status": row.Status, "errors": row.Errors, "warnings": row.Warnings}).Error; err != nil {
		return row, err
	}
	return s.JourneyFlowOperation(ctx, actor, connectionID, versionID, "refresh")
}
func (s *Service) JourneyFlowOperation(ctx context.Context, actor auth.CurrentUser, connectionID, versionID uuid.UUID, operation string) (bot.JourneyFlowDeployment, error) {
	if operation != "publish" && operation != "deprecate" {
		return s.journeyFlowOperation(ctx, actor, connectionID, versionID, operation)
	}
	var result bot.JourneyFlowDeployment
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var version bot.BotVersion
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", actor.OrganizationID, versionID).First(&version).Error; err != nil {
			return httperror.NotFound("Journey version not found")
		}
		copy := *s
		copy.db = tx
		var err error
		result, err = copy.journeyFlowOperation(ctx, actor, connectionID, versionID, operation)
		return err
	})
	return result, err
}
func (s *Service) journeyFlowOperation(ctx context.Context, actor auth.CurrentUser, connectionID, versionID uuid.UUID, operation string) (bot.JourneyFlowDeployment, error) {
	config, err := s.flowConnection(ctx, actor, connectionID)
	if err != nil {
		return bot.JourneyFlowDeployment{}, err
	}
	var row bot.JourneyFlowDeployment
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND connection_id = ? AND version_id = ?", actor.OrganizationID, connectionID, versionID).First(&row).Error; err != nil {
		return row, httperror.NotFound("Flow deployment not found")
	}
	path := row.ProviderFlowID + "?fields=id,status,validation_errors,preview"
	method := "GET"
	if operation == "publish" {
		if row.Status != "validated" {
			return row, httperror.BadRequest("Provider validation must pass before publishing")
		}
		graph, err := bot.NewService(s.db).GetConfiguration(ctx, actor, versionID)
		if err != nil {
			return row, err
		}
		compiled := bot.CompileJourneyFlow(bot.JourneyFromConfiguration(graph))
		raw, _ := json.Marshal(compiled.JSON)
		hash := sha256.Sum256(raw)
		if len(compiled.Errors) > 0 || hex.EncodeToString(hash[:]) != row.CompiledHash {
			return row, httperror.BadRequest("The draft changed after upload. Upload and validate it again")
		}
		if _, err := s.ResolveCredential(ctx, actor.OrganizationID, connectionID, flowPrivateKey); err != nil {
			return row, httperror.BadRequest("Register the encrypted Flow endpoint key before publishing")
		}
		path = row.ProviderFlowID + "/publish"
		method = "POST"
	} else if operation == "deprecate" {
		if row.Status != "published" {
			return row, httperror.BadRequest("Only published Flows can be deprecated")
		}
		path = row.ProviderFlowID + "/deprecate"
		method = "POST"
	} else if operation != "refresh" {
		return row, httperror.BadRequest("Unknown Flow operation")
	}
	result, err := s.flowGraph(ctx, config, method, path, "", nil)
	if err != nil {
		return row, err
	}
	if operation == "refresh" {
		if status := strings.ToLower(valueString(result["status"])); status == "published" || status == "deprecated" {
			row.Status = status
		} else {
			row.Status = "validated"
		}
		if failures, ok := result["validation_errors"].([]any); ok && len(failures) > 0 {
			row.Status = "failed"
			row.Errors = `["Meta reported Flow schema validation errors. Correct the compiled draft before publishing."]`
		} else {
			row.Errors = "[]"
		}
		if preview, ok := result["preview"].(map[string]any); ok {
			u := valueString(preview["preview_url"])
			parsed, e := url.Parse(u)
			if e == nil && parsed.Scheme == "https" && (parsed.Host == "business.facebook.com" || parsed.Host == "www.facebook.com") {
				row.PreviewURL = u
			}
		}
	} else if operation == "publish" {
		row.Status = "published"
	} else {
		row.Status = "deprecated"
	}
	if err := s.db.WithContext(ctx).Model(&row).Updates(map[string]any{"status": row.Status, "errors": row.Errors, "preview_url": row.PreviewURL, "updated_at": s.now()}).Error; err != nil {
		return row, err
	}
	err = audit(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, connectionID, "journey_flow_"+operation, map[string]any{"version_id": versionID, "deployment_id": row.ID, "status": row.Status})
	return row, err
}
func (s *Service) RegisterFlowKey(ctx context.Context, actor auth.CurrentUser, connectionID uuid.UUID) error {
	config, err := s.flowConnection(ctx, actor, connectionID)
	if err != nil {
		return err
	}
	if s.encrypted == nil {
		return httperror.BadRequest("Encrypted channel secret storage is required")
	}
	private, err := s.ResolveCredential(ctx, actor.OrganizationID, connectionID, flowPrivateKey)
	if err != nil {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return err
		}
		raw, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return err
		}
		private = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}))
		ref, err := s.encrypted.Save(ctx, actor.OrganizationID, connectionID, flowPrivateKey, private)
		if err != nil {
			return err
		}
		if _, err := s.platform.UpsertCredentialReference(ctx, actor, connectionID, channelplatform.CredentialReferenceInput{CredentialType: flowPrivateKey, SecretRef: ref, Status: channelplatform.CredentialPresent}); err != nil {
			return err
		}
	}
	block, _ := pem.Decode([]byte(private))
	if block == nil {
		return httperror.BadRequest("Flow private key is invalid")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return httperror.BadRequest("Flow key must be RSA")
	}
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return err
	}
	form := url.Values{"business_public_key": {string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public}))}}
	_, err = s.flowGraph(ctx, config, "POST", config.PhoneNumberID+"/whatsapp_business_encryption", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	return audit(s.db.WithContext(ctx), &actor.OrganizationID, &actor.ID, connectionID, "journey_flow_key_registered", map[string]any{"key_bits": 2048})
}
func marshal(value any) string { raw, _ := json.Marshal(value); return string(raw) }
