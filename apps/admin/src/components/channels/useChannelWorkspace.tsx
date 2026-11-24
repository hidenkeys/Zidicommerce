import { type FormEvent, useEffect, useRef, useState, type ComponentType } from "react";
import { Camera, Check, MessageCircle, MessageSquareText, Music2, X, type LucideProps } from "lucide-react";
import { API_BASE_URL, apiDelete, apiGet, apiPatch, apiPost } from "../../api/client";
import { useAuth } from "../../auth";
import { oauthConnectionFromSession } from "./OAuthChannelPanel";
import { humanStatus, maskPhone } from "../../lib/format";
import { launchMetaEmbeddedSignup, type MetaSignupLaunch } from "../../lib/metaEmbeddedSignup";
import { hasPermission } from "../../lib/permissions";
import { useNavigate, useParams } from "react-router-dom";
import type { ChannelConnection, ConnectionDetail, ProviderCatalogEntry, ProviderEvent, WhatsAppConfiguration, WhatsAppForm, WhatsAppTemplate, WhatsAppContactState, WhatsAppAdvancedMetrics, WhatsAppPolicyBlock, TemplateForm, WhatsAppHealth, EmbeddedSignupCompletion } from "./channel-types";

export type ProviderDefinition = {
  key: string;
  label: string;
  description: string;
  icon: ComponentType<LucideProps>;
};

export const providers: ProviderDefinition[] = [
  { key: "whatsapp", label: "WhatsApp", description: "Customer messaging, commerce conversations, and human support.", icon: MessageCircle },
  { key: "instagram", label: "Instagram", description: "Professional-account Direct messages through the shared Zidi conversation runtime.", icon: Camera },
  { key: "facebook", label: "Facebook Messenger", description: "Customer conversations for an explicitly selected Facebook Page.", icon: MessageSquareText },
  { key: "tiktok", label: "TikTok", description: "Official Login Kit profile connection. Customer messaging is unavailable.", icon: Music2 },
];

const emptyWhatsAppForm: WhatsAppForm = {
  phone_number_id: "",
  whatsapp_business_account_id: "",
  meta_business_account_id: "",
  display_phone_number: "",
  graph_api_version: "v20.0",
};

const emptyTemplateForm: TemplateForm = { name: "", language: "en", category: "utility", status: "draft", body: "", provider_template_id: "", variables: "", sample_values: "", rejection_reason: "" };

export const setupSteps = ["setup_started", "credentials_added", "webhook_verified", "phone_identity_verified", "test_message_ready", "test_message_sent", "inbound_test_received", "connected", "healthy"];

export function absoluteWebhookURL(value: string) {
  if (/^https?:\/\//i.test(value)) return value;
  try {
    return `${new URL(API_BASE_URL).origin}${value}`;
  } catch {
    return value;
  }
}

export function badgeTone(status: string): "neutral" | "success" | "warning" | "danger" | "info" {
  if (["active", "connected", "healthy", "present", "processed", "verified", "delivered", "read", "sent"].includes(status)) return "success";
  if (["degraded", "connecting", "setup_required", "expiring", "pending", "test_message_ready", "test_message_sent", "inbound_test_received"].includes(status)) return "warning";
  if (["failed", "expired", "revoked", "requires_attention", "requires_reauthorization"].includes(status)) return "danger";
  if (["received", "sandbox"].includes(status)) return "info";
  return "neutral";
}

export function providerLabel(provider: string) {
  return providers.find((item) => item.key === provider)?.label || humanStatus(provider);
}

export function ReadinessItem({ complete, label }: { complete: boolean; label: string }) {
  return <div className={complete ? "whatsapp-check complete" : "whatsapp-check"}>{complete ? <Check size={16} aria-hidden="true" /> : <X size={16} aria-hidden="true" />}<span>{label}</span></div>;
}

export function useChannelWorkspace() {
  const { connectionId } = useParams();
  const navigate = useNavigate();
  const [tab, setTab] = useState("overview");
  const [wizard, setWizard] = useState(0);
  const { user } = useAuth();
  const canView = hasPermission(user.role, "channels.view");
  const canManage = hasPermission(user.role, "channels.manage");
  const [connections, setConnections] = useState<ChannelConnection[]>([]);
  const [providerCatalog, setProviderCatalog] = useState<ProviderCatalogEntry[]>([]);
  const [selectedID, setSelectedID] = useState(() => connectionId || oauthConnectionFromSession());
  const selectedRef = useRef(selectedID);
  selectedRef.current = selectedID;
  const listRequest = useRef(0);
  const detailRequest = useRef(0);
  const [detailError, setDetailError] = useState("");
  const [detail, setDetail] = useState<ConnectionDetail | null>(null);
  const [events, setEvents] = useState<ProviderEvent[]>([]);
  const [whatsApp, setWhatsApp] = useState<WhatsAppConfiguration | null>(null);
  const [whatsAppForm, setWhatsAppForm] = useState<WhatsAppForm>(emptyWhatsAppForm);
	const [whatsAppTemplates, setWhatsAppTemplates] = useState<WhatsAppTemplate[]>([]);
	const [whatsAppContacts, setWhatsAppContacts] = useState<WhatsAppContactState[]>([]);
	const [whatsAppMetrics, setWhatsAppMetrics] = useState<WhatsAppAdvancedMetrics | null>(null);
	const [policyBlocks, setPolicyBlocks] = useState<WhatsAppPolicyBlock[]>([]);
	const [templateOpen, setTemplateOpen] = useState(false);
	const [editingTemplateID, setEditingTemplateID] = useState("");
	const [templateForm, setTemplateForm] = useState<TemplateForm>(emptyTemplateForm);
  const [credentialMode, setCredentialMode] = useState<"encrypted" | "reference">("reference");
  const [credentialForm, setCredentialForm] = useState({ credential_type: "access_token", secret: "" });
  const [testForm, setTestForm] = useState({ recipient: "", message: "Zidi WhatsApp connection test. Reply to confirm inbound delivery.", template_id: "", template_variables: {} as Record<string, string> });
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [createForm, setCreateForm] = useState({ provider: "whatsapp", display_name: "WhatsApp", ownership_model: "merchant_managed", environment: "sandbox" });
  const [editForm, setEditForm] = useState({ display_name: "", ownership_model: "merchant_managed", environment: "sandbox" });
  const [saving, setSaving] = useState(false);
  const [connectingMeta, setConnectingMeta] = useState(false);
  const [message, setMessage] = useState("");
  const [success, setSuccess] = useState("");

  async function loadConnections(preferredID?: string) {
    if (!canView) { setLoading(false); return ""; }
    const request = ++listRequest.current;
    try {
      const response = await apiGet<ChannelConnection[]>("/channel-platform/connections");
      if (request !== listRequest.current) return "";
      setConnections(response.data);
      // Hub data is loaded by ChannelsHub; never open the first channel implicitly.
      const nextID = preferredID !== undefined ? preferredID : connectionId || oauthConnectionFromSession() || "";
      selectedRef.current = nextID;
      setSelectedID(nextID);
      setMessage("");
      return nextID;
    } catch (error) {
      if (request === listRequest.current) setMessage(error instanceof Error ? error.message : "Could not load channels");
      return "";
    } finally {
      if (request === listRequest.current) setLoading(false);
    }
  }

  function clearDetail() {
    setDetail(null); setEvents([]); setWhatsApp(null);
    setWhatsAppTemplates([]); setWhatsAppContacts([]); setWhatsAppMetrics(null); setPolicyBlocks([]);
    setWhatsAppForm(emptyWhatsAppForm);
    setCredentialForm({ credential_type: "access_token", secret: "" });
    setTemplateOpen(false);
    setTestForm({ recipient: "", message: "Zidi WhatsApp connection test. Reply to confirm inbound delivery.", template_id: "", template_variables: {} });
  }

  async function loadDetail(connectionID: string) {
    const request = ++detailRequest.current;
    const current = () => request === detailRequest.current && selectedRef.current === connectionID;
    if (!connectionID || !canView) {
      clearDetail(); setDetailLoading(false); setDetailError("");
      return;
    }
    if (detail?.connection.id !== connectionID) clearDetail();
    setDetailError("");
    setDetailLoading(true);
    try {
      const [detailResponse, eventsResponse] = await Promise.all([
        apiGet<ConnectionDetail>(`/channel-platform/connections/${connectionID}`),
        apiGet<ProviderEvent[]>(`/channel-platform/connections/${connectionID}/events?limit=50`),
      ]);
      if (!current()) return;
      setDetail(detailResponse.data);
      setEvents(eventsResponse.data);
      setEditForm({ display_name: detailResponse.data.connection.display_name, ownership_model: detailResponse.data.connection.ownership_model, environment: detailResponse.data.connection.environment });
      if (detailResponse.data.connection.provider === "whatsapp") {
		const [whatsAppResponse, templatesResponse, contactsResponse, metricsResponse, blocksResponse] = await Promise.all([
			apiGet<WhatsAppConfiguration>(`/channel-platform/connections/${connectionID}/whatsapp`),
			apiGet<WhatsAppTemplate[]>(`/channel-platform/connections/${connectionID}/whatsapp/templates`),
			apiGet<WhatsAppContactState[]>(`/channel-platform/connections/${connectionID}/whatsapp/contacts?limit=50`),
			apiGet<WhatsAppAdvancedMetrics>(`/channel-platform/connections/${connectionID}/whatsapp/advanced-metrics`),
			apiGet<WhatsAppPolicyBlock[]>(`/channel-platform/connections/${connectionID}/whatsapp/policy-blocks?limit=20`),
		]);
        if (!current()) return;
        const configuration = whatsAppResponse.data;
        setWhatsApp(configuration);
		setWhatsAppTemplates(templatesResponse.data);
		setWhatsAppContacts(contactsResponse.data);
		setWhatsAppMetrics(metricsResponse.data);
		setPolicyBlocks(blocksResponse.data);
        setWhatsAppForm({
          phone_number_id: configuration.phone_number_id || "",
          whatsapp_business_account_id: configuration.whatsapp_business_account_id || "",
          meta_business_account_id: configuration.meta_business_account_id || "",
          display_phone_number: configuration.display_phone_number || "",
          graph_api_version: configuration.graph_api_version || "v20.0",
        });
        setCredentialMode(configuration.encrypted_storage_enabled ? "encrypted" : "reference");
      } else {
        setWhatsApp(null);
		setWhatsAppTemplates([]);
		setWhatsAppContacts([]);
		setWhatsAppMetrics(null);
		setPolicyBlocks([]);
      }
      if (current()) setMessage("");
    } catch (error) {
      if (current()) {
        const reason = error instanceof Error ? error.message : "Could not load channel details";
        setDetailError(reason); setMessage(reason); clearDetail();
      }
    } finally {
      if (current()) setDetailLoading(false);
    }
  }

  useEffect(() => {
    if (canView && (connectionId || oauthConnectionFromSession())) void loadConnections();
    else setLoading(false);
    return () => { listRequest.current++; };
  }, [canView, user.organization_id, connectionId]);

  useEffect(() => {
    void loadDetail(selectedID);
    return () => { detailRequest.current++; };
  }, [selectedID, canView, user.organization_id]);

  useEffect(() => {
    const id = connectionId || oauthConnectionFromSession();
    if (!new URLSearchParams(window.location.search).has("setup")) setWizard(0);
    selectedRef.current = id;
    setSelectedID(id); setTab(oauthConnectionFromSession() || new URLSearchParams(window.location.search).has("setup") ? "settings" : "overview"); clearDetail();
  }, [connectionId, user.organization_id]);

  function beginCreate(provider: ProviderDefinition) {
    setCreateForm({ provider: provider.key, display_name: provider.label, ownership_model: "merchant_managed", environment: "sandbox" });
    setCreateOpen(true);
  }

  async function createConnection(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    try {
      const response = await apiPost<ChannelConnection>("/channel-platform/connections", { ...createForm, status: "setup_required" });
      setCreateOpen(false);
      setWizard(createForm.provider === "whatsapp" ? 1 : 0);
      navigate(`/settings/whatsapp/${response.data.id}?setup=1`);
      setSuccess(`Connection started. Complete ${providerLabel(createForm.provider)} authorization to continue.`);
      const nextID = await loadConnections(response.data.id);
      await loadDetail(nextID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not create channel setup");
    } finally {
      setSaving(false);
    }
  }

  async function saveConnection(event: FormEvent) {
    event.preventDefault();
    if (!selectedID) return;
    setSaving(true);
    try {
      await apiPatch<ChannelConnection>(`/channel-platform/connections/${selectedID}`, editForm);
      setSuccess("Channel settings updated.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not update channel settings");
    } finally {
      setSaving(false);
    }
  }

  async function archiveConnection() {
    if (!selectedID) return;
    try {
      await apiDelete<ChannelConnection>(`/channel-platform/connections/${selectedID}`);
      setSuccess("Channel connection archived. Existing conversation history remains available.");
      setDetail(null);
      setEvents([]);
      setSelectedID("");
      navigate("/settings/whatsapp");
      await loadConnections("");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not archive channel");
    }
  }

  async function saveWhatsApp(event: FormEvent) {
    event.preventDefault();
    if (!selectedID) return;
    setSaving(true);
    try {
      const payload: Record<string, string> = {
        phone_number_id: whatsAppForm.phone_number_id,
        whatsapp_business_account_id: whatsAppForm.whatsapp_business_account_id,
        meta_business_account_id: whatsAppForm.meta_business_account_id,
        display_phone_number: whatsAppForm.display_phone_number,
        graph_api_version: whatsAppForm.graph_api_version,
      };
      await apiPatch<WhatsAppConfiguration>(`/channel-platform/connections/${selectedID}/whatsapp`, payload);
      setSuccess("WhatsApp account and phone references saved.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not save WhatsApp configuration");
    } finally {
      setSaving(false);
    }
  }

  async function rotateWhatsAppCredential(event: FormEvent) {
    event.preventDefault();
    if (!selectedID || !credentialForm.secret.trim()) return;
    if (!window.confirm("Save this credential? Replacing a credential stops use of its previous value.")) return;
    setSaving(true);
    try {
      const payload: Record<string, string> = { credential_type: credentialForm.credential_type };
      payload[credentialMode === "encrypted" ? "secret_value" : "secret_reference"] = credentialForm.secret.trim();
      const response = await apiPost<{ rotated: boolean }>(`/channel-platform/connections/${selectedID}/whatsapp/credentials/rotate`, payload);
      setCredentialForm({ ...credentialForm, secret: "" });
      setSuccess(response.data.rotated ? "Credential rotated. The previous value is no longer available." : "Credential added securely.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not update the WhatsApp credential");
    } finally {
      setSaving(false);
    }
  }

  async function sendWhatsAppTest(event: FormEvent) {
    event.preventDefault();
    if (!window.confirm(`Send a WhatsApp test message to ${testForm.recipient}? The server enforces recipient and messaging restrictions.`)) return;
    if (!selectedID) return;
    setSaving(true);
    try {
		const payload = testForm.template_id ? { recipient: testForm.recipient, template_id: testForm.template_id, template_variables: testForm.template_variables } : { recipient: testForm.recipient, message: testForm.message };
      const response = await apiPost<{ recipient_display: string }>(`/channel-platform/connections/${selectedID}/whatsapp/test-message`, payload);
      setTestForm({ ...testForm, recipient: "" });
      setSuccess(`Test message accepted for ${response.data.recipient_display}. Reply from that number to validate inbound delivery.`);
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not send the WhatsApp test message");
    } finally {
      setSaving(false);
    }
  }

	function beginCreateTemplate() {
		setEditingTemplateID("");
		setTemplateForm(emptyTemplateForm);
		setTemplateOpen(true);
	}

	function beginEditTemplate(template: WhatsAppTemplate) {
		setEditingTemplateID(template.id);
		setTemplateForm({ name: template.name, language: template.language, category: template.category, status: template.status, body: template.body, provider_template_id: template.provider_template_id || "", variables: template.variable_schema.join(", "), sample_values: template.variable_schema.map((name) => `${name}=${template.sample_values[name] || ""}`).join(", "), rejection_reason: template.rejection_reason || "" });
		setTemplateOpen(true);
	}

	function templatePayload() {
		const variable_schema = templateForm.variables.split(",").map((item) => item.trim()).filter(Boolean);
		const sample_values: Record<string, string> = {};
		templateForm.sample_values.split(",").forEach((pair) => {
			const [name, ...value] = pair.split("=");
			if (name?.trim()) sample_values[name.trim()] = value.join("=").trim();
		});
		return { name: templateForm.name, language: templateForm.language, category: templateForm.category, status: templateForm.status, body: templateForm.body, provider_template_id: templateForm.provider_template_id, variable_schema, sample_values, rejection_reason: templateForm.rejection_reason, header_metadata: {}, footer_metadata: {}, buttons_metadata: [] };
	}

	async function saveTemplate(event: FormEvent) {
		event.preventDefault();
		if (!selectedID) return;
		setSaving(true);
		try {
			const path = `/channel-platform/connections/${selectedID}/whatsapp/templates${editingTemplateID ? `/${editingTemplateID}` : ""}`;
			if (editingTemplateID) await apiPatch<WhatsAppTemplate>(path, templatePayload());
			else await apiPost<WhatsAppTemplate>(path, templatePayload());
			setTemplateOpen(false);
			setSuccess(editingTemplateID ? "WhatsApp template record updated." : "Local WhatsApp template record created.");
			await loadDetail(selectedID);
		} catch (error) {
			setMessage(error instanceof Error ? error.message : "Could not save the WhatsApp template");
		} finally {
			setSaving(false);
		}
	}

	async function archiveTemplate(templateID: string) {
		if (!selectedID) return;
		setSaving(true);
		try {
			await apiDelete<WhatsAppTemplate>(`/channel-platform/connections/${selectedID}/whatsapp/templates/${templateID}`);
			setSuccess("WhatsApp template archived. It can no longer be used for policy-approved sends.");
			await loadDetail(selectedID);
		} catch (error) {
			setMessage(error instanceof Error ? error.message : "Could not archive the WhatsApp template");
		} finally {
			setSaving(false);
		}
	}

	function templatePreview() {
		const payload = templatePayload();
		let body = payload.body;
		payload.variable_schema.forEach((name, index) => {
			const value = payload.sample_values[name] || `{{${name}}}`;
			body = body.split(`{{${name}}}`).join(value).split(`{{${index + 1}}}`).join(value);
		});
		return body;
	}

  async function completeWhatsAppSetup() {
    if (!selectedID) return;
    setSaving(true);
    try {
      await apiPost(`/channel-platform/connections/${selectedID}/whatsapp/complete`, {});
      setSuccess("WhatsApp validation completed. Run the final health check.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not complete WhatsApp setup");
    } finally {
      setSaving(false);
    }
  }

  async function migrateLegacyCredentials() {
    if (!selectedID) return;
    setSaving(true);
    try {
      await apiPost(`/channel-platform/connections/${selectedID}/whatsapp/migrate-legacy`, {});
      setSuccess("Legacy WhatsApp settings migrated. The original values were retained for rollback.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not migrate legacy WhatsApp settings");
    } finally {
      setSaving(false);
    }
  }

  async function checkWhatsAppHealth() {
    if (!selectedID) return;
    setSaving(true);
    try {
      const response = await apiPost<WhatsAppHealth>(`/channel-platform/connections/${selectedID}/whatsapp/health-check`, {});
      setSuccess(response.data.status === "healthy" && response.data.setup_state === "healthy" ? "WhatsApp is connected and healthy." : response.data.status === "healthy" ? `Transport checks passed. Continue from ${humanStatus(response.data.setup_state)}.` : `WhatsApp requires attention: ${response.data.issues.map(humanStatus).join(", ")}.`);
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not evaluate WhatsApp health");
    } finally {
      setSaving(false);
    }
  }

  async function connectWithMeta() {
    if (!selectedID || !whatsApp?.embedded_signup_available) return;
    setConnectingMeta(true);
    setMessage("");
    try {
      const initiation = await apiPost<MetaSignupLaunch & { attempt_token: string }>(`/channel-platform/connections/${selectedID}/whatsapp/embedded-signup/initiate`, {});
      const result = await launchMetaEmbeddedSignup(initiation.data);
      const completion = await apiPost<EmbeddedSignupCompletion>(`/channel-platform/connections/${selectedID}/whatsapp/embedded-signup/complete`, {
        attempt_token: initiation.data.attempt_token,
        authorization_code: result.authorizationCode,
        whatsapp_business_account_id: result.whatsappBusinessAccountID,
        phone_number_id: result.phoneNumberID,
      });
      setSuccess(`${maskPhone(completion.data.display_phone_number, "WhatsApp")} is authorized. Complete the end-to-end message check before relying on the connection.`);
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not complete Meta signup");
    } finally {
      setConnectingMeta(false);
    }
  }

  async function requestZidiHelp() {
    if (!selectedID) return;
    setSaving(true);
    setMessage("");
    try {
      await apiPost<WhatsAppConfiguration>(`/channel-platform/connections/${selectedID}/whatsapp/assisted-setup`, { status: "requested" });
      setSuccess("Assisted setup requested. Zidi will track the Meta steps and tell you when your action is required.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not request assisted setup");
    } finally {
      setSaving(false);
    }
  }

  async function updateAssistedStatus(status: string) {
    if (!selectedID) return;
    setSaving(true);
    try {
      await apiPost<WhatsAppConfiguration>(`/channel-platform/connections/${selectedID}/whatsapp/assisted-setup`, { status, note: status === "awaiting_merchant_action" ? "Complete the Meta authorization step from this page." : "" });
      setSuccess("Assisted setup status updated.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not update assisted setup");
    } finally {
      setSaving(false);
    }
  }

  async function setConnectionStatus(status: "disconnected" | "connecting") {
    if (status === "disconnected" && !window.confirm("Disconnect this number and stop active processing?")) return;
    if (!selectedID) return;
    setSaving(true);
    try {
      await apiPatch<ChannelConnection>(`/channel-platform/connections/${selectedID}`, { status });
      setSuccess(status === "disconnected" ? "WhatsApp disconnected from active processing. Its history remains available." : "Connection reopened. Authorize with Meta again if required.");
      await loadConnections(selectedID);
      await loadDetail(selectedID);
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not update connection status");
    } finally {
      setSaving(false);
    }
  }

  async function copyWebhookURL() {
    try {
      await navigator.clipboard.writeText(absoluteWebhookURL(whatsApp?.webhook_callback_url || ""));
      setSuccess("Webhook URL copied.");
    } catch {
      setMessage("Could not copy the webhook URL.");
    }
  }

  const activeConnections = connections.filter((connection) => connection.status !== "archived");
  const selected = detail?.connection;
  const currentSetupStep = whatsApp ? setupSteps.indexOf(whatsApp.setup_state) : -1;
  const webhookURL = whatsApp ? absoluteWebhookURL(whatsApp.webhook_callback_url) : "";

  return { detailError, canView, navigate, tab, setTab, wizard, setWizard, canManage, connections, setConnections, providerCatalog, setProviderCatalog, selectedID, setSelectedID, detail, setDetail, events, setEvents, whatsApp, setWhatsApp, whatsAppForm, setWhatsAppForm, whatsAppTemplates, setWhatsAppTemplates, whatsAppContacts, setWhatsAppContacts, whatsAppMetrics, setWhatsAppMetrics, policyBlocks, setPolicyBlocks, templateOpen, setTemplateOpen, editingTemplateID, setEditingTemplateID, templateForm, setTemplateForm, credentialMode, setCredentialMode, credentialForm, setCredentialForm, testForm, setTestForm, loading, setLoading, detailLoading, setDetailLoading, createOpen, setCreateOpen, createForm, setCreateForm, editForm, setEditForm, saving, setSaving, connectingMeta, setConnectingMeta, message, setMessage, success, setSuccess, loadConnections, loadDetail, beginCreate, createConnection, saveConnection, archiveConnection, saveWhatsApp, rotateWhatsAppCredential, sendWhatsAppTest, beginCreateTemplate, beginEditTemplate, templatePayload, saveTemplate, archiveTemplate, templatePreview, completeWhatsAppSetup, migrateLegacyCredentials, checkWhatsAppHealth, connectWithMeta, requestZidiHelp, updateAssistedStatus, setConnectionStatus, copyWebhookURL, activeConnections, selected, currentSetupStep, webhookURL, user, connectionId };
}
