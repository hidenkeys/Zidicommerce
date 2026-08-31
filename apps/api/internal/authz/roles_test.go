package authz

import "testing"

func TestRoleCapabilities(t *testing.T) {
	if !MerchantAdmin.CanManageOrganization() {
		t.Fatal("merchant admin should manage organization")
	}
	if StoreStaff.CanManageOrganization() {
		t.Fatal("store staff should not manage organization")
	}
	if !StoreStaff.CanOperateStore() {
		t.Fatal("store staff should operate assigned stores")
	}
	if IsValid("admin") {
		t.Fatal("legacy role must not be accepted in ZidiCommerce")
	}
}

func TestRolePermissionMatrix(t *testing.T) {
	cases := []struct {
		role       Role
		permission Permission
		allowed    bool
	}{
		{MerchantAdmin, PermissionPaymentsManage, true},
		{MerchantAdmin, PermissionBotPublish, true},
		{StoreManager, PermissionInventoryAdjust, true},
		{StoreManager, PermissionPaymentsManage, false},
		{StoreStaff, PermissionOrdersManage, true},
		{StoreStaff, PermissionInventoryAdjust, false},
		{SupportAgent, PermissionConversationsManage, true},
		{SupportAgent, PermissionChannelsManage, false},
		{Viewer, PermissionOrdersView, true},
		{Viewer, PermissionOrdersCancel, false},
	}
	for _, tc := range cases {
		if got := tc.role.HasPermission(tc.permission); got != tc.allowed {
			t.Fatalf("%s permission %s = %t, want %t", tc.role, tc.permission, got, tc.allowed)
		}
	}
}

func TestStoreScopedRoles(t *testing.T) {
	if !StoreManager.RequiresStoreScope() || !StoreStaff.RequiresStoreScope() {
		t.Fatal("store manager and staff must be store scoped")
	}
	if MerchantAdmin.RequiresStoreScope() || SupportAgent.RequiresStoreScope() || Viewer.RequiresStoreScope() {
		t.Fatal("organization-wide roles should not use store assignment scope")
	}
}
