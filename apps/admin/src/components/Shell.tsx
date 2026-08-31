import { useEffect, useState } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
import { Bot, Boxes, Building2, ClipboardCheck, CreditCard, FileClock, Home, LifeBuoy, LogOut, Menu, MessageSquareText, PackageSearch, Settings, ShieldCheck, ShoppingBag, Sparkles, Store, Users, X, type LucideIcon } from "lucide-react";
import { apiGet } from "../api/client";
import { useAuth } from "../auth";
import { roleLabel } from "../lib/format";
import { hasPermission, type Permission } from "../lib/permissions";
import { ZidiCommerceLogo } from "./brand/ZidiCommerceLogo";
import { IconButton } from "./ui";

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
  "/team": { eyebrow: "Operations", title: "People" },
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
  if (titles[pathname]) return titles[pathname];
  const match = Object.keys(titles).find((path) => path !== "/" && pathname.startsWith(path));
  return match ? titles[match] : { eyebrow: "ZidiCommerce", title: "Workspace" };
}

export function Shell() {
  const { user, logout } = useAuth();
  const location = useLocation();
  const [fieldService, setFieldService] = useState(false);
  const [organizationName, setOrganizationName] = useState("");
  const [mobileOpen, setMobileOpen] = useState(false);
  const fieldPortalURL = import.meta.env.VITE_FIELD_PORTAL_URL ?? "";
  useEffect(() => {
    apiGet<Record<string, unknown>>("/organizations/current").then((response) => {
      const metadata = String(response.data.metadata ?? "").toLowerCase();
      setFieldService(metadata.includes("field_service") || metadata.includes("handyman"));
      setOrganizationName(String(response.data.name ?? ""));
    }).catch(() => undefined);
  }, []);
  useEffect(() => setMobileOpen(false), [location.pathname]);
  const visibleGroups = groups
    .filter((group) => !group.roles || group.roles.includes(user.role))
    .filter((group) => !fieldService || group.label !== "Commerce")
    .map((group) => ({ ...group, items: group.items.filter((item) => (!item.roles || item.roles.includes(user.role)) && (!item.permission || hasPermission(user.role, item.permission))) }))
    .filter((group) => group.items.length > 0);
  if (fieldService && fieldPortalURL) {
    visibleGroups.splice(1, 0, { label: "Field service", items: [{ label: "Operations", path: `${fieldPortalURL.replace(/\/$/, "")}/owner`, external: true, icon: ShieldCheck }] });
  }
  const meta = pageMeta(location.pathname);

  return (
    <div className="app-shell">
      {mobileOpen ? <button className="sidebar-scrim" type="button" aria-label="Close navigation" onClick={() => setMobileOpen(false)} /> : null}
      <aside className={mobileOpen ? "sidebar open" : "sidebar"} aria-label="Primary navigation">
        <div className="brand">
          <ZidiCommerceLogo tone="light" subtitle={organizationName || "Merchant workspace"} />
          <IconButton className="sidebar-close" label="Close navigation" icon={X} onClick={() => setMobileOpen(false)} />
        </div>

        <nav className="nav-groups">
          {visibleGroups.map((group) => (
            <section key={group.label} className={group.collapsed ? "nav-advanced" : undefined}>
              <p>{group.label}</p>
              {group.items.map(({ label, path, external, icon: Icon }) => external ? (
                <a key={path} href={path}><Icon size={17} aria-hidden="true" /><span>{label}</span></a>
              ) : (
                <NavLink key={path} to={path} end={path === "/"}><Icon size={17} aria-hidden="true" /><span>{label}</span></NavLink>
              ))}
            </section>
          ))}
        </nav>
      </aside>

      <main className="main-panel">
        <header className="topbar">
          <div className="topbar-context">
            <IconButton className="mobile-menu" label="Open navigation" icon={Menu} onClick={() => setMobileOpen(true)} />
            <div>
            <span className="eyebrow">{meta.eyebrow}</span>
              <strong>{meta.title}</strong>
            </div>
          </div>
          <div className="topbar-actions">
            <div className="user-summary"><span>{String(user.email || roleLabel(user.role)).slice(0, 1).toUpperCase()}</span><div><strong>{user.email || "Signed in"}</strong><small>{roleLabel(user.role)}</small></div></div>
            <IconButton label="Log out" icon={LogOut} onClick={logout} />
          </div>
        </header>
        <Outlet />
      </main>
    </div>
  );
}
