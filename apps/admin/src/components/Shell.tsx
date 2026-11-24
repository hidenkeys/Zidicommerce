import { useEffect, useRef, useState } from "react";
import { Link, NavLink, Outlet, useLocation } from "react-router-dom";
import { PanelLeft, Search, Bell, Bot, Boxes, Building2, ClipboardCheck, CreditCard, FileClock, Home, LifeBuoy, LogOut, Menu, MessageSquareText, PackageSearch, Settings, ShieldCheck, ShoppingBag, Sparkles, Store, Users, X, type LucideIcon } from "lucide-react";
import { apiGet } from "../api/client";
import { hasOrganization, useAuth } from "../auth";
import { roleLabel } from "../lib/format";
import { hasPermission, type Permission } from "../lib/permissions";
import { ZidiCommerceLogo } from "./brand/ZidiCommerceLogo";
import { routeAliases } from "../lib/route-aliases";
import { IconButton, Modal } from "./ui";
import { ProductTour } from "./onboarding/ProductTour";
import { availableTourSteps, tourSteps, tourStorageKey, shouldStartTour, persistTour, type TourFlag } from "../lib/onboarding";

type NavGroup = {
  label: string;
  items: NavItem[];
  roles?: string[];
  collapsed?: boolean;
};

type NavItem = {
  label: string;
  path: string;
  permission?: Permission;
  roles?: string[];
  external?: boolean;
  icon: LucideIcon;
};

const groups: NavGroup[] = [
  {
    label: "Home",
    items: [
      { label: "Overview", path: "/", permission: "organization.view", icon: Home },
      { label: "Setup", path: "/setup", permission: "organization.view", roles: ["merchant_admin", "platform_admin"], icon: ClipboardCheck },
    ],
  },
  {
    label: "Commerce",
    items: [
      { label: "Orders", path: "/orders", permission: "orders.view", icon: ShoppingBag },
      { label: "Catalogue", path: "/catalogue", permission: "catalogue.view", roles: ["merchant_admin", "platform_admin", "store_manager", "viewer"], icon: PackageSearch },
      { label: "Inventory", path: "/inventory", permission: "inventory.view", roles: ["merchant_admin", "platform_admin", "store_manager", "store_staff", "viewer"], icon: Boxes },
      { label: "Stores", path: "/stores", permission: "stores.view", roles: ["merchant_admin", "platform_admin", "store_manager", "store_staff", "viewer"], icon: Store },
      { label: "Customers", path: "/customers", permission: "customers.view", roles: ["merchant_admin", "platform_admin", "store_manager", "support_agent", "viewer"], icon: Users },
      { label: "Payments", path: "/settings/payments", permission: "payments.view", roles: ["merchant_admin", "platform_admin", "viewer"], icon: CreditCard },
    ],
  },
  {
    label: "Engagement",
    items: [
      { label: "Assistant", path: "/assistant", permission: "bot.view", roles: ["merchant_admin", "platform_admin", "store_manager", "viewer"], icon: Bot },
      { label: "Flow Studio", path: "/journeys", permission: "bot.view", icon: ClipboardCheck },
      { label: "AI test workspace", path: "/assistant/ai-chat", permission: "ai.use", roles: ["merchant_admin", "platform_admin"], icon: Sparkles },
      { label: "Conversations", path: "/conversations", permission: "conversations.view", icon: MessageSquareText },
      { label: "Business knowledge", path: "/knowledge", permission: "knowledge.view", roles: ["merchant_admin", "platform_admin", "store_manager", "support_agent", "viewer"], icon: LifeBuoy },
    ],
  },
  {
    label: "Operations",
    items: [
      { label: "Team", path: "/team", permission: "staff.view", roles: ["merchant_admin", "platform_admin", "store_manager", "viewer"], icon: Users },
      { label: "Audit log", path: "/advanced/audit-logs", permission: "audit.view", icon: FileClock },
    ],
  },
  {
    label: "Organization",
    items: [
      { label: "Business settings", path: "/settings/business", permission: "settings.view", roles: ["merchant_admin", "platform_admin", "store_manager", "viewer"], icon: Settings },
      { label: "Channels", path: "/settings/whatsapp", permission: "channels.view", roles: ["merchant_admin", "platform_admin", "viewer"], icon: MessageSquareText },
    ],
  },
  {
    label: "Advanced",
    collapsed: true,
    items: [
      { label: "Bot Builder", path: "/advanced/bot-builder", permission: "bot.manage", icon: Bot },
      { label: "Payment tools", path: "/advanced/payments", permission: "payments.manage", icon: CreditCard },
      { label: "Delivery lookup", path: "/advanced/delivery", permission: "stores.update", icon: Store },
      { label: "Import data", path: "/advanced/import", permission: "catalogue.manage", icon: PackageSearch },
    ],
  },
  {
    label: "Platform",
    items: [{ label: "Organizations", path: "/platform/organizations", icon: Building2 }],
    roles: ["platform_admin"],
  },
];

const titles: Record<string, { eyebrow: string; title: string }> = {
  "/": { eyebrow: "Home", title: "Overview" },
  "/setup": { eyebrow: "Home", title: "Setup" },
  "/orders": { eyebrow: "Commerce", title: "Orders" },
  "/catalogue": { eyebrow: "Commerce", title: "Catalogue" },
  "/inventory": { eyebrow: "Commerce", title: "Inventory" },
  "/customers": { eyebrow: "Commerce", title: "Customers" },
  "/stores": { eyebrow: "Commerce", title: "Stores" },
  "/assistant": { eyebrow: "Engagement", title: "Your assistant" },
  "/assistant/ai-chat": { eyebrow: "Engagement", title: "AI Chat" },
  "/conversations": { eyebrow: "Engagement", title: "Conversations" },
  "/knowledge": { eyebrow: "Engagement", title: "Knowledge" },
  "/journeys": { eyebrow: "Engagement", title: "Flow Studio" },
  "/team": { eyebrow: "Operations", title: "Team" },
  "/settings/business": { eyebrow: "Organization", title: "Business" },
  "/settings/payments": { eyebrow: "Commerce", title: "Payments" },
  "/settings/whatsapp": { eyebrow: "Organization", title: "Channels" },
  "/advanced/bot-builder": { eyebrow: "Advanced", title: "Bot Builder" },
  "/advanced/payments": { eyebrow: "Advanced", title: "Payment tools" },
  "/advanced/delivery": { eyebrow: "Advanced", title: "Delivery lookup" },
  "/advanced/import": { eyebrow: "Advanced", title: "Import" },
  "/advanced/audit-logs": { eyebrow: "Operations", title: "Audit logs" },
  "/platform/organizations": { eyebrow: "Platform", title: "Organizations" },
};

function pageMeta(pathname: string) {
  pathname = routeAliases[pathname] || pathname;
  if (titles[pathname]) return titles[pathname];
  const match = Object.keys(titles).find((path) => path !== "/" && pathname.startsWith(path));
  return match ? titles[match] : { eyebrow: "ZidiCommerce", title: "Workspace" };
}

export function Shell() {
  const { user, logout } = useAuth();
  const organizationReady = hasOrganization(user);
  const location = useLocation();
  const activePath = routeAliases[location.pathname] || location.pathname;
  const [tourOpen, setTourOpen] = useState(false);
  const [orgLoaded, setOrgLoaded] = useState("");
  const tourSession = useRef("");
  const tourKey = tourStorageKey(user.id, user.organization_id);
  useEffect(() => {
    if (!organizationReady || orgLoaded !== user.organization_id || tourSession.current === tourKey) return;
    tourSession.current = tourKey;
    let start = true;
    try { start = shouldStartTour(window.localStorage, tourKey); } catch { /* Storage unavailable. */ }
    setTourOpen(start);
    if (start) { try { persistTour(window.localStorage, tourKey, "started"); } catch { /* Storage unavailable. */ } }
  }, [organizationReady, orgLoaded, tourKey]);
  function closeTour(flag: TourFlag) { setTourOpen(false); try { persistTour(window.localStorage, tourKey, flag); } catch { /* Storage unavailable. */ } }
  const [fieldService, setFieldService] = useState(false);
  const [organizationName, setOrganizationName] = useState("");
  const [mobileOpen, setMobileOpen] = useState(false);
  const [compact,setCompact] = useState(() => localStorage.getItem("zidi.sidebar.compact") === "yes");
  const [panel,setPanel] = useState<"find"|"notifications"|"account">();
  const [query,setQuery] = useState("");
  const [handoffs,setHandoffs] = useState<number>();
  const [inboxError,setInboxError] = useState("");
  const menuButton = useRef<HTMLButtonElement>(null);
  const closeNav = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!mobileOpen) return;
    closeNav.current?.focus();
    const close = (event: KeyboardEvent) => { if(event.key==="Escape"){setMobileOpen(false);menuButton.current?.focus();} };
    window.addEventListener("keydown",close);
    return () => window.removeEventListener("keydown",close);
  },[mobileOpen]);
  useEffect(() => {
    if(panel !== "notifications") return;
    let current=true;
    setHandoffs(undefined);setInboxError("");
    apiGet<{conversation_status:string}[]>("/runtime/conversations").then(r=>{
      if(current)setHandoffs(r.data.filter(c=>["human_requested","human_assigned"].includes(c.conversation_status)).length);
    }).catch(()=>{if(current)setInboxError("Conversation activity could not be loaded.");});
    return ()=>{current=false;};
  },[panel,user.organization_id]);
  const fieldPortalURL = import.meta.env.VITE_FIELD_PORTAL_URL ?? "";
  useEffect(() => {
    apiGet<Record<string, unknown>>("/organizations/current").then((response) => {
      const metadata = String(response.data.metadata ?? "").toLowerCase();
      setFieldService(metadata.includes("field_service") || metadata.includes("handyman"));
      setOrganizationName(String(response.data.name ?? ""));
    }).catch(() => undefined).finally(() => setOrgLoaded(user.organization_id));
  }, [user.organization_id]);
  useEffect(() => setMobileOpen(false), [location.pathname]);
  const visibleGroups = (organizationReady ? groups : [])
    .filter((group) => !group.roles || group.roles.includes(user.role))
    .filter((group) => !fieldService || group.label !== "Commerce")
    .map((group) => ({ ...group, items: group.items.filter((item) => (!item.roles || item.roles.includes(user.role)) && (!item.permission || hasPermission(user.role, item.permission))) }))
    .filter((group) => group.items.length > 0);
  if (fieldService && fieldPortalURL) {
    visibleGroups.splice(1, 0, { label: "Field service", items: [{ label: "Operations", path: `${fieldPortalURL.replace(/\/$/, "")}/owner`, external: true, icon: ShieldCheck }] });
  }
  const steps = availableTourSteps(visibleGroups.flatMap(group => group.items.map(item => item.path)));
  const meta = pageMeta(location.pathname);

  return (
    <div className={compact ? "app-shell compact-sidebar" : "app-shell"}>
      {mobileOpen ? <button className="sidebar-scrim" type="button" aria-label="Close navigation" onClick={() => setMobileOpen(false)} /> : null}
      <aside className={mobileOpen ? "sidebar open" : "sidebar"} aria-label="Primary navigation">
        <div className="brand">
          <ZidiCommerceLogo tone="dark" subtitle={organizationName || (organizationReady ? "Merchant workspace" : "Workspace setup")} />
          <IconButton ref={closeNav} className="sidebar-close" label="Close navigation" icon={X} onClick={() => setMobileOpen(false)} />
        </div>

        <nav className="nav-groups">
          {visibleGroups.map((group) => (
            <section key={group.label} className={group.collapsed ? "nav-advanced" : undefined}>
              <p>{group.label}</p>
              {group.items.map(({ label, path, external, icon: Icon }) => external ? (
                <a key={path} href={path} title={label}><Icon size={17} aria-hidden="true" /><span>{label}</span></a>
              ) : (
                <Link data-tour={tourSteps.find(step => step.path === path)?.id} key={path} title={label} to={path} className={activePath === path ? "active" : undefined} aria-current={activePath === path ? "page" : undefined}><Icon size={17} aria-hidden="true" /><span>{label}</span></Link>
              ))}
            </section>
          ))}
        </nav>
      </aside>

      <main className="main-panel">
        <header className="topbar">
          <div className="topbar-context">
            <IconButton className="desktop-nav-toggle" label={compact ? "Expand navigation" : "Collapse navigation"} icon={PanelLeft} onClick={() => {localStorage.setItem("zidi.sidebar.compact",compact ? "no":"yes");setCompact(!compact);}} /><IconButton ref={menuButton} className="mobile-menu" label="Open navigation" icon={Menu} onClick={() => setMobileOpen(true)} />
            <div>
            <span className="topbar-group">{meta.eyebrow}</span>
              <strong>{meta.title}</strong>
            </div>
          </div>
          <div className="topbar-actions">
            <IconButton label="Find a page" icon={Search} onClick={()=>{setQuery("");setPanel("find");}} />
            {hasPermission(user.role,"conversations.view") && <IconButton label="Conversation activity" icon={Bell} onClick={()=>setPanel("notifications")} />}
            <button className="account-button" onClick={()=>setPanel("account")} aria-label="Open account menu">{String(user.email || roleLabel(user.role)).slice(0,1).toUpperCase()}</button>
          </div>
        </header>
        <div data-tour-region={tourSteps.find(step => step.path === activePath)?.id}><Outlet /></div>
        <Modal open={Boolean(panel)} title={panel==="find" ? "Find a page" : panel==="notifications" ? "Conversation activity" : "Your account"} onClose={()=>setPanel(undefined)}>
          {panel==="find" && <><label>Search navigation<input type="search" value={query} onChange={e=>setQuery(e.target.value)} /></label><div className="shell-results">{visibleGroups.flatMap(group=>group.items.filter(item=>!item.external && (group.label+" "+item.label).toLowerCase().includes(query.trim().toLowerCase())).map(item=><NavLink key={item.path} to={item.path} onClick={()=>setPanel(undefined)}><item.icon size={16}/>{item.label}<small>{group.label}</small></NavLink>))}{!visibleGroups.some(group=>group.items.some(item=>!item.external && (group.label+" "+item.label).toLowerCase().includes(query.trim().toLowerCase()))) && <p>No matching pages.</p>}</div></>}
          {panel==="notifications" && <><p role="status">{inboxError || (handoffs===undefined ? "Loading conversations…" : handoffs===0 ? "No conversations are waiting for your team." : handoffs+" conversations need your team.")}</p><NavLink to="/conversations" onClick={()=>setPanel(undefined)}>Open conversations</NavLink></>}
          {panel==="account" && <><strong>{user.email}</strong><p>{roleLabel(user.role)} · {organizationName}</p><button type="button" onClick={()=>{setPanel(undefined);setTourOpen(true);}}>Take the tour</button><button onClick={logout}><LogOut size={16}/> Sign out</button></>}
        </Modal>
        {tourOpen && steps.length > 0 && <ProductTour key={tourKey} steps={steps} onClose={closeTour} />}
      </main>
    </div>
  );
}
