package runtime

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
)

type ActionHandler func(context.Context, RuntimeContext, map[string]any) (map[string]any, error)

type ActionRegistry struct {
	handlers map[string]ActionHandler
}

func NewActionRegistry(commerce *core.Service) *ActionRegistry {
	registry := &ActionRegistry{handlers: map[string]ActionHandler{}}
	adapter := commerceActions{commerce: commerce}
	registry.Register("get_store", adapter.getStore)
	registry.Register("get_stores", adapter.getStore)
	registry.Register("get_categories", adapter.getCategories)
	registry.Register("get_products", adapter.getProducts)
	registry.Register("get_store_catalogue", adapter.getStoreCatalogue)
	registry.Register("get_product", adapter.getProduct)
	registry.Register("get_variant", adapter.getVariant)
	registry.Register("get_inventory", adapter.getInventory)
	registry.Register("check_inventory", adapter.checkInventory)
	registry.Register("create_cart", adapter.createCart)
	registry.Register("get_or_create_cart", adapter.getOrCreateCart)
	registry.Register("add_to_cart", adapter.addToCart)
	registry.Register("update_cart_item", adapter.updateCartItem)
	registry.Register("remove_cart_item", adapter.removeCartItem)
	registry.Register("calculate_cart", adapter.calculateCart)
	registry.Register("select_fulfilment_mode", adapter.selectFulfilmentMode)
	registry.Register("create_order", adapter.createOrder)
	registry.Register("generate_invoice", adapter.getOrder)
	registry.Register("initialize_payment", adapter.initializePayment)
	registry.Register("check_payment", adapter.checkPayment)
	registry.Register("get_order", adapter.getOrder)
	registry.Register("get_order_status", adapter.getOrderStatus)
	registry.Register("get_customer_orders", adapter.getCustomerOrders)
	registry.Register("create_complaint", adapter.createComplaint)
	registry.Register("notify_customer", adapter.notifyCustomer)
	registry.Register("notify_store", adapter.notifyStore)
	registry.Register("handoff_to_agent", func(_ context.Context, _ RuntimeContext, _ map[string]any) (map[string]any, error) {
		return map[string]any{"handoff": true, "message": "A team member will continue from here."}, nil
	})
	return registry
}

func (r *ActionRegistry) Register(key string, handler ActionHandler) {
	r.handlers[strings.ToLower(strings.TrimSpace(key))] = handler
}

func (r *ActionRegistry) Execute(ctx context.Context, action bot.Action, runtimeContext RuntimeContext) (map[string]any, error) {
	handler, ok := r.handlers[strings.ToLower(strings.TrimSpace(action.ActionType))]
	if !ok {
		return nil, runtimeErrorf(ErrActionNotFound, "This action is not available yet.", "action %s is not registered", action.ActionType)
	}
	inputs, err := mapActionInputs(action.InputMappings, runtimeContext)
	if err != nil {
		return nil, err
	}
	outputs, err := handler(ctx, runtimeContext, inputs)
	if err != nil {
		return nil, err
	}
	applyActionOutputs(action.OutputMappings, outputs, runtimeContext.Variables)
	return outputs, nil
}

type commerceActions struct {
	commerce *core.Service
}

func (a commerceActions) actor(ctx RuntimeContext) auth.CurrentUser {
	return auth.CurrentUser{ID: uuid.Nil, OrganizationID: ctx.Session.OrganizationID, Role: authz.MerchantAdmin}
}

func (a commerceActions) getStore(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	if raw := strings.TrimSpace(stringValue(inputs["store_id"])); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, runtimeError(ErrInvalidInput, "Store selection is not valid.")
		}
		store, err := a.commerce.GetStore(ctx, a.actor(runtimeContext), id)
		if err != nil {
			return nil, runtimeErrorf(ErrActionFailed, "I could not load that store.", "get_store failed: %v", err)
		}
		return map[string]any{"store_id": store.ID.String(), "store_name": store.Name, "store_address": store.Address}, nil
	}
	stores, err := a.commerce.ListStores(ctx, a.actor(runtimeContext))
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not load stores.", "list stores failed: %v", err)
	}
	rows := make([]map[string]any, 0, len(stores))
	for _, store := range stores {
		if store.Status == core.StatusActive {
			rows = append(rows, map[string]any{"id": store.ID.String(), "name": store.Name, "address": store.Address})
		}
	}
	return map[string]any{"stores": rows, "count": len(rows)}, nil
}

func (a commerceActions) getCategories(ctx context.Context, runtimeContext RuntimeContext, _ map[string]any) (map[string]any, error) {
	categories, err := a.commerce.ListCategories(ctx, a.actor(runtimeContext))
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not load categories.", "list categories failed: %v", err)
	}
	rows := make([]map[string]any, 0, len(categories))
	for _, category := range categories {
		if category.Status == core.StatusActive {
			rows = append(rows, map[string]any{"id": category.ID.String(), "name": category.Name, "slug": category.Slug})
		}
	}
	return map[string]any{"categories": rows, "count": len(rows)}, nil
}

func (a commerceActions) getProducts(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	products, err := a.commerce.ListProducts(ctx, a.actor(runtimeContext))
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not load products.", "list products failed: %v", err)
	}
	categoryID := strings.TrimSpace(stringValue(inputs["category_id"]))
	rows := make([]map[string]any, 0, len(products))
	for _, product := range products {
		if product.Status != core.StatusActive {
			continue
		}
		if categoryID != "" && stringUUID(product.CategoryID) != categoryID {
			continue
		}
		variants := make([]map[string]any, 0, len(product.Variants))
		for _, variant := range product.Variants {
			if variant.Status == core.StatusActive {
				variants = append(variants, map[string]any{"id": variant.ID.String(), "name": variant.Name, "price_minor": variant.PriceMinor, "currency": variant.Currency})
			}
		}
		rows = append(rows, map[string]any{"id": product.ID.String(), "name": product.Name, "category_id": stringUUID(product.CategoryID), "description": product.Description, "image_url": firstImageURL(product.Images), "variants": variants})
	}
	return map[string]any{"products": rows, "count": len(rows)}, nil
}

func (a commerceActions) getStoreCatalogue(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	outputs, err := a.getProducts(ctx, runtimeContext, inputs)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(stringValue(inputs["store_id"])) == "" {
		return outputs, nil
	}
	inventory, err := a.getInventory(ctx, runtimeContext, inputs)
	if err != nil {
		return nil, err
	}
	outputs["inventory"] = inventory["inventory"]
	return outputs, nil
}

func (a commerceActions) getProduct(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	productID, err := requiredUUID(inputs, "product_id")
	if err != nil {
		return nil, err
	}
	product, err := a.commerce.GetProduct(ctx, a.actor(runtimeContext), productID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not load that product.", "get product failed: %v", err)
	}
	return productOutput(product), nil
}

func (a commerceActions) getVariant(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	variantID, err := requiredUUID(inputs, "variant_id")
	if err != nil {
		return nil, err
	}
	variant, err := a.commerce.GetVariant(ctx, a.actor(runtimeContext), variantID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not load that item.", "get variant failed: %v", err)
	}
	return variantOutput(variant), nil
}

func (a commerceActions) getInventory(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	var storeID *uuid.UUID
	if raw := strings.TrimSpace(stringValue(inputs["store_id"])); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return nil, runtimeError(ErrInvalidInput, "Store selection is not valid.")
		}
		storeID = &parsed
	}
	levels, err := a.commerce.ListInventory(ctx, a.actor(runtimeContext), storeID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not load inventory.", "list inventory failed: %v", err)
	}
	rows := make([]map[string]any, 0, len(levels))
	for _, level := range levels {
		rows = append(rows, map[string]any{"id": level.ID.String(), "store_id": level.StoreID.String(), "variant_id": level.VariantID.String(), "available": level.Available()})
	}
	return map[string]any{"inventory": rows, "count": len(rows)}, nil
}

func (a commerceActions) checkInventory(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	storeID, err := requiredUUID(inputs, "store_id")
	if err != nil {
		return nil, err
	}
	variantID, err := requiredUUID(inputs, "variant_id")
	if err != nil {
		return nil, err
	}
	quantity, err := requiredInt(inputs, "quantity")
	if err != nil {
		return nil, err
	}
	level, err := a.commerce.CheckInventory(ctx, a.actor(runtimeContext), storeID, variantID, quantity)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "That quantity is not available.", "check inventory failed: %v", err)
	}
	return map[string]any{"inventory_id": level.ID.String(), "store_id": level.StoreID.String(), "variant_id": level.VariantID.String(), "available": level.Available(), "requested": quantity, "in_stock": true}, nil
}

func (a commerceActions) createCart(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	customerID, err := requiredUUID(inputs, "customer_id")
	if err != nil {
		return nil, err
	}
	storeID, err := requiredUUID(inputs, "store_id")
	if err != nil {
		return nil, err
	}
	cart, err := a.commerce.CreateCart(ctx, a.actor(runtimeContext), core.CartInput{CustomerID: customerID, StoreID: storeID, Currency: defaultString(stringValue(inputs["currency"]), "NGN")})
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not create your cart.", "create cart failed: %v", err)
	}
	return map[string]any{"cart_id": cart.ID.String(), "cart_status": cart.Status}, nil
}

func (a commerceActions) getOrCreateCart(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	customerID, err := requiredUUID(inputs, "customer_id")
	if err != nil {
		return nil, err
	}
	storeID, err := requiredUUID(inputs, "store_id")
	if err != nil {
		return nil, err
	}
	cart, err := a.commerce.GetOrCreateActiveCart(ctx, a.actor(runtimeContext), core.CartInput{CustomerID: customerID, StoreID: storeID, Currency: defaultString(stringValue(inputs["currency"]), "NGN")})
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not prepare your cart.", "get or create cart failed: %v", err)
	}
	return map[string]any{"cart_id": cart.ID.String(), "cart_status": cart.Status}, nil
}

func (a commerceActions) addToCart(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	cartID, err := requiredUUID(inputs, "cart_id")
	if err != nil {
		return nil, err
	}
	variantID, err := requiredUUID(inputs, "variant_id")
	if err != nil {
		return nil, err
	}
	quantity, err := requiredInt(inputs, "quantity")
	if err != nil {
		return nil, err
	}
	summary, err := a.commerce.AddCartItem(ctx, a.actor(runtimeContext), cartID, core.CartItemInput{VariantID: variantID, Quantity: quantity})
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not add that item to your cart.", "add cart item failed: %v", err)
	}
	return cartOutputs(summary), nil
}

func (a commerceActions) updateCartItem(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	cartID, err := requiredUUID(inputs, "cart_id")
	if err != nil {
		return nil, err
	}
	itemID, err := requiredUUID(inputs, "item_id")
	if err != nil {
		return nil, err
	}
	quantity, err := requiredInt(inputs, "quantity")
	if err != nil {
		return nil, err
	}
	summary, err := a.commerce.UpdateCartItem(ctx, a.actor(runtimeContext), cartID, itemID, core.CartItemInput{Quantity: quantity})
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not update your cart.", "update cart item failed: %v", err)
	}
	return cartOutputs(summary), nil
}

func (a commerceActions) removeCartItem(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	cartID, err := requiredUUID(inputs, "cart_id")
	if err != nil {
		return nil, err
	}
	itemID, err := requiredUUID(inputs, "item_id")
	if err != nil {
		return nil, err
	}
	summary, err := a.commerce.RemoveCartItem(ctx, a.actor(runtimeContext), cartID, itemID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not remove that item.", "remove cart item failed: %v", err)
	}
	return cartOutputs(summary), nil
}

func (a commerceActions) calculateCart(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	cartID, err := requiredUUID(inputs, "cart_id")
	if err != nil {
		return nil, err
	}
	summary, err := a.commerce.GetCart(ctx, a.actor(runtimeContext), cartID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not calculate your cart.", "calculate cart failed: %v", err)
	}
	return cartOutputs(summary), nil
}

func (a commerceActions) selectFulfilmentMode(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	storeID, err := requiredUUID(inputs, "store_id")
	if err != nil {
		return nil, err
	}
	mode := defaultString(stringValue(inputs["fulfilment_type"]), core.FulfilmentPickup)
	store, err := a.commerce.GetStore(ctx, a.actor(runtimeContext), storeID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not confirm that store.", "get store failed: %v", err)
	}
	for _, candidate := range store.FulfilmentModes {
		if candidate.Mode == mode && candidate.Enabled {
			return map[string]any{"store_id": store.ID.String(), "fulfilment_type": mode, "delivery_fee_minor": candidate.DeliveryFeeMinor, "fulfilment_metadata": candidate.Metadata}, nil
		}
	}
	return nil, runtimeError(ErrInvalidInput, "That delivery option is not available for this store.")
}

func (a commerceActions) createOrder(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	customerID, err := requiredUUID(inputs, "customer_id")
	if err != nil {
		return nil, err
	}
	storeID, err := requiredUUID(inputs, "store_id")
	if err != nil {
		return nil, err
	}
	var cartID *uuid.UUID
	if raw := strings.TrimSpace(stringValue(inputs["cart_id"])); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return nil, runtimeError(ErrInvalidInput, "Cart selection is not valid.")
		}
		cartID = &parsed
	}
	idempotency := defaultString(stringValue(inputs["idempotency_key"]), "runtime-order-"+runtimeContext.Session.ID.String())
	order, err := a.commerce.CreateOrder(ctx, a.actor(runtimeContext), core.OrderInput{CartID: cartID, StoreID: storeID, CustomerID: customerID, FulfilmentType: defaultString(stringValue(inputs["fulfilment_type"]), core.FulfilmentPickup), RecipientName: stringValue(inputs["recipient_name"]), RecipientPhone: stringValue(inputs["recipient_phone"]), DeliveryAddress: stringValue(inputs["delivery_address"]), Currency: defaultString(stringValue(inputs["currency"]), "NGN"), IdempotencyKey: idempotency})
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not create your order.", "create order failed: %v", err)
	}
	return orderOutputs(order), nil
}

func (a commerceActions) checkPayment(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	reference := strings.TrimSpace(stringValue(inputs["payment_reference"]))
	if reference == "" {
		return nil, runtimeError(ErrInvalidVariable, "Payment reference is missing.")
	}
	payment, err := a.commerce.GetPaymentByReference(ctx, a.actor(runtimeContext), reference)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not find that payment.", "get payment failed: %v", err)
	}
	return map[string]any{"payment_id": payment.ID.String(), "payment_reference": payment.Reference, "payment_status": payment.Status, "order_id": payment.OrderID.String(), "amount_minor": payment.AmountMinor, "currency": payment.Currency}, nil
}

func (a commerceActions) initializePayment(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	orderID, err := requiredUUID(inputs, "order_id")
	if err != nil {
		return nil, err
	}
	payment, err := a.commerce.InitializePayment(ctx, a.actor(runtimeContext), core.PaymentInput{OrderID: orderID, Provider: stringValue(inputs["provider"]), Email: stringValue(inputs["email"]), CallbackURL: stringValue(inputs["callback_url"]), IdempotencyKey: defaultString(stringValue(inputs["idempotency_key"]), "runtime-payment-"+runtimeContext.Session.ID.String())})
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not initialize payment.", "initialize payment failed: %v", err)
	}
	return map[string]any{"payment_id": payment.ID.String(), "payment_reference": payment.Reference, "payment_status": payment.Status, "payment_url": payment.AuthorizationURL, "amount_minor": payment.AmountMinor, "currency": payment.Currency}, nil
}

func (a commerceActions) getCustomerOrders(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	customerID, err := requiredUUID(inputs, "customer_id")
	if err != nil {
		return nil, err
	}
	orders, err := a.commerce.ListCustomerOrders(ctx, a.actor(runtimeContext), customerID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not load customer orders.", "list customer orders failed: %v", err)
	}
	rows := make([]map[string]any, 0, len(orders))
	for _, order := range orders {
		rows = append(rows, orderOutputs(order))
	}
	return map[string]any{"orders": rows, "count": len(rows)}, nil
}

func (a commerceActions) createComplaint(_ context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	return map[string]any{"complaint_id": formatRuntimeReference("cmp", runtimeContext.Session.ID), "status": "open", "message": defaultString(stringValue(inputs["message"]), "Your complaint has been recorded. A team member will follow up.")}, nil
}

func (a commerceActions) notifyCustomer(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	return a.recordNotification(ctx, runtimeContext, inputs, "customer_notification")
}

func (a commerceActions) notifyStore(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	return a.recordNotification(ctx, runtimeContext, inputs, "store_notification")
}

func (a commerceActions) recordNotification(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any, notificationType string) (map[string]any, error) {
	var orderID *uuid.UUID
	if raw := strings.TrimSpace(stringValue(inputs["order_id"])); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return nil, runtimeError(ErrInvalidInput, "Order selection is not valid.")
		}
		orderID = &parsed
	}
	var customerID *uuid.UUID
	if raw := strings.TrimSpace(stringValue(inputs["customer_id"])); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			return nil, runtimeError(ErrInvalidInput, "Customer selection is not valid.")
		}
		customerID = &parsed
	}
	channelID := runtimeContext.Session.ChannelID
	notification, err := a.commerce.RecordNotification(ctx, a.actor(runtimeContext), core.NotificationInput{OrderID: orderID, CustomerID: customerID, ChannelID: &channelID, Type: defaultString(stringValue(inputs["type"]), notificationType), Recipient: stringValue(inputs["recipient"]), Status: "queued", Payload: jsonValue(map[string]any{"message": stringValue(inputs["message"])})})
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not record that notification.", "record notification failed: %v", err)
	}
	return map[string]any{"notification_id": notification.ID.String(), "notification_status": notification.Status}, nil
}

func (a commerceActions) getOrder(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	orderID, err := requiredUUID(inputs, "order_id")
	if err != nil {
		return nil, err
	}
	order, err := a.commerce.GetOrder(ctx, a.actor(runtimeContext), orderID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not find that order.", "get order failed: %v", err)
	}
	return orderOutputs(order), nil
}

func (a commerceActions) getOrderStatus(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	outputs, err := a.getOrder(ctx, runtimeContext, inputs)
	if err != nil {
		return nil, err
	}
	return map[string]any{"order_id": outputs["order_id"], "order_number": outputs["order_number"], "order_status": outputs["order_status"]}, nil
}

func mapActionInputs(raw string, ctx RuntimeContext) (map[string]any, error) {
	mappings := parseJSONMap(raw)
	inputs := map[string]any{}
	for field, source := range mappings {
		sourcePath := stringValue(source)
		value, ok := ctx.Resolve(sourcePath)
		if !ok {
			if !strings.Contains(sourcePath, ".") && sourcePath != "" {
				inputs[field] = sourcePath
				continue
			}
			return nil, runtimeErrorf(ErrInvalidVariable, "Required information is missing.", "missing variable %s for input %s", sourcePath, field)
		}
		inputs[field] = value
	}
	return inputs, nil
}

func applyActionOutputs(raw string, outputs map[string]any, variables map[string]any) {
	mappings := parseJSONMap(raw)
	for outputName, target := range mappings {
		targetPath := stringValue(target)
		value, ok := outputs[outputName]
		if !ok {
			value, ok = lookupPath(outputs, outputName)
		}
		if !ok {
			continue
		}
		if strings.HasPrefix(targetPath, "variables.") {
			setPath(variables, targetPath, value)
		}
	}
}

func requiredUUID(inputs map[string]any, key string) (uuid.UUID, error) {
	raw := strings.TrimSpace(stringValue(inputs[key]))
	if raw == "" {
		return uuid.Nil, runtimeErrorf(ErrInvalidVariable, "Required information is missing.", "missing %s", key)
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, runtimeErrorf(ErrInvalidInput, "Some order information is not valid.", "invalid uuid for %s", key)
	}
	return id, nil
}

func requiredInt(inputs map[string]any, key string) (int, error) {
	n, ok := numberValue(inputs[key])
	if !ok {
		return 0, runtimeErrorf(ErrInvalidInput, "Please enter a valid number.", "invalid integer for %s", key)
	}
	return int(n), nil
}

func cartOutputs(summary core.CartSummary) map[string]any {
	return map[string]any{"cart_id": summary.Cart.ID.String(), "cart_status": summary.Cart.Status, "subtotal_minor": summary.SubtotalMinor, "total_minor": summary.TotalMinor, "currency": summary.Currency, "item_count": len(summary.Items)}
}

func orderOutputs(order core.Order) map[string]any {
	return map[string]any{"order_id": order.ID.String(), "order_number": order.OrderNumber, "order_status": order.Status, "fulfilment_type": order.FulfilmentType, "subtotal_minor": order.SubtotalMinor, "delivery_fee_minor": order.DeliveryFeeMinor, "total_minor": order.TotalMinor, "currency": order.Currency}
}

func productOutput(product core.Product) map[string]any {
	variants := make([]map[string]any, 0, len(product.Variants))
	for _, variant := range product.Variants {
		variants = append(variants, variantOutput(variant))
	}
	return map[string]any{"product_id": product.ID.String(), "product_name": product.Name, "category_id": stringUUID(product.CategoryID), "description": product.Description, "image_url": firstImageURL(product.Images), "variants": variants}
}

func variantOutput(variant core.Variant) map[string]any {
	return map[string]any{"variant_id": variant.ID.String(), "variant_name": variant.Name, "sku": variant.SKU, "price_minor": variant.PriceMinor, "currency": variant.Currency, "product_id": variant.ProductID.String(), "product_name": variant.Product.Name}
}

func firstImageURL(images []core.ProductImage) string {
	for _, image := range images {
		if strings.TrimSpace(image.URL) != "" {
			return image.URL
		}
	}
	return ""
}

func stringUUID(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
