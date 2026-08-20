package authz

type Role string

const (
	PlatformAdmin Role = "platform_admin"
	MerchantAdmin Role = "merchant_admin"
	StoreManager  Role = "store_manager"
	StoreStaff    Role = "store_staff"
	SupportAgent  Role = "support_agent"
	Viewer        Role = "viewer"
)

func (r Role) String() string {
	return string(r)
}

func (r Role) CanManageOrganization() bool {
	return r == PlatformAdmin || r == MerchantAdmin
}

func (r Role) CanOperateStore() bool {
	return r == PlatformAdmin || r == MerchantAdmin || r == StoreManager || r == StoreStaff
}

func (r Role) CanViewCommerce() bool {
	switch r {
	case PlatformAdmin, MerchantAdmin, StoreManager, StoreStaff, SupportAgent, Viewer:
		return true
	default:
		return false
	}
}

func IsValid(role string) bool {
	switch Role(role) {
	case PlatformAdmin, MerchantAdmin, StoreManager, StoreStaff, SupportAgent, Viewer:
		return true
	default:
		return false
	}
}
