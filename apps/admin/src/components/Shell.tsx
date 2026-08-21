import { NavLink, Outlet } from "react-router-dom";
import { useAuth } from "../auth";

type NavGroup = {
  label: string;
  items: string[][];
  roles?: string[];
};

const groups: NavGroup[] = [
  {
    label: "Overview",
    items: [["Dashboard", "/"]],
  },
  {
    label: "Commerce",
    items: [
      ["Stores", "/commerce/stores"],
      ["Catalogue", "/commerce/catalogue"],
      ["Inventory", "/commerce/inventory"],
      ["Orders", "/commerce/orders"],
      ["Customers", "/commerce/customers"],
    ],
  },
  {
    label: "Organization",
    items: [
      ["Business", "/organization/business"],
      ["Team", "/organization/team"],
      ["Stores & Access", "/organization/access"],
      ["Audit logs", "/organization/audit-logs"],
    ],
  },
  {
    label: "Configuration",
    items: [
      ["Payments", "/configuration/payments"],
      ["Fulfilment", "/configuration/fulfilment"],
      ["Channels", "/configuration/channels"],
    ],
  },
  {
    label: "Automation",
    items: [
      ["Bots", "/automation/bots"],
      ["Bot Versions", "/automation/versions"],
      ["Conversations", "/automation/conversations"],
      ["Support Handoffs", "/automation/support-handoffs"],
    ],
  },
  {
    label: "Platform",
    items: [["Organizations", "/platform/organizations"]],
    roles: ["platform_admin"],
  },
  {
    label: "System",
    items: [
      ["Readiness", "/settings/readiness"],
      ["Merchant Import", "/settings/import"],
      ["Settings", "/settings"],
    ],
  },
];

export function Shell() {
  const { user, logout } = useAuth();
  const visibleGroups = groups.filter((group) => !group.roles || group.roles.includes(user.role));

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <div className="brand">
          <div className="brand-mark">ZC</div>
          <div>
            <strong>ZidiCommerce</strong>
            <span>Merchant operations</span>
          </div>
        </div>

        <nav className="nav-groups">
          {visibleGroups.map((group) => (
            <section key={group.label}>
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
            <span className="eyebrow">Phase 9 Merchant Pilot</span>
            <h1>ZidiCommerce Admin</h1>
          </div>
          <div className="topbar-actions">
            <span className="muted">{user.role}</span>
            <button type="button" onClick={logout}>Log out</button>
          </div>
        </header>
        <Outlet />
      </main>
    </div>
  );
}
