package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/bot"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"gorm.io/gorm"
)

type ActionHandler func(context.Context, RuntimeContext, map[string]any) (map[string]any, error)

type ActionRegistry struct {
	handlers map[string]ActionHandler
}

func NewActionRegistry(db *gorm.DB, commerce *core.Service) *ActionRegistry {
	registry := &ActionRegistry{handlers: map[string]ActionHandler{}}
	adapter := commerceActions{db: db, commerce: commerce}
	registry.Register("get_store", adapter.getStore)
	registry.Register("get_stores", adapter.getStore)
	registry.Register("select_store", adapter.selectStore)
	registry.Register("get_categories", adapter.getCategories)
	registry.Register("select_category", adapter.selectCategory)
	registry.Register("get_products", adapter.getProducts)
	registry.Register("select_product", adapter.selectProduct)
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
	registry.Register("get_fulfilment_modes", adapter.getFulfilmentModes)
	registry.Register("select_fulfilment_mode", adapter.selectFulfilmentMode)
	registry.Register("create_order", adapter.createOrder)
	registry.Register("cancel_order", adapter.cancelOrder)
	registry.Register("generate_invoice", adapter.getOrder)
	registry.Register("initialize_payment", adapter.initializePayment)
	registry.Register("check_payment", adapter.checkPayment)
	registry.Register("get_order", adapter.getOrder)
	registry.Register("get_order_status", adapter.getOrderStatus)
	registry.Register("get_customer_orders", adapter.getCustomerOrders)
	registry.Register("select_order", adapter.selectOrder)
	registry.Register("match_faq", adapter.matchFAQ)
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
	db       *gorm.DB
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
	return map[string]any{"stores": rows, "count": len(rows), "message": numberedRowsMessage("Choose an open store:", rows, func(row map[string]any) string {
		return strings.TrimSpace(fmt.Sprintf("%s - %s", stringValue(row["name"]), stringValue(row["address"])))
	})}, nil
}

func (a commerceActions) selectStore(_ context.Context, _ RuntimeContext, inputs map[string]any) (map[string]any, error) {
	row, err := selectRuntimeRow(inputs["stores"], inputs["selection"])
	if err != nil {
		return nil, runtimeError(ErrInvalidInput, "Choose a store using its number.")
	}
	return map[string]any{"store_id": stringValue(row["id"]), "store_name": stringValue(row["name"]), "store_address": stringValue(row["address"]), "message": "Store selected: " + stringValue(row["name"])}, nil
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
	return map[string]any{"categories": rows, "count": len(rows), "message": numberedRowsMessage("Choose a category:", rows, func(row map[string]any) string {
		return stringValue(row["name"])
	})}, nil
}

func (a commerceActions) selectCategory(_ context.Context, _ RuntimeContext, inputs map[string]any) (map[string]any, error) {
	row, err := selectRuntimeRow(inputs["categories"], inputs["selection"])
	if err != nil {
		return nil, runtimeError(ErrInvalidInput, "Choose a category using its number.")
	}
	return map[string]any{"category_id": stringValue(row["id"]), "category_name": stringValue(row["name"]), "message": "Category selected: " + stringValue(row["name"])}, nil
}

func (a commerceActions) getProducts(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	products, err := a.commerce.ListProducts(ctx, a.actor(runtimeContext))
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not load products.", "list products failed: %v", err)
	}
	categoryID := strings.TrimSpace(stringValue(inputs["category_id"]))
	storeID := strings.TrimSpace(stringValue(inputs["store_id"]))
	availability := map[string]int{}
	if storeID != "" {
		parsed, err := uuid.Parse(storeID)
		if err != nil {
			return nil, runtimeError(ErrInvalidInput, "Store selection is not valid.")
		}
		levels, err := a.commerce.ListInventory(ctx, a.actor(runtimeContext), &parsed)
		if err != nil {
			return nil, runtimeErrorf(ErrActionFailed, "I could not load inventory.", "list inventory failed: %v", err)
		}
		for _, level := range levels {
			availability[level.VariantID.String()] = level.Available()
		}
	}
	rows := make([]map[string]any, 0, len(products))
	options := make([]map[string]any, 0)
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
				option := map[string]any{"id": variant.ID.String(), "variant_id": variant.ID.String(), "name": product.Name, "product_id": product.ID.String(), "product_name": product.Name, "variant_name": variant.Name, "price_minor": variant.PriceMinor, "currency": variant.Currency, "description": product.Description, "image_url": firstImageURL(product.Images)}
				if storeID != "" {
					option["available"] = availability[variant.ID.String()]
					if availability[variant.ID.String()] <= 0 {
						continue
					}
				}
				variants = append(variants, map[string]any{"id": variant.ID.String(), "name": variant.Name, "price_minor": variant.PriceMinor, "currency": variant.Currency})
				options = append(options, option)
			}
		}
		rows = append(rows, map[string]any{"id": product.ID.String(), "name": product.Name, "category_id": stringUUID(product.CategoryID), "description": product.Description, "image_url": firstImageURL(product.Images), "variants": variants})
	}
	return map[string]any{"products": rows, "product_options": options, "count": len(options), "message": numberedRowsMessage("Choose a product:", options, func(row map[string]any) string {
		availabilityLabel := ""
		if value, ok := numberValue(row["available"]); ok {
			availabilityLabel = fmt.Sprintf(" - %d available", int(value))
		}
		return fmt.Sprintf("%s - %s - %s%s", stringValue(row["product_name"]), stringValue(row["variant_name"]), formatMinorCurrency(int64Value(row["price_minor"]), stringValue(row["currency"])), availabilityLabel)
	})}, nil
}

func (a commerceActions) selectProduct(_ context.Context, _ RuntimeContext, inputs map[string]any) (map[string]any, error) {
	row, err := selectRuntimeRow(inputs["product_options"], inputs["selection"])
	if err != nil {
		return nil, runtimeError(ErrInvalidInput, "Choose a product using its number.")
	}
	message := fmt.Sprintf("%s - %s\nPrice: %s", stringValue(row["product_name"]), stringValue(row["variant_name"]), formatMinorCurrency(int64Value(row["price_minor"]), stringValue(row["currency"])))
	if description := strings.TrimSpace(stringValue(row["description"])); description != "" {
		message += "\n" + description
	}
	return map[string]any{"product_id": stringValue(row["product_id"]), "product_name": stringValue(row["product_name"]), "variant_id": stringValue(row["variant_id"]), "variant_name": stringValue(row["variant_name"]), "price_minor": int64Value(row["price_minor"]), "currency": stringValue(row["currency"]), "available": int64Value(row["available"]), "message": message}, nil
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
	if err := ensureSessionCustomer(runtimeContext, customerID); err != nil {
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
	if err := ensureSessionCustomer(runtimeContext, customerID); err != nil {
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
	if err := a.ensureCartCustomer(ctx, runtimeContext, cartID); err != nil {
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
	outputs := cartOutputs(summary)
	outputs["message"] = cartSummaryMessage(summary)
	return outputs, nil
}

func (a commerceActions) updateCartItem(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	cartID, err := requiredUUID(inputs, "cart_id")
	if err != nil {
		return nil, err
	}
	if err := a.ensureCartCustomer(ctx, runtimeContext, cartID); err != nil {
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
	if err := a.ensureCartCustomer(ctx, runtimeContext, cartID); err != nil {
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
	if err := a.ensureCartCustomer(ctx, runtimeContext, cartID); err != nil {
		return nil, err
	}
	summary, err := a.commerce.GetCart(ctx, a.actor(runtimeContext), cartID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not calculate your cart.", "calculate cart failed: %v", err)
	}
	outputs := cartOutputs(summary)
	outputs["message"] = cartSummaryMessage(summary)
	return outputs, nil
}

func (a commerceActions) getFulfilmentModes(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	storeID, err := requiredUUID(inputs, "store_id")
	if err != nil {
		return nil, err
	}
	store, err := a.commerce.GetStore(ctx, a.actor(runtimeContext), storeID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not confirm that store.", "get store failed: %v", err)
	}
	rows := make([]map[string]any, 0, len(store.FulfilmentModes))
	for _, mode := range store.FulfilmentModes {
		if !mode.Enabled {
			continue
		}
		rows = append(rows, map[string]any{"id": mode.Mode, "mode": mode.Mode, "label": customerFulfilmentLabel(mode.Mode), "delivery_fee_minor": mode.DeliveryFeeMinor, "metadata": mode.Metadata})
	}
	if len(rows) == 0 {
		return nil, runtimeError(ErrActionFailed, "This store has no enabled fulfilment options.")
	}
	return map[string]any{"fulfilment_options": rows, "count": len(rows), "message": numberedRowsMessage("Choose how to receive your order:", rows, func(row map[string]any) string {
		fee := int64Value(row["delivery_fee_minor"])
		label := stringValue(row["label"])
		if fee > 0 {
			label += " - " + formatMinorCurrency(fee, "NGN")
		}
		return label
	})}, nil
}

func (a commerceActions) selectFulfilmentMode(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	storeID, err := requiredUUID(inputs, "store_id")
	if err != nil {
		return nil, err
	}
	mode := defaultString(stringValue(inputs["fulfilment_type"]), core.FulfilmentPickup)
	if rows := inputs["fulfilment_options"]; rows != nil && strings.TrimSpace(stringValue(inputs["selection"])) != "" {
		row, err := selectRuntimeRow(rows, inputs["selection"])
		if err != nil {
			return nil, runtimeError(ErrInvalidInput, "Choose a fulfilment option using its number.")
		}
		mode = stringValue(row["mode"])
	}
	store, err := a.commerce.GetStore(ctx, a.actor(runtimeContext), storeID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not confirm that store.", "get store failed: %v", err)
	}
	for _, candidate := range store.FulfilmentModes {
		if candidate.Mode == mode && candidate.Enabled {
			return map[string]any{"store_id": store.ID.String(), "fulfilment_type": mode, "fulfilment_label": customerFulfilmentLabel(mode), "delivery_fee_minor": candidate.DeliveryFeeMinor, "fulfilment_metadata": candidate.Metadata, "message": "Fulfilment selected: " + customerFulfilmentLabel(mode)}, nil
		}
	}
	return nil, runtimeError(ErrInvalidInput, "That delivery option is not available for this store.")
}

func (a commerceActions) createOrder(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	customerID, err := requiredUUID(inputs, "customer_id")
	if err != nil {
		return nil, err
	}
	if err := ensureSessionCustomer(runtimeContext, customerID); err != nil {
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
		if err := a.ensureCartCustomer(ctx, runtimeContext, parsed); err != nil {
			return nil, err
		}
		cartID = &parsed
	}
	idempotency := defaultString(stringValue(inputs["idempotency_key"]), "runtime-order-"+runtimeContext.Session.ID.String())
	order, err := a.commerce.CreateOrder(ctx, a.actor(runtimeContext), core.OrderInput{CartID: cartID, StoreID: storeID, CustomerID: customerID, FulfilmentType: defaultString(stringValue(inputs["fulfilment_type"]), core.FulfilmentPickup), RecipientName: stringValue(inputs["recipient_name"]), RecipientPhone: stringValue(inputs["recipient_phone"]), DeliveryAddress: stringValue(inputs["delivery_address"]), Currency: defaultString(stringValue(inputs["currency"]), "NGN"), IdempotencyKey: idempotency})
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not create your order.", "create order failed: %v", err)
	}
	outputs := orderOutputs(order)
	outputs["message"] = orderSummaryMessage(order, "Order created. Please complete payment to confirm it.")
	return outputs, nil
}

func (a commerceActions) cancelOrder(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	orderID, err := requiredUUID(inputs, "order_id")
	if err != nil {
		return nil, err
	}
	if _, err := a.getOrder(ctx, runtimeContext, map[string]any{"order_id": orderID.String()}); err != nil {
		return nil, err
	}
	order, err := a.commerce.TransitionOrder(ctx, a.actor(runtimeContext), orderID, core.TransitionInput{Status: core.OrderCancelled, Reason: defaultString(stringValue(inputs["reason"]), "customer_cancelled_before_payment"), IdempotencyKey: defaultString(stringValue(inputs["idempotency_key"]), "runtime-cancel-"+runtimeContext.Session.ID.String())})
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not cancel that order.", "cancel order failed: %v", err)
	}
	outputs := orderOutputs(order)
	outputs["message"] = "Your order has been cancelled."
	return outputs, nil
}

func (a commerceActions) checkPayment(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	customerID, err := requireSessionCustomer(runtimeContext)
	if err != nil {
		return nil, err
	}
	reference := strings.TrimSpace(stringValue(inputs["payment_reference"]))
	if reference == "" {
		return nil, runtimeError(ErrInvalidVariable, "Payment reference is missing.")
	}
	payment, err := a.commerce.GetPaymentByReference(ctx, a.actor(runtimeContext), reference)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not find that payment.", "get payment failed: %v", err)
	}
	if payment.Status != core.PaymentPaid {
		if verified, err := a.commerce.VerifyPayment(ctx, a.actor(runtimeContext), core.PaymentVerifyInput{Reference: payment.Reference}); err == nil {
			payment = verified
		}
	}
	message := "Your payment has not been completed yet. You can try again, cancel the order, or talk to support."
	if payment.Status == core.PaymentPaid {
		order, err := a.commerce.GetOrder(ctx, a.actor(runtimeContext), payment.OrderID)
		if err != nil {
			return nil, runtimeErrorf(ErrActionFailed, "I could not load your confirmed order.", "get paid order failed: %v", err)
		}
		if order.CustomerID != customerID {
			return nil, runtimeError(ErrActionFailed, "I could not load that payment.")
		}
		var fulfilment *core.Fulfilment
		if row, err := a.commerce.GetFulfilment(ctx, a.actor(runtimeContext), order.ID); err == nil {
			fulfilment = &row
		}
		message = paymentConfirmationMessage(order, payment, fulfilment)
	}
	return map[string]any{"payment_id": payment.ID.String(), "payment_reference": payment.Reference, "payment_status": payment.Status, "order_id": payment.OrderID.String(), "amount_minor": payment.AmountMinor, "currency": payment.Currency, "message": message}, nil
}

func (a commerceActions) initializePayment(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	orderID, err := requiredUUID(inputs, "order_id")
	if err != nil {
		return nil, err
	}
	if _, err := a.getOrder(ctx, runtimeContext, map[string]any{"order_id": orderID.String()}); err != nil {
		return nil, err
	}
	payment, err := a.commerce.InitializePayment(ctx, a.actor(runtimeContext), core.PaymentInput{OrderID: orderID, Provider: stringValue(inputs["provider"]), Email: stringValue(inputs["email"]), CallbackURL: stringValue(inputs["callback_url"]), IdempotencyKey: defaultString(stringValue(inputs["idempotency_key"]), "runtime-payment-"+runtimeContext.Session.ID.String())})
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not initialize payment.", "initialize payment failed: %v", err)
	}
	return map[string]any{"payment_id": payment.ID.String(), "payment_reference": payment.Reference, "payment_status": payment.Status, "payment_url": payment.AuthorizationURL, "amount_minor": payment.AmountMinor, "currency": payment.Currency, "message": fmt.Sprintf("Use this secure payment link to complete your order:\n%s\n\nAfter payment, we will confirm your order automatically.", payment.AuthorizationURL)}, nil
}

func (a commerceActions) getCustomerOrders(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	customerID, err := requiredUUID(inputs, "customer_id")
	if err != nil {
		return nil, err
	}
	if err := ensureSessionCustomer(runtimeContext, customerID); err != nil {
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
	message := numberedRowsMessage("Choose an order to track:", rows, func(row map[string]any) string {
		return fmt.Sprintf("%s - %s - %s", stringValue(row["order_number"]), customerOrderStatus(stringValue(row["order_status"])), formatMinorCurrency(int64Value(row["total_minor"]), stringValue(row["currency"])))
	})
	if len(rows) == 0 {
		message = "I could not find any recent orders for this WhatsApp number."
	}
	return map[string]any{"orders": rows, "count": len(rows), "message": message}, nil
}

func (a commerceActions) selectOrder(_ context.Context, _ RuntimeContext, inputs map[string]any) (map[string]any, error) {
	row, err := selectRuntimeRow(inputs["orders"], inputs["selection"])
	if err != nil {
		return nil, runtimeError(ErrInvalidInput, "Choose an order using its number.")
	}
	return map[string]any{"order_id": stringValue(row["order_id"]), "order_number": stringValue(row["order_number"]), "message": "Order selected: " + stringValue(row["order_number"])}, nil
}

func ensureSessionCustomer(runtimeContext RuntimeContext, customerID uuid.UUID) error {
	sessionCustomerID, err := requireSessionCustomer(runtimeContext)
	if err != nil {
		return err
	}
	if customerID != sessionCustomerID {
		return runtimeError(ErrActionFailed, "I could not load those details.")
	}
	return nil
}

func requireSessionCustomer(runtimeContext RuntimeContext) (uuid.UUID, error) {
	if runtimeContext.Session.CustomerID == nil || *runtimeContext.Session.CustomerID == uuid.Nil {
		return uuid.Nil, runtimeError(ErrActionFailed, "I could not verify your customer details.")
	}
	return *runtimeContext.Session.CustomerID, nil
}

func (a commerceActions) ensureCartCustomer(ctx context.Context, runtimeContext RuntimeContext, cartID uuid.UUID) error {
	customerID, err := requireSessionCustomer(runtimeContext)
	if err != nil {
		return err
	}
	summary, err := a.commerce.GetCart(ctx, a.actor(runtimeContext), cartID)
	if err != nil {
		return runtimeErrorf(ErrActionFailed, "I could not load that cart.", "get cart failed: %v", err)
	}
	if summary.Cart.CustomerID != customerID {
		return runtimeError(ErrActionFailed, "I could not load that cart.")
	}
	return nil
}

func (a commerceActions) matchFAQ(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	query := normalizeSearchText(stringValue(inputs["query"]))
	if query == "" {
		return map[string]any{"matched": false, "message": "Please send the question you want answered."}, nil
	}
	var faqs []bot.FAQ
	if err := a.db.WithContext(ctx).Where("organization_id = ? AND status = ?", runtimeContext.Session.OrganizationID, core.StatusActive).Find(&faqs).Error; err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not search FAQs right now.", "faq lookup failed: %v", err)
	}
	best := bot.FAQ{}
	bestScore := 0.0
	for _, faq := range faqs {
		score := runtimeFAQScore(query, faq)
		if score > bestScore {
			bestScore = score
			best = faq
		}
	}
	if best.ID == uuid.Nil || bestScore < 0.24 {
		return map[string]any{"matched": false, "message": "I do not have a configured answer for that yet. I can hand you over to support."}, nil
	}
	return map[string]any{"matched": true, "faq_id": best.ID.String(), "answer": best.Answer, "message": best.Answer}, nil
}

func (a commerceActions) createComplaint(_ context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	message := defaultString(stringValue(inputs["message"]), "Customer complaint")
	return map[string]any{"complaint_id": formatRuntimeReference("cmp", runtimeContext.Session.ID), "status": "open", "handoff": true, "reason": message, "message": "Your complaint has been recorded. A team member will follow up."}, nil
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
	customerID, err := requireSessionCustomer(runtimeContext)
	if err != nil {
		return nil, err
	}
	orderID, err := requiredUUID(inputs, "order_id")
	if err != nil {
		return nil, err
	}
	order, err := a.commerce.GetOrder(ctx, a.actor(runtimeContext), orderID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not find that order.", "get order failed: %v", err)
	}
	if order.CustomerID != customerID {
		return nil, runtimeErrorf(ErrActionFailed, "I could not find that order.", "order %s does not belong to customer %s", order.ID, customerID)
	}
	return orderOutputs(order), nil
}

func (a commerceActions) getOrderStatus(ctx context.Context, runtimeContext RuntimeContext, inputs map[string]any) (map[string]any, error) {
	customerID, err := requireSessionCustomer(runtimeContext)
	if err != nil {
		return nil, err
	}
	orderID, err := requiredUUID(inputs, "order_id")
	if err != nil {
		return nil, err
	}
	order, err := a.commerce.GetOrder(ctx, a.actor(runtimeContext), orderID)
	if err != nil {
		return nil, runtimeErrorf(ErrActionFailed, "I could not find that order.", "get order failed: %v", err)
	}
	if order.CustomerID != customerID {
		return nil, runtimeErrorf(ErrActionFailed, "I could not find that order.", "order %s does not belong to customer %s", order.ID, customerID)
	}
	return map[string]any{"order_id": order.ID.String(), "order_number": order.OrderNumber, "order_status": order.Status, "message": orderSummaryMessage(order, "Here is the latest status for your order.")}, nil
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

func cartSummaryMessage(summary core.CartSummary) string {
	lines := []string{"Cart:"}
	if len(summary.Items) == 0 {
		lines = append(lines, "No items yet.")
	} else {
		for _, item := range summary.Items {
			lines = append(lines, fmt.Sprintf("- %s x%d - %s", item.ProductName, item.Item.Quantity, formatMinorCurrency(item.TotalMinor, summary.Currency)))
		}
	}
	lines = append(lines, "Total: "+formatMinorCurrency(summary.TotalMinor, summary.Currency))
	return strings.Join(lines, "\n")
}

func orderSummaryMessage(order core.Order, intro string) string {
	lines := []string{}
	if strings.TrimSpace(intro) != "" {
		lines = append(lines, intro)
	}
	lines = append(lines, "Order: "+order.OrderNumber)
	lines = append(lines, "Status: "+customerOrderStatus(order.Status))
	if len(order.Items) > 0 {
		lines = append(lines, "Items:")
		for _, item := range order.Items {
			name := item.ProductName
			if item.VariantName != "" && item.VariantName != "Regular" {
				name += " - " + item.VariantName
			}
			lines = append(lines, fmt.Sprintf("- %s x%d - %s", name, item.Quantity, formatMinorCurrency(item.TotalMinor, order.Currency)))
		}
	}
	if order.Store.Name != "" {
		lines = append(lines, "Store: "+order.Store.Name)
	}
	lines = append(lines, "Fulfilment: "+customerFulfilmentLabel(order.FulfilmentType))
	lines = append(lines, "Total: "+formatMinorCurrency(order.TotalMinor, order.Currency))
	return strings.Join(lines, "\n")
}

func paymentConfirmationMessage(order core.Order, payment core.Payment, fulfilment *core.Fulfilment) string {
	lines := []string{orderSummaryMessage(order, "Payment confirmed.")}
	if fulfilment != nil {
		if strings.TrimSpace(fulfilment.DeliveryAddress) != "" {
			lines = append(lines, "Delivery address: "+strings.TrimSpace(fulfilment.DeliveryAddress))
		}
		if strings.TrimSpace(fulfilment.RecipientName) != "" {
			lines = append(lines, "Recipient: "+strings.TrimSpace(fulfilment.RecipientName))
		}
		if strings.TrimSpace(fulfilment.RecipientPhone) != "" {
			lines = append(lines, "Recipient phone: "+strings.TrimSpace(fulfilment.RecipientPhone))
		}
	}
	lines = append(lines, "Payment status: "+customerPaymentStatus(payment.Status))
	lines = append(lines, "We will keep you updated here.")
	return strings.Join(lines, "\n")
}

func customerOrderStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case core.OrderPending:
		return "Order received"
	case core.OrderAwaitingPayment:
		return "Awaiting payment"
	case core.OrderPaid:
		return "Order confirmed"
	case core.OrderProcessing:
		return "Your order is being prepared"
	case core.OrderReady:
		return "Your order is ready"
	case core.OrderOutForDelivery:
		return "Your order is on the way"
	case core.OrderCompleted:
		return "Order completed"
	case core.OrderCancelled:
		return "Order cancelled"
	case core.OrderRefunded:
		return "Order refunded"
	default:
		return defaultString(status, "Order received")
	}
}

func customerPaymentStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case core.PaymentPaid:
		return "Paid"
	case core.PaymentPending:
		return "Pending"
	case core.PaymentFailed:
		return "Failed"
	case core.PaymentExpired:
		return "Expired"
	default:
		return defaultString(status, "Unknown")
	}
}

func customerFulfilmentLabel(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case core.FulfilmentPickup:
		return "Pickup"
	case core.FulfilmentCustomerRider:
		return "Own rider"
	case core.FulfilmentMerchantRider:
		return "Merchant delivery"
	default:
		return defaultString(mode, "Pickup")
	}
}

func numberedRowsMessage(title string, rows []map[string]any, label func(map[string]any) string) string {
	if len(rows) == 0 {
		return title + "\nNo available options right now."
	}
	lines := []string{title}
	for index, row := range rows {
		lines = append(lines, fmt.Sprintf("%d. %s", index+1, label(row)))
	}
	return strings.Join(lines, "\n")
}

func selectRuntimeRow(value any, selection any) (map[string]any, error) {
	rows := mapSlice(value)
	if len(rows) == 0 {
		return nil, runtimeError(ErrInvalidVariable, "Available options are missing.")
	}
	text := strings.TrimSpace(stringValue(selection))
	if text == "" {
		return nil, runtimeError(ErrInvalidInput, "Please choose an option.")
	}
	if index, err := strconv.Atoi(text); err == nil && index >= 1 && index <= len(rows) {
		return rows[index-1], nil
	}
	for _, row := range rows {
		if strings.EqualFold(text, stringValue(row["id"])) || strings.EqualFold(text, stringValue(row["name"])) || strings.EqualFold(text, stringValue(row["label"])) || strings.EqualFold(text, stringValue(row["product_name"])) || strings.EqualFold(text, stringValue(row["order_number"])) {
			return row, nil
		}
	}
	return nil, runtimeError(ErrInvalidInput, "Please choose one of the available options.")
}

func mapSlice(value any) []map[string]any {
	if rows, ok := value.([]map[string]any); ok {
		return rows
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil
	}
	return rows
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		if n, ok := numberValue(value); ok {
			return int64(n)
		}
		return 0
	}
}

func formatMinorCurrency(amount int64, currency string) string {
	currency = defaultString(strings.ToUpper(strings.TrimSpace(currency)), "NGN")
	major := amount / 100
	minor := amount % 100
	symbol := currency + " "
	if currency == "NGN" {
		symbol = "NGN "
	}
	if minor == 0 {
		return fmt.Sprintf("%s%d", symbol, major)
	}
	return fmt.Sprintf("%s%d.%02d", symbol, major, minor)
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

func runtimeFAQScore(query string, faq bot.FAQ) float64 {
	question := normalizeSearchText(faq.Question)
	answer := normalizeSearchText(faq.Answer)
	if question == query {
		return 1
	}
	if strings.Contains(question, query) || strings.Contains(query, question) {
		return 0.86
	}
	best := overlapScore(tokenSet(query), tokenSet(question))
	best = maxFloat(best, overlapScore(tokenSet(query), tokenSet(answer))*0.35)
	var keywords []string
	_ = json.Unmarshal([]byte(defaultArray(faq.Keywords)), &keywords)
	for _, keyword := range keywords {
		keyword = normalizeSearchText(keyword)
		if keyword == "" {
			continue
		}
		if keyword == query || strings.Contains(query, keyword) || strings.Contains(keyword, query) {
			return 0.92
		}
		best = maxFloat(best, overlapScore(tokenSet(query), tokenSet(keyword))*0.8)
	}
	return best
}

func normalizeSearchText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(".", " ", ",", " ", "?", " ", "!", " ", "-", " ", "_", " ", ":", " ", ";", " ", "\n", " ")
	return strings.Join(strings.Fields(replacer.Replace(value)), " ")
}

func tokenSet(value string) map[string]struct{} {
	tokens := map[string]struct{}{}
	for _, part := range strings.Fields(normalizeSearchText(value)) {
		if len(part) >= 3 {
			tokens[part] = struct{}{}
		}
	}
	return tokens
}

func overlapScore(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	hits := 0
	for token := range a {
		if _, ok := b[token]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(a))
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
