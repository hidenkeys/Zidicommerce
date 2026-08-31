import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet } from "../api/client";
import { useAuth } from "../auth";
import { Badge, Card, Flash, LoadingState, Page, SectionHeader, SetupItem } from "../components/ui";
import { setupHref } from "../lib/format";
import { isOrganizationAdmin } from "../lib/permissions";
import { merchantReadinessItems, readinessSections, type SetupStatus } from "../lib/readiness";

const labels: Record<string, { title: string; copy: string }> = {
  organization: { title: "Business profile", copy: "Confirm the identity, currency, and timezone used on orders." },
  stores: { title: "Store", copy: "Add the location customers order from." },
  catalogue: { title: "Catalogue", copy: "Add products customers can buy." },
  inventory: { title: "Inventory", copy: "Set how many of each product you have." },
  payments: { title: "Payments", copy: "Enable and test how customers will pay." },
  whatsapp: { title: "Customer channel", copy: "Optional for now. External channel connections are deferred." },
  bot: { title: "Assistant", copy: "Review, test, and publish the customer assistant." },
  faqs: { title: "Business knowledge", copy: "Publish policies and answers the assistant may use." },
  team: { title: "Team", copy: "Optional for a solo merchant. Invite staff and assign their stores." },
};

export function SetupPage() {
  const { user } = useAuth();
  const canManageSetup = isOrganizationAdmin(user.role);
  const [setupStatus, setSetupStatus] = useState<SetupStatus | null>(null);
  const [message, setMessage] = useState("");
  const [loading, setLoading] = useState(canManageSetup);

  useEffect(() => {
    if (!canManageSetup) return;
    async function load() {
      try {
        const response = await apiGet<SetupStatus>("/bot-setup/status");
        setSetupStatus(response.data);
      } catch (error) {
        setMessage(error instanceof Error ? error.message : "Could not load setup");
      } finally {
        setLoading(false);
      }
    }
    void load();
  }, [canManageSetup]);

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
      help="Readiness is calculated from current organization data. External customer channels remain optional and deferred in this phase."
    >
      <Flash message={message} />
      {loading ? <LoadingState label="Checking business readiness" /> : null}
      <Card>
        <div className="readiness-summary"><div><span className="section-kicker">Required setup</span><strong>{requiredDone} of {requiredItems.length || 7} ready</strong></div><Badge tone={ready ? "success" : "warning"}>{ready ? "Operationally ready" : `${requiredItems.length - requiredDone} remaining`}</Badge></div>
        <div className="progress-bar"><span style={{ width: `${requiredItems.length ? (requiredDone / requiredItems.length) * 100 : 0}%` }} /></div>
      </Card>
      {readinessSections.map((section) => {
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
      })}
    </Page>
  );
}
