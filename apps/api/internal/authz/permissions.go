package authz

type Permission string

const (
	PermissionOrganizationView   Permission = "organization.view"
	PermissionOrganizationUpdate Permission = "organization.update"

	PermissionStaffView   Permission = "staff.view"
	PermissionStaffInvite Permission = "staff.invite"
	PermissionStaffUpdate Permission = "staff.update"
	PermissionStaffRemove Permission = "staff.remove"

	PermissionStoresView   Permission = "stores.view"
	PermissionStoresCreate Permission = "stores.create"
	PermissionStoresUpdate Permission = "stores.update"
	PermissionStoresDelete Permission = "stores.delete"

	PermissionCatalogueView   Permission = "catalogue.view"
	PermissionCatalogueManage Permission = "catalogue.manage"

	PermissionInventoryView   Permission = "inventory.view"
	PermissionInventoryAdjust Permission = "inventory.adjust"

	PermissionCustomersView   Permission = "customers.view"
	PermissionCustomersManage Permission = "customers.manage"

	PermissionOrdersView   Permission = "orders.view"
	PermissionOrdersManage Permission = "orders.manage"
	PermissionOrdersCancel Permission = "orders.cancel"

	PermissionPaymentsView      Permission = "payments.view"
	PermissionPaymentsManage    Permission = "payments.manage"
	PermissionPaymentsReconcile Permission = "payments.reconcile"

	PermissionConversationsView   Permission = "conversations.view"
	PermissionConversationsManage Permission = "conversations.manage"

	PermissionComplaintsView   Permission = "complaints.view"
	PermissionComplaintsManage Permission = "complaints.manage"

	PermissionBotView    Permission = "bot.view"
	PermissionBotManage  Permission = "bot.manage"
	PermissionBotPublish Permission = "bot.publish"

	PermissionAIUse       Permission = "ai.use"
	PermissionAIConfigure Permission = "ai.configure"
	PermissionAIExecute   Permission = "ai.execute"

	PermissionKnowledgeView   Permission = "knowledge.view"
	PermissionKnowledgeManage Permission = "knowledge.manage"

	PermissionAuditView Permission = "audit.view"

	PermissionSettingsView   Permission = "settings.view"
	PermissionSettingsManage Permission = "settings.manage"

	PermissionChannelsView   Permission = "channels.view"
	PermissionChannelsManage Permission = "channels.manage"

	PermissionBillingView   Permission = "billing.view"
	PermissionBillingManage Permission = "billing.manage"
)

var allOrganizationPermissions = []Permission{
	PermissionOrganizationView, PermissionOrganizationUpdate,
	PermissionStaffView, PermissionStaffInvite, PermissionStaffUpdate, PermissionStaffRemove,
	PermissionStoresView, PermissionStoresCreate, PermissionStoresUpdate, PermissionStoresDelete,
	PermissionCatalogueView, PermissionCatalogueManage,
	PermissionInventoryView, PermissionInventoryAdjust,
	PermissionCustomersView, PermissionCustomersManage,
	PermissionOrdersView, PermissionOrdersManage, PermissionOrdersCancel,
	PermissionPaymentsView, PermissionPaymentsManage, PermissionPaymentsReconcile,
	PermissionConversationsView, PermissionConversationsManage,
	PermissionComplaintsView, PermissionComplaintsManage,
	PermissionBotView, PermissionBotManage, PermissionBotPublish,
	PermissionAIUse, PermissionAIConfigure, PermissionAIExecute,
	PermissionKnowledgeView, PermissionKnowledgeManage,
	PermissionAuditView,
	PermissionSettingsView, PermissionSettingsManage,
	PermissionChannelsView, PermissionChannelsManage,
	PermissionBillingView, PermissionBillingManage,
}

var rolePermissions = map[Role][]Permission{
	PlatformAdmin: allOrganizationPermissions,
	MerchantAdmin: allOrganizationPermissions,
	StoreManager: {
		PermissionOrganizationView,
		PermissionStaffView,
		PermissionStoresView, PermissionStoresUpdate,
		PermissionCatalogueView, PermissionCatalogueManage,
		PermissionInventoryView, PermissionInventoryAdjust,
		PermissionCustomersView,
		PermissionOrdersView, PermissionOrdersManage, PermissionOrdersCancel,
		PermissionConversationsView, PermissionConversationsManage,
		PermissionComplaintsView, PermissionComplaintsManage,
		PermissionBotView,
		PermissionAIUse, PermissionAIExecute,
		PermissionKnowledgeView,
		PermissionSettingsView,
	},
	StoreStaff: {
		PermissionOrganizationView,
		PermissionStoresView,
		PermissionCatalogueView,
		PermissionInventoryView,
		PermissionCustomersView,
		PermissionOrdersView, PermissionOrdersManage, PermissionOrdersCancel,
		PermissionConversationsView,
		PermissionComplaintsView,
		PermissionAIUse,
	},
	SupportAgent: {
		PermissionOrganizationView,
		PermissionCustomersView, PermissionCustomersManage,
		PermissionOrdersView,
		PermissionConversationsView, PermissionConversationsManage,
		PermissionComplaintsView, PermissionComplaintsManage,
		PermissionAIUse,
		PermissionKnowledgeView,
	},
	Viewer: {
		PermissionOrganizationView,
		PermissionStaffView,
		PermissionStoresView,
		PermissionCatalogueView,
		PermissionInventoryView,
		PermissionCustomersView,
		PermissionOrdersView,
		PermissionPaymentsView,
		PermissionConversationsView,
		PermissionComplaintsView,
		PermissionBotView,
		PermissionKnowledgeView,
		PermissionAuditView,
		PermissionSettingsView,
		PermissionChannelsView,
		PermissionBillingView,
	},
	ServiceProvider: {},
}

func (r Role) Permissions() []Permission {
	permissions := rolePermissions[r]
	out := make([]Permission, len(permissions))
	copy(out, permissions)
	return out
}

func (r Role) HasPermission(permission Permission) bool {
	for _, candidate := range rolePermissions[r] {
		if candidate == permission {
			return true
		}
	}
	return false
}

func (r Role) RequiresStoreScope() bool {
	return r == StoreManager || r == StoreStaff
}

type RuntimeActionMode string

const (
	RuntimeActionRead  RuntimeActionMode = "read"
	RuntimeActionWrite RuntimeActionMode = "write"
)

type RuntimeActionClass string

const (
	RuntimeActionClassRead                  RuntimeActionClass = "read"
	RuntimeActionClassSafeDeterministic     RuntimeActionClass = "safe_deterministic"
	RuntimeActionClassCommerceMutation      RuntimeActionClass = "commerce_mutation"
	RuntimeActionClassHighRiskCommerce      RuntimeActionClass = "high_risk_commerce"
	RuntimeActionConfirmationNone           string             = "none"
	RuntimeActionConfirmationCustomerIntent string             = "customer_intent"
	RuntimeActionConfirmationProviderProof  string             = "provider_proof"
	RuntimeActionConfirmationHumanReview    string             = "human_review"
)

type RuntimeActionPolicy struct {
	Action       string
	Mode         RuntimeActionMode
	Permission   Permission
	Class        RuntimeActionClass
	Confirmation string
}

func PolicyForRuntimeAction(action string) RuntimeActionPolicy {
	switch action {
	case "get_store", "get_stores", "select_store":
		return RuntimeActionPolicy{Action: action, Mode: RuntimeActionRead, Permission: PermissionStoresView, Class: RuntimeActionClassRead, Confirmation: RuntimeActionConfirmationNone}
	case "get_categories", "select_category", "get_products", "select_product", "get_store_catalogue", "get_product", "get_variant":
		return RuntimeActionPolicy{Action: action, Mode: RuntimeActionRead, Permission: PermissionCatalogueView, Class: RuntimeActionClassRead, Confirmation: RuntimeActionConfirmationNone}
	case "get_inventory", "check_inventory":
		return RuntimeActionPolicy{Action: action, Mode: RuntimeActionRead, Permission: PermissionInventoryView, Class: RuntimeActionClassRead, Confirmation: RuntimeActionConfirmationNone}
	case "create_cart", "get_or_create_cart", "add_to_cart", "update_cart_item", "remove_cart_item", "calculate_cart":
		return RuntimeActionPolicy{Action: action, Mode: RuntimeActionWrite, Permission: PermissionOrdersManage, Class: RuntimeActionClassSafeDeterministic, Confirmation: RuntimeActionConfirmationCustomerIntent}
	case "get_fulfilment_modes", "select_fulfilment_mode", "create_order", "generate_invoice":
		return RuntimeActionPolicy{Action: action, Mode: RuntimeActionWrite, Permission: PermissionOrdersManage, Class: RuntimeActionClassCommerceMutation, Confirmation: RuntimeActionConfirmationCustomerIntent}
	case "cancel_order":
		return RuntimeActionPolicy{Action: action, Mode: RuntimeActionWrite, Permission: PermissionOrdersCancel, Class: RuntimeActionClassHighRiskCommerce, Confirmation: RuntimeActionConfirmationHumanReview}
	case "initialize_payment", "check_payment":
		return RuntimeActionPolicy{Action: action, Mode: RuntimeActionWrite, Permission: PermissionPaymentsManage, Class: RuntimeActionClassHighRiskCommerce, Confirmation: RuntimeActionConfirmationProviderProof}
	case "get_order", "get_order_status", "get_customer_orders", "select_order":
		return RuntimeActionPolicy{Action: action, Mode: RuntimeActionRead, Permission: PermissionOrdersView, Class: RuntimeActionClassRead, Confirmation: RuntimeActionConfirmationNone}
	case "match_faq":
		return RuntimeActionPolicy{Action: action, Mode: RuntimeActionRead, Permission: PermissionKnowledgeView, Class: RuntimeActionClassRead, Confirmation: RuntimeActionConfirmationNone}
	case "create_complaint":
		return RuntimeActionPolicy{Action: action, Mode: RuntimeActionWrite, Permission: PermissionComplaintsManage, Class: RuntimeActionClassSafeDeterministic, Confirmation: RuntimeActionConfirmationCustomerIntent}
	case "notify_customer", "notify_store":
		return RuntimeActionPolicy{Action: action, Mode: RuntimeActionWrite, Permission: PermissionConversationsManage, Class: RuntimeActionClassSafeDeterministic, Confirmation: RuntimeActionConfirmationNone}
	case "handoff_to_agent":
		return RuntimeActionPolicy{Action: action, Mode: RuntimeActionWrite, Permission: PermissionConversationsManage, Class: RuntimeActionClassSafeDeterministic, Confirmation: RuntimeActionConfirmationCustomerIntent}
	default:
		return RuntimeActionPolicy{Action: action}
	}
}
