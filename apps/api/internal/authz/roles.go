package authz

type Role string

const (
	PlatformAdmin   Role = "platform_admin"
	MerchantAdmin   Role = "merchant_admin"
	StoreManager    Role = "store_manager"
	StoreStaff      Role = "store_staff"
	SupportAgent    Role = "support_agent"
	Viewer          Role = "viewer"
	ServiceProvider Role = "service_provider"
)

func (r Role) String() string {
	return string(r)
}

func (r Role) CanManageOrganization() bool {
	return r.HasPermission(PermissionOrganizationUpdate)
}

func (r Role) CanOperateStore() bool {
	return r.HasPermission(PermissionStoresUpdate) || r.HasPermission(PermissionOrdersManage) || r.HasPermission(PermissionInventoryAdjust)
}

func (r Role) CanViewCommerce() bool {
	return r.HasPermission(PermissionStoresView) ||
		r.HasPermission(PermissionCatalogueView) ||
		r.HasPermission(PermissionInventoryView) ||
		r.HasPermission(PermissionCustomersView) ||
		r.HasPermission(PermissionOrdersView)
}

func IsValid(role string) bool {
	switch Role(role) {
	case PlatformAdmin, MerchantAdmin, StoreManager, StoreStaff, SupportAgent, Viewer, ServiceProvider:
		return true
	default:
		return false
	}
}

func (r Role) IsServiceProvider() bool {
	return r == ServiceProvider
}
