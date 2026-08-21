import { NavLink, Outlet, useLocation } from "react-router-dom";
import { useAuth } from "../auth";
import { roleLabel } from "../lib/format";

type NavGroup = {
  label: string;
  items: string[][];
  roles?: string[];
  collapsed?: boolean;
};

const groups: NavGroup[] = [
  {
    label: "Home",
    items: [
      ["Overview", "/"],
      ["Setup", "/setup"],
    ],
  },
  {
    label: "Commerce",
    items: [
      ["Orders", "/orders"],
      ["Catalogue", "/catalogue"],
      ["Inventory", "/inventory"],
      ["Customers", "/customers"],
      ["Stores", "/stores"],
    ],
  },
  {
    label: "Assistant",
    items: [
      ["Your assistant", "/assistant"],
      ["Conversations", "/conversations"],
      ["Knowledge", "/knowledge"],
    ],
  },
  {
    label: "Team",
    items: [["People", "/team"]],
  },
  {
    label: "Settings",
    items: [
      ["Business", "/settings/business"],
      ["Payments", "/settings/payments"],
      ["WhatsApp", "/settings/whatsapp"],
    ],
  },
  {
    label: "Advanced",
    collapsed: true,
    items: [
      ["Bot Builder", "/advanced/bot-builder"],
      ["Payment tools", "/advanced/payments"],
      ["Delivery lookup", "/advanced/delivery"],
      ["Import", "/advanced/import"],
      ["Audit logs", "/advanced/audit-logs"],
    ],
  },
  {
    label: "Platform",
    items: [["Organizations", "/platform/organizations"]],
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
  "/assistant": { eyebrow: "Assistant", title: "Your assistant" },
  "/conversations": { eyebrow: "Assistant", title: "Conversations" },
  "/knowledge": { eyebrow: "Assistant", title: "Knowledge" },
  "/team": { eyebrow: "Team", title: "People" },
  "/settings/business": { eyebrow: "Settings", title: "Business" },
  "/settings/payments": { eyebrow: "Settings", title: "Payments" },
  "/settings/whatsapp": { eyebrow: "Settings", title: "WhatsApp" },
  "/advanced/bot-builder": { eyebrow: "Advanced", title: "Bot Builder" },
  "/advanced/payments": { eyebrow: "Advanced", title: "Payment tools" },
  "/advanced/delivery": { eyebrow: "Advanced", title: "Delivery lookup" },
  "/advanced/import": { eyebrow: "Advanced", title: "Import" },
  "/advanced/audit-logs": { eyebrow: "Advanced", title: "Audit logs" },
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
  const visibleGroups = groups.filter((group) => !group.roles || group.roles.includes(user.role));
  const meta = pageMeta(location.pathname);

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">ZC</div>
          <div>
            <strong>ZidiCommerce</strong>
            <span>Merchant workspace</span>
          </div>
        </div>

        <nav className="nav-groups">
          {visibleGroups.map((group) => (
            <section key={group.label} className={group.collapsed ? "nav-advanced" : undefined}>
              <p>{group.label}</p>
              {group.items.map(([label, path]) => (
                <NavLink key={path} to={path} end={path === "/"}>
                  {label}
                </NavLink>
              ))}
            </section>
          ))}
        </nav>
      </aside>

      <main className="main-panel">
        <header className="topbar">
          <div>
            <span className="eyebrow">{meta.eyebrow}</span>
            <h1>{meta.title}</h1>
          </div>
          <div className="topbar-actions">
            <span className="muted">{roleLabel(user.role)}</span>
            <button type="button" onClick={logout}>
              Log out
            </button>
          </div>
        </header>
        <Outlet />
      </main>
    </div>
  );
}
