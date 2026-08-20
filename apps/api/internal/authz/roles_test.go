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
