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
	registry.Register("get_categories", adapter.getCategories)
	registry.Register("get_products", adapter.getProducts)
	registry.Register("get_inventory", adapter.getInventory)
	registry.Register("create_cart", adapter.createCart)
	registry.Register("add_to_cart", adapter.addToCart)
	registry.Register("calculate_cart", adapter.calculateCart)
	registry.Register("create_order", adapter.createOrder)
	registry.Register("initialize_payment", adapter.initializePayment)
	registry.Register("get_order", adapter.getOrder)
	registry.Register("get_order_status", adapter.getOrderStatus)
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

func (a commerceActions) getProducts(ctx context.Context, runtimeContext RuntimeContext, _ map[string]any) (map[string]any, error) {
	products, err := a.commerce.ListProducts(ctx, a.actor(runtimeContext))
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not load products.", "list products failed: %v", err)
	}
	rows := make([]map[string]any, 0, len(products))
	for _, product := range products {
		if product.Status != core.StatusActive {
			continue
		}
		variants := make([]map[string]any, 0, len(product.Variants))
		for _, variant := range product.Variants {
			if variant.Status == core.StatusActive {
				variants = append(variants, map[string]any{"id": variant.ID.String(), "name": variant.Name, "price_minor": variant.PriceMinor, "currency": variant.Currency})
			}
		}
		rows = append(rows, map[string]any{"id": product.ID.String(), "name": product.Name, "category_id": stringUUID(product.CategoryID), "variants": variants})
	}
	return map[string]any{"products": rows, "count": len(rows)}, nil
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
	return map[string]any{"order_id": order.ID.String(), "order_number": order.OrderNumber, "order_status": order.Status, "total_minor": order.TotalMinor, "currency": order.Currency}
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
