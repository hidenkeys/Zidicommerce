import { NavLink, Outlet } from "react-router-dom";
import { useAuth } from "../auth";

type NavGroup = {
  label: string;
  items: string[][];
  roles?: string[];
};

const groups: NavGroup[] = [
  {
    label: "Home",
    items: [["Overview", "/"]],
  },
  {
    label: "Sell",
    items: [
      ["Orders", "/sell/orders"],
      ["Catalogue", "/sell/catalogue"],
      ["Inventory", "/sell/inventory"],
      ["Customers", "/sell/customers"],
    ],
  },
  {
    label: "Automation",
    items: [
      ["My Bot", "/automation/bot"],
      ["Conversations", "/automation/conversations"],
      ["Knowledge", "/automation/knowledge"],
    ],
  },
  {
    label: "Business",
    items: [
      ["Stores", "/business/stores"],
      ["Team", "/business/team"],
      ["Payments", "/business/payments"],
      ["Delivery", "/business/delivery"],
    ],
  },
  {
    label: "Settings",
    items: [
      ["Business", "/settings/business"],
      ["WhatsApp", "/settings/whatsapp"],
      ["Integrations", "/settings/integrations"],
    ],
  },
  {
    label: "Advanced",
    items: [
      ["Bot Builder", "/advanced/bot-builder"],
      ["Readiness", "/advanced/readiness"],
      ["Import", "/advanced/import"],
      ["Audit logs", "/advanced/audit-logs"],
      ["Access", "/advanced/access"],
    ],
  },
  {
    label: "Platform",
    items: [["Organizations", "/platform/organizations"]],
    roles: ["platform_admin"],
  },
];

function roleLabel(role: string) {
  switch (role) {
    case "merchant_admin":
      return "Owner";
    case "store_manager":
      return "Manager";
    case "store_staff":
      return "Staff";
    case "support_agent":
      return "Support";
    case "platform_admin":
      return "Platform";
    default:
      return role;
  }
}

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
            <span>Your business, in one place</span>
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
            <span className="eyebrow">Merchant workspace</span>
            <h1>ZidiCommerce</h1>
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
