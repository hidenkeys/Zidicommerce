import { FormEvent, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet, apiPost } from "../api/client";
import { hasOrganization, useAuth } from "../auth";
import { Badge, Card, Flash, LoadingState, Page, SectionHeader, SetupItem } from "../components/ui";
import { setupHref, slugify } from "../lib/format";
import { isOrganizationAdmin } from "../lib/permissions";
import { merchantReadinessItems, readinessSections, type SetupStatus } from "../lib/readiness";

const labels: Record<string, { title: string; copy: string }> = {
  organization: { title: "Business profile", copy: "Confirm the identity, currency, and timezone used on orders." },
  stores: { title: "Store", copy: "Add the location customers order from." },
  catalogue: { title: "Catalogue", copy: "Add products customers can buy." },
  inventory: { title: "Inventory", copy: "Set how many of each product you have." },
  payments: { title: "Payments", copy: "Enable and test how customers will pay." },
  whatsapp: { title: "Customer channel", copy: "Connect WhatsApp when you are ready to receive customer messages." },
  bot: { title: "Assistant", copy: "Review, test, and publish the customer assistant." },
  faqs: { title: "Business knowledge", copy: "Publish policies and answers the assistant may use." },
  team: { title: "Team", copy: "Invite staff and assign their stores." },
};

export function SetupPage() {
  const { user, activateSession } = useAuth();
  const organizationCreated = hasOrganization(user);
  const canManageSetup = isOrganizationAdmin(user.role);
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [message, setMessage] = useState("");
  const [messageTone, setMessageTone] = useState<"error" | "success">("error");
  const [loading, setLoading] = useState(canManageSetup && organizationCreated);
  const [creating, setCreating] = useState(false);
  const [retry, setRetry] = useState(0);
  const [slugEdited, setSlugEdited] = useState(false);
  const [organization, setOrganization] = useState({
    name: "",
    slug: "",
    country: "NG",
    currency: "NGN",
    timezone: "Africa/Lagos",
    contact_name: "",
    contact_email: "",
    contact_phone: "",
  });

  useEffect(() => {
    if (!canManageSetup || !organizationCreated) return;
    async function load() {
      setLoading(true);
      setMessage("");
      try {
        const response = await apiGet<SetupStatus>("/bot-setup/status");
        setSetupStatus(response.data);
      } catch (error) {
        setMessageTone("error");
        setMessage(error instanceof Error ? error.message : "Could not load setup");
      } finally {
        setLoading(false);
      }
    }
    void load();
  }, [canManageSetup, organizationCreated, retry]);

  async function createOrganization(event: FormEvent) {
    event.preventDefault();
    setMessage("");
    setMessageTone("error");
    setCreating(true);
    try {
      const response = await apiPost<{ access_token: string }>("/onboarding/organization", organization);
      await activateSession(response.data.access_token);
      setMessageTone("success");
      setMessage("Business workspace created. Continue with the required setup steps.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Could not create the business workspace");
    } finally {
      setCreating(false);
    }
  }

  if (!organizationCreated) {
    return (
      <Page title="Set up your workspace" description="Start with your business details. You can add stores and products next.">
        <Flash message={message} tone={messageTone} />
        <Card className="workspace-onboarding">
          <form className="workspace-onboarding-form" onSubmit={createOrganization}>
            <SectionHeader title="Business identity" />
            <label>
              Business name
              <input
                autoFocus
                required
                value={organization.name}
                onChange={(event) => {
                  const name = event.target.value;
                  setOrganization((current) => ({ ...current, name, slug: slugEdited ? current.slug : slugify(name) }));
                }}
              />
            </label>
            <label>
              Workspace address
              <input
                required
                pattern="[a-z0-9]+(?:-[a-z0-9]+)*"
                aria-describedby="workspace-address-help"
                value={organization.slug}
                onChange={(event) => {
                  setSlugEdited(true);
                  setOrganization((current) => ({ ...current, slug: slugify(event.target.value) }));
                }}
              />
            </label>
            <small id="workspace-address-help">Use lowercase letters, numbers and hyphens.</small>
            <SectionHeader title="Location and currency" description="Used for prices, order times and business operations." />
            <div className="workspace-fields">
              <label>
                Country code
                <input required maxLength={2} value={organization.country} onChange={(event) => setOrganization((current) => ({ ...current, country: event.target.value.toUpperCase() }))} />
              </label>
              <label>
                Currency
                <input required maxLength={3} value={organization.currency} onChange={(event) => setOrganization((current) => ({ ...current, currency: event.target.value.toUpperCase() }))} />
              </label>
              <label>
                Timezone
                <input required value={organization.timezone} onChange={(event) => setOrganization((current) => ({ ...current, timezone: event.target.value }))} />
              </label>
            </div>
            <details className="workspace-contact-details">
              <summary>Contact details <span className="muted">(optional)</span></summary>
              <div className="workspace-fields">
              <label>
                Contact name
                <input autoComplete="name" value={organization.contact_name} onChange={(event) => setOrganization((current) => ({ ...current, contact_name: event.target.value }))} />
              </label>
              <label>
                Contact email
                <input type="email" autoComplete="email" value={organization.contact_email} onChange={(event) => setOrganization((current) => ({ ...current, contact_email: event.target.value }))} />
              </label>
              <label>
                Contact phone
                <input type="tel" autoComplete="tel" value={organization.contact_phone} onChange={(event) => setOrganization((current) => ({ ...current, contact_phone: event.target.value }))} />
              </label>
              </div>
            </details>
            <button type="submit" disabled={creating || !organization.name || !organization.slug}>
              {creating ? "Creating workspace..." : "Create workspace"}
            </button>
          </form>
        </Card>
      </Page>
    );
  }

  if (!canManageSetup) {
    return (
      <Page title="Business setup" description="Organization setup is managed by an administrator.">
        <Card>
          <h3>Your workspace is ready for your role</h3>
          <p>You can continue with the operational areas assigned to you. Business configuration and go-live readiness remain with an organization administrator.</p>
          <Link className="button primary" to="/">Return to overview</Link>
        </Card>
      </Page>
    );
  }

  const items = merchantReadinessItems(setupStatus);
  const requiredItems = items.filter((item) => item.required);
  const requiredDone = requiredItems.filter((item) => item.complete).length;
  const ready = requiredItems.length > 0 && requiredDone === requiredItems.length;

  return (
    <Page
      title={ready ? "Ready for daily operations" : "Getting started"}
      description={ready ? "The required business, selling, and customer-service foundations are in place." : "Complete the required steps, then review the optional team and channel preparation."}
    >
      <Flash message={message} tone={messageTone} />
      {loading ? <LoadingState label="Checking business readiness" /> : null}
      {!loading && !setupStatus && message ? <div className="page-actions"><button type="button" onClick={() => setRetry((value) => value + 1)}>Try again</button></div> : null}
      {!loading && setupStatus ? <Card>
        <div className="readiness-summary"><div><span className="section-kicker">Required setup</span><strong>{requiredDone} of {requiredItems.length || 7} ready</strong></div><Badge tone={ready ? "success" : "warning"}>{ready ? "Operationally ready" : `${requiredItems.length - requiredDone} remaining`}</Badge></div>
        <div className="progress-bar" role="progressbar" aria-label="Required setup completed" aria-valuemin={0} aria-valuemax={requiredItems.length} aria-valuenow={requiredDone}><span style={{ width: `${requiredItems.length ? (requiredDone / requiredItems.length) * 100 : 0}%` }} /></div>
      </Card> : null}
      <div data-tour-region="setup-checklist" className="readiness-grid">{!loading && setupStatus ? readinessSections.map((section) => {
        const sectionItems = items.filter((item) => item.group === section.key);
        if (sectionItems.length === 0) return null;
        return (
          <section key={section.key} className="readiness-section">
            <SectionHeader title={section.title} description={section.description} />
            <Card>
              <div className="setup-list">
                {sectionItems.map((item) => {
                  const copy = labels[item.key];
                  return <SetupItem key={item.key} complete={item.complete} label={copy?.title ?? item.label} to={setupHref(item.key)} description={item.complete ? (item.required ? "Ready" : "Configured") : `${item.required ? "Required" : "Optional"}. ${copy?.copy ?? item.description}`} />;
                })}
              </div>
            </Card>
          </section>
        );
      }) : null}</div>
    </Page>
  );
}
