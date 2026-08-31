package core

import (
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/httperror"
)

type Handler struct {
	service *Service
	tokens  *auth.TokenManager
}

func NewHandler(service *Service, tokens *auth.TokenManager) *Handler {
	return &Handler{service: service, tokens: tokens}
}

func (h *Handler) Register(router fiber.Router) {
	router.Post("/onboarding/organization", h.onboardOrganization)
	router.Patch("/onboarding/progress", h.updateOnboarding)

	router.Get("/organizations", auth.RequireRole(authz.PlatformAdmin), h.listOrganizations)
	router.Post("/organizations", auth.RequireRole(authz.PlatformAdmin), h.createOrganization)
	router.Get("/organizations/:id", h.getOrganization)
	router.Patch("/organizations/:id", h.updateOrganization)
	router.Get("/organizations/current/members", h.listMembers)
	router.Post("/organizations/current/invitations", h.inviteMember)
	router.Get("/organizations/current/invitations", h.listInvitations)
	router.Post("/organizations/current/invitations/:id/resend", h.resendInvitation)
	router.Patch("/organizations/current/members/:id", h.updateMember)
	router.Get("/organizations/current/members/:id/stores", h.listMemberStores)
	router.Put("/organizations/current/members/:id/stores", h.assignMemberStores)
	router.Get("/organizations/current/audit-logs", h.listAuditLogs)

	router.Get("/stores", auth.RequirePermission(authz.PermissionStoresView), h.listStores)
	router.Post("/stores", auth.RequirePermission(authz.PermissionStoresCreate), h.createStore)
	router.Get("/stores/:id", h.getStore)
	router.Patch("/stores/:id", h.updateStore)
	router.Post("/stores/:id/activate", h.setStoreStatus(StatusActive))
	router.Post("/stores/:id/deactivate", h.setStoreStatus(StatusInactive))

	router.Get("/catalogue/categories", h.listCategories)
	router.Post("/catalogue/categories", auth.RequirePermission(authz.PermissionCatalogueManage), h.createCategory)
	router.Get("/catalogue/products", h.listProducts)
	router.Post("/catalogue/products", auth.RequirePermission(authz.PermissionCatalogueManage), h.createProduct)
	router.Get("/catalogue/products/:id", h.getProduct)
	router.Patch("/catalogue/products/:id", auth.RequirePermission(authz.PermissionCatalogueManage), h.updateProduct)
	router.Post("/catalogue/products/:id/variants", auth.RequirePermission(authz.PermissionCatalogueManage), h.createVariant)
	router.Post("/catalogue/products/:id/images", auth.RequirePermission(authz.PermissionCatalogueManage), h.createImage)
	router.Patch("/catalogue/variants/:id", auth.RequirePermission(authz.PermissionCatalogueManage), h.updateVariant)

	router.Get("/inventory", auth.RequirePermission(authz.PermissionInventoryView), h.listInventory)
	router.Post("/inventory", auth.RequirePermission(authz.PermissionInventoryAdjust), h.upsertInventory)
	router.Patch("/inventory/:id", auth.RequirePermission(authz.PermissionInventoryAdjust), h.updateInventory)

	router.Get("/customers", auth.RequirePermission(authz.PermissionCustomersView), h.listCustomers)
	router.Post("/customers", auth.RequirePermission(authz.PermissionCustomersManage), h.createCustomer)

	router.Post("/carts", h.createCart)
	router.Get("/carts/:id", h.getCart)
	router.Post("/carts/:id/items", h.addCartItem)
	router.Patch("/carts/:id/items/:item_id", h.updateCartItem)
	router.Delete("/carts/:id/items/:item_id", h.removeCartItem)
	router.Delete("/carts/:id/items", h.clearCart)

	router.Get("/orders", auth.RequirePermission(authz.PermissionOrdersView), h.listOrders)
	router.Post("/orders", h.createOrder)
	router.Get("/orders/:id", h.getOrder)
	router.Get("/orders/:id/operations", h.getOrderOperations)
	router.Post("/orders/:id/transition", auth.RequirePermission(authz.PermissionOrdersManage), h.transitionOrder)

	router.Get("/payments", auth.RequirePermission(authz.PermissionPaymentsView), h.listPayments)
	router.Post("/payments/initialize", h.initializePayment)
	router.Post("/payments/verify", h.verifyPayment)
	router.Post("/payments/:id/reconcile", auth.RequirePermission(authz.PermissionPaymentsReconcile), h.reconcilePayment)
	router.Get("/payment-configurations", auth.RequirePermission(authz.PermissionPaymentsView), h.listPaymentConfigurations)
	router.Post("/payment-configurations", auth.RequirePermission(authz.PermissionPaymentsManage), h.upsertPaymentConfiguration)
	router.Post("/payment-configurations/:provider/test", auth.RequirePermission(authz.PermissionPaymentsManage), h.testPaymentConfiguration)

	router.Get("/fulfilment/:order_id", h.getFulfilment)
	router.Patch("/fulfilment/:order_id", auth.RequirePermission(authz.PermissionOrdersManage), h.updateFulfilment)

	router.Get("/channels", auth.RequirePermission(authz.PermissionChannelsView), h.listChannels)
	router.Post("/channels", auth.RequirePermission(authz.PermissionChannelsManage), h.createChannel)
	router.Patch("/channels/:id", auth.RequirePermission(authz.PermissionChannelsManage), h.updateChannel)
	router.Post("/channels/:id/test", auth.RequirePermission(authz.PermissionChannelsManage), h.testChannel)
	router.Post("/channels/:id/disconnect", auth.RequirePermission(authz.PermissionChannelsManage), h.disconnectChannel)

	router.Get("/merchant-imports", auth.RequireRole(authz.PlatformAdmin, authz.MerchantAdmin), h.listMerchantImportJobs)
	router.Post("/merchant-imports/configuration", auth.RequireRole(authz.PlatformAdmin, authz.MerchantAdmin), h.importMerchantConfiguration)
}

func (h *Handler) RegisterPublic(router fiber.Router) {
	router.Get("/invitations/:token", h.previewInvitation)
	router.Post("/invitations/:token/accept", h.acceptInvitation)
	router.Post("/payments/paystack/webhook", h.paystackWebhook)
}

func (h *Handler) onboardOrganization(c *fiber.Ctx) error {
	var input OrganizationInput
	if err := bind(c, &input); err != nil {
		return err
	}
	org, membership, err := h.service.OnboardOrganization(c.UserContext(), mustUser(c), input)
	if err != nil {
		return err
	}
	token, err := h.tokens.Issue(membership.UserID, org.ID, membership.Role)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"organization": org, "membership": membership, "access_token": token}})
}

func (h *Handler) updateOnboarding(c *fiber.Ctx) error {
	var input struct {
		OnboardingState string `json:"onboarding_state"`
	}
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateOnboardingState(c.UserContext(), mustUser(c), input.OnboardingState)
	return respond(c, data, err)
}

func (h *Handler) listMembers(c *fiber.Ctx) error {
	data, err := h.service.ListMembers(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) inviteMember(c *fiber.Ctx) error {
	var input InviteInput
	if err := bind(c, &input); err != nil {
		return err
	}
	invitation, _, err := h.service.InviteMember(c.UserContext(), mustUser(c), input)
	if err != nil {
		if apiErr, ok := err.(httperror.APIError); ok && apiErr.Code == "EMAIL_DELIVERY_FAILED" {
			return c.Status(apiErr.StatusCode).JSON(fiber.Map{
				"data": invitation,
				"error": fiber.Map{
					"code":    apiErr.Code,
					"message": apiErr.Message,
				},
			})
		}
		return err
	}
	return respond(c, invitation, nil)
}

func (h *Handler) resendInvitation(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	invitation, err := h.service.ResendInvitation(c.UserContext(), mustUser(c), id)
	if err != nil {
		if apiErr, ok := err.(httperror.APIError); ok && apiErr.Code == "EMAIL_DELIVERY_FAILED" {
			return c.Status(apiErr.StatusCode).JSON(fiber.Map{
				"data": invitation,
				"error": fiber.Map{
					"code":    apiErr.Code,
					"message": apiErr.Message,
				},
			})
		}
		return err
	}
	return respond(c, invitation, nil)
}

func (h *Handler) previewInvitation(c *fiber.Ctx) error {
	data, err := h.service.PreviewInvitation(c.UserContext(), c.Params("token"))
	return respond(c, data, err)
}

func (h *Handler) listInvitations(c *fiber.Ctx) error {
	data, err := h.service.ListInvitations(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) acceptInvitation(c *fiber.Ctx) error {
	var input AcceptInvitationInput
	if err := bind(c, &input); err != nil {
		return err
	}
	input.Token = c.Params("token")
	accepted, err := h.service.AcceptInvitation(c.UserContext(), input)
	if err != nil {
		return err
	}
	token, err := h.tokens.Issue(accepted.UserID, accepted.OrganizationID, authz.Role(accepted.Role))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"acceptance": accepted, "access_token": token}})
}

func (h *Handler) updateMember(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input MemberUpdateInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateMember(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) listMemberStores(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ListMemberStores(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) assignMemberStores(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input struct {
		StoreIDs []uuid.UUID `json:"store_ids"`
	}
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.AssignMemberStores(c.UserContext(), mustUser(c), id, input.StoreIDs)
	return respond(c, data, err)
}

func (h *Handler) listAuditLogs(c *fiber.Ctx) error {
	data, err := h.service.ListAuditLogs(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) createOrganization(c *fiber.Ctx) error {
	var input OrganizationInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateOrganization(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) listOrganizations(c *fiber.Ctx) error {
	data, err := h.service.ListOrganizations(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) getOrganization(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetOrganization(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) updateOrganization(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input OrganizationInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateOrganization(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) createStore(c *fiber.Ctx) error {
	var input StoreInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateStore(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) listStores(c *fiber.Ctx) error {
	data, err := h.service.ListStores(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) getStore(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetStore(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) updateStore(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input StoreInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateStore(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) setStoreStatus(status string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := paramID(c, "id")
		if err != nil {
			return err
		}
		data, err := h.service.SetStoreStatus(c.UserContext(), mustUser(c), id, status)
		return respond(c, data, err)
	}
}

func (h *Handler) createCategory(c *fiber.Ctx) error {
	var input CategoryInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateCategory(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) listCategories(c *fiber.Ctx) error {
	data, err := h.service.ListCategories(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) createProduct(c *fiber.Ctx) error {
	var input ProductInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateProduct(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) listProducts(c *fiber.Ctx) error {
	data, err := h.service.ListProducts(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) getProduct(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetProduct(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) updateProduct(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input ProductInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateProduct(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) createVariant(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input VariantInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateVariant(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) updateVariant(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input VariantUpdateInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateVariant(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) createImage(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input ProductImageInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateProductImage(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) listInventory(c *fiber.Ctx) error {
	var storeID *uuid.UUID
	if raw := c.Query("store_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return httperror.BadRequest("Invalid store_id")
		}
		storeID = &parsed
	}
	data, err := h.service.ListInventory(c.UserContext(), mustUser(c), storeID)
	return respond(c, data, err)
}

func (h *Handler) upsertInventory(c *fiber.Ctx) error {
	var input InventoryCreateInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpsertInventory(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) updateInventory(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input InventoryInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateInventory(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) createCustomer(c *fiber.Ctx) error {
	var input CustomerInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateCustomer(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) listCustomers(c *fiber.Ctx) error {
	data, err := h.service.ListCustomers(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) createCart(c *fiber.Ctx) error {
	var input CartInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateCart(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) getCart(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetCart(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) addCartItem(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input CartItemInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.AddCartItem(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) updateCartItem(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	itemID, err := paramID(c, "item_id")
	if err != nil {
		return err
	}
	var input CartItemInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateCartItem(c.UserContext(), mustUser(c), id, itemID, input)
	return respond(c, data, err)
}

func (h *Handler) removeCartItem(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	itemID, err := paramID(c, "item_id")
	if err != nil {
		return err
	}
	data, err := h.service.RemoveCartItem(c.UserContext(), mustUser(c), id, itemID)
	return respond(c, data, err)
}

func (h *Handler) clearCart(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ClearCart(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) createOrder(c *fiber.Ctx) error {
	var input OrderInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateOrder(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) listOrders(c *fiber.Ctx) error {
	filter := OrderFilter{Status: c.Query("status")}
	if raw := c.Query("store_id"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return httperror.BadRequest("Invalid store_id")
		}
		filter.StoreID = &parsed
	}
	data, err := h.service.ListOrders(c.UserContext(), mustUser(c), filter)
	return respond(c, data, err)
}

func (h *Handler) getOrder(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetOrder(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) getOrderOperations(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.GetOrderOperations(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) transitionOrder(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input TransitionInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.TransitionOrder(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) initializePayment(c *fiber.Ctx) error {
	var input PaymentInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.InitializePayment(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) verifyPayment(c *fiber.Ctx) error {
	var input PaymentVerifyInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.VerifyPayment(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) reconcilePayment(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.ReconcilePayment(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) paystackWebhook(c *fiber.Ctx) error {
	data, err := h.service.HandlePaystackWebhook(c.UserContext(), c.BodyRaw(), c.Get("X-Paystack-Signature"))
	return respond(c, data, err)
}

func (h *Handler) listPayments(c *fiber.Ctx) error {
	data, err := h.service.ListPayments(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) listPaymentConfigurations(c *fiber.Ctx) error {
	data, err := h.service.ListPaymentConfigurations(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) upsertPaymentConfiguration(c *fiber.Ctx) error {
	var input PaymentConfigurationInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpsertPaymentConfiguration(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) testPaymentConfiguration(c *fiber.Ctx) error {
	data, err := h.service.TestPaymentConfiguration(c.UserContext(), mustUser(c), c.Params("provider"))
	return respond(c, data, err)
}

func (h *Handler) getFulfilment(c *fiber.Ctx) error {
	id, err := paramID(c, "order_id")
	if err != nil {
		return err
	}
	data, err := h.service.GetFulfilment(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) updateFulfilment(c *fiber.Ctx) error {
	id, err := paramID(c, "order_id")
	if err != nil {
		return err
	}
	var input FulfilmentInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateFulfilment(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) createChannel(c *fiber.Ctx) error {
	var input ChannelInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.CreateChannel(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) updateChannel(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	var input ChannelInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.UpdateChannel(c.UserContext(), mustUser(c), id, input)
	return respond(c, data, err)
}

func (h *Handler) testChannel(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.TestChannel(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) disconnectChannel(c *fiber.Ctx) error {
	id, err := paramID(c, "id")
	if err != nil {
		return err
	}
	data, err := h.service.DisconnectChannel(c.UserContext(), mustUser(c), id)
	return respond(c, data, err)
}

func (h *Handler) listChannels(c *fiber.Ctx) error {
	data, err := h.service.ListChannels(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func (h *Handler) importMerchantConfiguration(c *fiber.Ctx) error {
	var input MerchantImportInput
	if err := bind(c, &input); err != nil {
		return err
	}
	data, err := h.service.ImportMerchantConfiguration(c.UserContext(), mustUser(c), input)
	return respond(c, data, err)
}

func (h *Handler) listMerchantImportJobs(c *fiber.Ctx) error {
	data, err := h.service.ListMerchantImportJobs(c.UserContext(), mustUser(c))
	return respond(c, data, err)
}

func mustUser(c *fiber.Ctx) auth.CurrentUser {
	user, err := auth.GetCurrentUser(c)
	if err != nil {
		panic(err)
	}
	return user
}

func bind(c *fiber.Ctx, target any) error {
	if err := c.BodyParser(target); err != nil {
		return httperror.BadRequest("Invalid JSON payload")
	}
	return nil
}

func paramID(c *fiber.Ctx, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params(name))
	if err != nil {
		return uuid.Nil, httperror.BadRequest("Invalid ID")
	}
	return id, nil
}

func respond(c *fiber.Ctx, data any, err error) error {
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": data})
}
