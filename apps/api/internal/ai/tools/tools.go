package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/ai/provider"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/auth"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/authz"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/commerce/core"
	"github.com/hidenkeys/zidicommerce/apps/api/internal/runtime"
	"gorm.io/gorm"
)

type DebugCall struct {
	Name      string         `json:"name"`
	Inputs    map[string]any `json:"inputs"`
	Result    map[string]any `json:"result,omitempty"`
	Status    string         `json:"status"`
	Error     string         `json:"error,omitempty"`
	LatencyMS int64          `json:"latency_ms"`
}

type Registry struct {
	actions  *runtime.ActionRegistry
	commerce *core.Service
}

func NewRegistry(db *gorm.DB, commerce *core.Service) *Registry {
	return &Registry{actions: runtime.NewActionRegistry(db, commerce), commerce: commerce}
}

func Definitions() []provider.Tool {
	return []provider.Tool{
		tool("get_stores", "List the merchant's active stores and their addresses.", object(nil, nil)),
		tool("get_store", "Get one store by store_id. If store_id is omitted, lists stores.", object(map[string]any{"store_id": stringSchema("Store UUID")}, nil)),
		tool("get_categories", "List active product catalogue categories.", object(nil, nil)),
		tool("get_products", "List active products, variants, descriptions, prices, and product ids. Use this for product searches, prices, and product discovery.", object(map[string]any{
			"category_id": stringSchema("Optional category UUID"),
			"store_id":    stringSchema("Optional store UUID to include availability"),
		}, nil)),
		tool("get_product", "Get product details by product_id. Use get_products first if you only know a product name.", object(map[string]any{"product_id": stringSchema("Product UUID")}, []string{"product_id"})),
		tool("get_inventory", "List inventory availability, optionally for one store.", object(map[string]any{"store_id": stringSchema("Optional store UUID")}, nil)),
		tool("check_inventory", "Check whether a specific variant is available at a store for a quantity. Use get_products and get_stores first to get ids.", object(map[string]any{
			"store_id":     stringSchema("Store UUID. Use this when known."),
			"store_name":   stringSchema("Store or branch name from the customer, such as Lekki branch. Use if store_id is not known."),
			"variant_id":   stringSchema("Variant UUID. Use this when known."),
			"product_name": stringSchema("Product name from the customer, such as milkshake. Use if variant_id is not known."),
			"variant_name": stringSchema("Variant name from the customer, such as regular or large."),
			"quantity":     map[string]any{"type": "integer", "description": "Requested quantity, default 1", "minimum": 1},
		}, nil)),
		tool("get_customer_orders", "List orders belonging to the selected customer. Use this when the customer asks about their order but does not give an order id.", object(nil, nil)),
		tool("get_order", "Get a selected customer's order details by order_id.", object(map[string]any{"order_id": stringSchema("Order UUID")}, []string{"order_id"})),
		tool("get_order_status", "Get a selected customer's order status by order_id. Use get_customer_orders first if the customer did not provide an order id.", object(map[string]any{"order_id": stringSchema("Order UUID")}, []string{"order_id"})),
		tool("match_faq", "Search configured merchant FAQs for policy or support questions.", object(map[string]any{"query": stringSchema("Customer question")}, []string{"query"})),
	}
}

func (r *Registry) Execute(ctx context.Context, name string, session runtime.ConversationSession, inputs map[string]any) (map[string]any, DebugCall, error) {
	start := time.Now()
	name = strings.ToLower(strings.TrimSpace(name))
	call := DebugCall{Name: name, Inputs: clone(inputs), Status: "ok"}
	if !allowed(name) {
		call.Status = "blocked"
		call.Error = "tool is not available to AI"
		call.LatencyMS = time.Since(start).Milliseconds()
		return nil, call, errBlockedTool(name)
	}
	if inputs == nil {
		inputs = map[string]any{}
	}
	if name == "get_customer_orders" && session.CustomerID != nil {
		inputs["customer_id"] = session.CustomerID.String()
	}
	if name == "check_inventory" {
		if _, ok := inputs["quantity"]; !ok {
			inputs["quantity"] = 1
		}
		if err := r.resolveInventoryInputs(ctx, session, inputs); err != nil {
			call.Inputs = clone(inputs)
			call.Status = "error"
			call.Error = err.Error()
			call.LatencyMS = time.Since(start).Milliseconds()
			return nil, call, err
		}
	}
	rtx := runtime.RuntimeContext{
		Session:   session,
		Variables: map[string]any{},
		System:    map[string]any{},
		Source:    "ai",
	}
	result, err := r.actions.ExecuteDirect(ctx, name, rtx, inputs)
	call.Inputs = clone(inputs)
	call.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		call.Status = "error"
		call.Error = err.Error()
		return nil, call, err
	}
	call.Result = result
	return result, call, nil
}

func allowed(name string) bool {
	switch name {
	case "get_store", "get_stores", "get_categories", "get_products", "get_product", "get_inventory", "check_inventory", "get_order", "get_order_status", "get_customer_orders", "match_faq":
		return true
	default:
		return false
	}
}

type blockedToolError string

func errBlockedTool(name string) error   { return blockedToolError("blocked AI tool: " + name) }
func (e blockedToolError) Error() string { return string(e) }

func tool(name, description string, parameters map[string]any) provider.Tool {
	return provider.Tool{Type: "function", Function: provider.ToolFunction{Name: name, Description: description, Parameters: parameters}}
}

func object(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	out := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func clone(input map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range input {
		if strings.HasPrefix(key, "_") {
			continue
		}
		if id, ok := value.(uuid.UUID); ok {
			out[key] = id.String()
			continue
		}
		out[key] = value
	}
	return out
}

func (r *Registry) resolveInventoryInputs(ctx context.Context, session runtime.ConversationSession, inputs map[string]any) error {
	if r.commerce == nil {
		return nil
	}
	actor := auth.CurrentUser{ID: uuid.Nil, OrganizationID: session.OrganizationID, Role: authz.MerchantAdmin}
	latestText := strings.TrimSpace(stringValue(inputs["_latest_user_text"]))
	if strings.TrimSpace(stringValue(inputs["store_id"])) == "" {
		storeQuery := firstNonEmpty(stringValue(inputs["store_name"]), latestText)
		storeID, err := r.resolveStoreID(ctx, actor, storeQuery)
		if err != nil {
			return err
		}
		if storeID != uuid.Nil {
			inputs["store_id"] = storeID.String()
		}
	}
	if strings.TrimSpace(stringValue(inputs["variant_id"])) == "" {
		productQuery := firstNonEmpty(stringValue(inputs["product_name"]), latestText)
		variantQuery := stringValue(inputs["variant_name"])
		variantID, err := r.resolveVariantID(ctx, actor, productQuery, variantQuery)
		if err != nil {
			return err
		}
		if variantID != uuid.Nil {
			inputs["variant_id"] = variantID.String()
		}
	}
	return nil
}

func (r *Registry) resolveStoreID(ctx context.Context, actor auth.CurrentUser, query string) (uuid.UUID, error) {
	stores, err := r.commerce.ListStores(ctx, actor)
	if err != nil {
		return uuid.Nil, err
	}
	query = searchable(query)
	if query == "" && len(stores) == 1 {
		return stores[0].ID, nil
	}
	var matches []core.Store
	for _, store := range stores {
		if store.Status != core.StatusActive {
			continue
		}
		haystack := searchable(store.Name + " " + store.Address + " " + store.City + " " + store.Code)
		if query != "" && containsSearchToken(haystack, query) {
			matches = append(matches, store)
		}
	}
	if len(matches) == 1 {
		return matches[0].ID, nil
	}
	return uuid.Nil, nil
}

func (r *Registry) resolveVariantID(ctx context.Context, actor auth.CurrentUser, productQuery, variantQuery string) (uuid.UUID, error) {
	products, err := r.commerce.ListProducts(ctx, actor)
	if err != nil {
		return uuid.Nil, err
	}
	productQuery = searchable(productQuery)
	variantQuery = searchable(variantQuery)
	var matches []core.Variant
	for _, product := range products {
		if product.Status != core.StatusActive {
			continue
		}
		productHaystack := searchable(product.Name + " " + product.Description + " " + product.Slug)
		productMatches := productQuery != "" && containsSearchToken(productHaystack, productQuery)
		for _, variant := range product.Variants {
			if variant.Status != core.StatusActive {
				continue
			}
			variantHaystack := searchable(variant.Name + " " + variant.SKU)
			if productMatches || (productQuery != "" && containsSearchToken(variantHaystack, productQuery)) {
				if variantQuery == "" || containsSearchToken(variantHaystack, variantQuery) {
					matches = append(matches, variant)
				}
			}
		}
	}
	if len(matches) == 1 {
		return matches[0].ID, nil
	}
	return uuid.Nil, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func searchable(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			builder.WriteRune(r)
			continue
		}
		builder.WriteByte(' ')
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func containsSearchToken(haystack, needle string) bool {
	haystack = searchable(haystack)
	needle = searchable(needle)
	if haystack == "" || needle == "" {
		return false
	}
	if strings.Contains(haystack, needle) {
		return true
	}
	for _, token := range strings.Fields(needle) {
		if len(token) <= 2 {
			continue
		}
		if strings.Contains(haystack, token) || strings.Contains(haystack, singular(token)) {
			return true
		}
	}
	return false
}

func singular(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 3 && strings.HasSuffix(value, "s") {
		return strings.TrimSuffix(value, "s")
	}
	return value
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}
