package bot

type ModuleSpec struct {
	Key         string          `json:"key"`
	Name        string          `json:"name"`
	Category    string          `json:"category"`
	Description string          `json:"description"`
	Parameters  []ParameterSpec `json:"parameters"`
}

type ParameterSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     string `json:"default"`
}

type ActionSpec struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description string   `json:"description"`
	Inputs      []string `json:"inputs"`
	Outputs     []string `json:"outputs"`
}

type QuestionTypeSpec struct {
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	ResponseModes []string `json:"response_modes"`
}

func SystemModules() []ModuleSpec {
	return []ModuleSpec{
		{Key: "ORDER", Name: "Order", Category: "Commerce", Description: "Guide customers through store selection, catalogue browsing, cart, fulfilment, and payment.", Parameters: []ParameterSpec{
			{Name: "require_customer_name", Type: "boolean", Description: "Collect customer name before checkout.", Default: "true"},
			{Name: "allow_pickup", Type: "boolean", Description: "Allow customer pickup.", Default: "true"},
			{Name: "allow_customer_rider", Type: "boolean", Description: "Allow customer-arranged pickup.", Default: "true"},
			{Name: "allow_merchant_rider", Type: "boolean", Description: "Allow business-managed delivery.", Default: "false"},
			{Name: "require_payment", Type: "boolean", Description: "Require payment before fulfilment.", Default: "true"},
		}},
		{Key: "TRACK_ORDER", Name: "Track Order", Category: "Commerce", Description: "Collect an order reference and show order progress."},
		{Key: "CATALOGUE", Name: "Catalogue", Category: "Commerce", Description: "Browse categories and products without starting checkout."},
		{Key: "STORE_LOCATOR", Name: "Store Locator", Category: "Commerce", Description: "Find stores by list or customer location."},
		{Key: "COMPLAINT", Name: "Complaint", Category: "Support", Description: "Collect complaint category and description for follow-up."},
		{Key: "FAQ", Name: "FAQ", Category: "Support", Description: "Answer configured frequently asked questions."},
		{Key: "HUMAN_HANDOFF", Name: "Human Handoff", Category: "Support", Description: "Route the customer to a staff/support handoff path."},
		{Key: "CONTACT_SUPPORT", Name: "Contact Support", Category: "Support", Description: "Share support contact options or collect callback details."},
		{Key: "SERVICE_BOOKING", Name: "Service booking", Category: "Field service", Description: "Configurable home-service request, booking fee, matching, and human handoff.", Parameters: []ParameterSpec{
			{Name: "entry_step", Type: "string", Description: "First step when the module starts.", Default: "start"},
			{Name: "menu_intent", Type: "string", Description: "Main menu intent key.", Default: "book_service"},
			{Name: "menu_label", Type: "string", Description: "Customer-facing menu label.", Default: "Book a service"},
		}},
	}
}

func SystemActions() []ActionSpec {
	return []ActionSpec{
		{Key: "get_store", Name: "Get Store", Category: "Commerce", Inputs: []string{"store_id"}, Outputs: []string{"store_id", "store_name", "store_address"}},
		{Key: "get_stores", Name: "List Stores", Category: "Commerce", Outputs: []string{"stores", "count"}},
		{Key: "select_store", Name: "Select Store", Category: "Commerce", Inputs: []string{"stores", "selection"}, Outputs: []string{"store_id", "store_name", "store_address"}},
		{Key: "get_nearby_store", Name: "Get Nearby Store", Category: "Commerce", Inputs: []string{"customer.location"}, Outputs: []string{"store.id", "store.distance"}},
		{Key: "get_categories", Name: "Get Categories", Category: "Commerce", Outputs: []string{"categories"}},
		{Key: "select_category", Name: "Select Category", Category: "Commerce", Inputs: []string{"categories", "selection"}, Outputs: []string{"category_id", "category_name"}},
		{Key: "get_products", Name: "Get Products", Category: "Commerce", Inputs: []string{"category_id", "store_id"}, Outputs: []string{"products"}},
		{Key: "select_product", Name: "Select Product", Category: "Commerce", Inputs: []string{"product_options", "selection"}, Outputs: []string{"product_id", "variant_id", "product_name", "variant_name", "price_minor", "available"}},
		{Key: "get_store_catalogue", Name: "Get Store Catalogue", Category: "Commerce", Inputs: []string{"store_id", "category_id"}, Outputs: []string{"products", "inventory"}},
		{Key: "get_product", Name: "Get Product", Category: "Commerce", Inputs: []string{"product_id"}, Outputs: []string{"product_id", "product_name", "variants", "image_url"}},
		{Key: "get_variant", Name: "Get Variant", Category: "Commerce", Inputs: []string{"variant_id"}, Outputs: []string{"variant_id", "variant_name", "price_minor", "currency"}},
		{Key: "get_inventory", Name: "List Inventory", Category: "Commerce", Inputs: []string{"store_id"}, Outputs: []string{"inventory"}},
		{Key: "check_inventory", Name: "Check Inventory", Category: "Commerce", Inputs: []string{"store_id", "variant_id", "quantity"}, Outputs: []string{"available", "in_stock"}},
		{Key: "create_cart", Name: "Create Cart", Category: "Commerce", Inputs: []string{"customer_id", "store_id"}, Outputs: []string{"cart_id"}},
		{Key: "get_or_create_cart", Name: "Get Or Create Cart", Category: "Commerce", Inputs: []string{"customer_id", "store_id"}, Outputs: []string{"cart_id"}},
		{Key: "add_to_cart", Name: "Add To Cart", Category: "Commerce", Inputs: []string{"cart_id", "variant_id", "quantity"}, Outputs: []string{"cart_id", "total_minor"}},
		{Key: "update_cart_item", Name: "Update Cart Item", Category: "Commerce", Inputs: []string{"cart_id", "item_id", "quantity"}, Outputs: []string{"cart_id", "total_minor"}},
		{Key: "remove_cart_item", Name: "Remove Cart Item", Category: "Commerce", Inputs: []string{"cart_id", "item_id"}, Outputs: []string{"cart_id", "total_minor"}},
		{Key: "calculate_cart", Name: "Calculate Cart", Category: "Commerce", Inputs: []string{"cart_id"}, Outputs: []string{"total_minor"}},
		{Key: "get_fulfilment_modes", Name: "Get Fulfilment Modes", Category: "Fulfilment", Inputs: []string{"store_id"}, Outputs: []string{"fulfilment_options", "count"}},
		{Key: "select_fulfilment_mode", Name: "Select Fulfilment Mode", Category: "Fulfilment", Inputs: []string{"store_id", "fulfilment_type"}, Outputs: []string{"fulfilment_type", "delivery_fee_minor"}},
		{Key: "create_order", Name: "Create Order", Category: "Commerce", Inputs: []string{"cart_id", "store_id", "customer_id", "fulfilment_type"}, Outputs: []string{"order_id", "order_number", "total_minor", "order_status"}},
		{Key: "cancel_order", Name: "Cancel Order", Category: "Commerce", Inputs: []string{"order_id"}, Outputs: []string{"order_id", "order_number", "order_status"}},
		{Key: "generate_invoice", Name: "Generate Invoice", Category: "Commerce", Inputs: []string{"order_id"}, Outputs: []string{"order_number", "total_minor", "currency"}},
		{Key: "initialize_payment", Name: "Initialize Payment", Category: "Payments", Inputs: []string{"order_id", "email"}, Outputs: []string{"payment_url", "payment_reference"}},
		{Key: "check_payment", Name: "Check Payment", Category: "Payments", Inputs: []string{"payment_reference"}, Outputs: []string{"payment_status"}},
		{Key: "get_order", Name: "Get Order", Category: "Commerce", Inputs: []string{"order_id"}, Outputs: []string{"order_status", "total_minor"}},
		{Key: "get_order_status", Name: "Get Order Status", Category: "Commerce", Inputs: []string{"order_id"}, Outputs: []string{"order_status"}},
		{Key: "get_customer_orders", Name: "Get Customer Orders", Category: "Commerce", Inputs: []string{"customer_id"}, Outputs: []string{"orders", "count"}},
		{Key: "select_order", Name: "Select Order", Category: "Commerce", Inputs: []string{"orders", "selection"}, Outputs: []string{"order_id", "order_number"}},
		{Key: "match_faq", Name: "Match FAQ", Category: "Support", Inputs: []string{"query"}, Outputs: []string{"matched", "answer", "message"}},
		{Key: "create_complaint", Name: "Create Complaint", Category: "Support", Inputs: []string{"customer_id", "message"}, Outputs: []string{"complaint_id"}},
		{Key: "notify_customer", Name: "Notify Customer", Category: "Notifications", Inputs: []string{"customer_id", "order_id", "message"}, Outputs: []string{"notification_id", "notification_status"}},
		{Key: "notify_store", Name: "Notify Store", Category: "Notifications", Inputs: []string{"order_id", "message"}, Outputs: []string{"notification_id", "notification_status"}},
		{Key: "handoff_to_agent", Name: "Handoff To Agent", Category: "Support", Inputs: []string{"customer_id", "reason"}, Outputs: []string{"handoff"}},
		{Key: "get_service_welcome", Name: "Get Service Welcome", Category: "Field service", Outputs: []string{"welcome_message", "company_name", "message"}},
		{Key: "list_service_pools", Name: "List Service Pools", Category: "Field service", Outputs: []string{"service_options", "count", "message"}},
		{Key: "select_service_pool", Name: "Select Service Pool", Category: "Field service", Inputs: []string{"service_options", "selection"}, Outputs: []string{"pool_id", "service_name", "message"}},
		{Key: "create_service_request", Name: "Create Service Request", Category: "Field service", Inputs: []string{"pool_id", "customer_name", "address", "description"}, Outputs: []string{"request_id", "request_code", "message"}},
		{Key: "initialize_booking_fee", Name: "Initialize Booking Fee", Category: "Field service", Inputs: []string{"request_id"}, Outputs: []string{"payment_url", "payment_reference", "message"}},
		{Key: "check_booking_payment", Name: "Check Booking Payment", Category: "Field service", Inputs: []string{"payment_reference"}, Outputs: []string{"payment_status", "message"}},
		{Key: "submit_service_rating", Name: "Submit Service Rating", Category: "Field service", Inputs: []string{"score", "feedback"}, Outputs: []string{"ok", "message"}},
	}
}

func QuestionTypes() []QuestionTypeSpec {
	return []QuestionTypeSpec{
		{Key: "text", Name: "Text", ResponseModes: []string{"free_text"}},
		{Key: "number", Name: "Number", ResponseModes: []string{"free_text"}},
		{Key: "email", Name: "Email", ResponseModes: []string{"free_text"}},
		{Key: "phone", Name: "Phone", ResponseModes: []string{"free_text"}},
		{Key: "location", Name: "Location", ResponseModes: []string{"location"}},
		{Key: "single_choice", Name: "Single choice", ResponseModes: []string{"buttons", "list", "single_choice"}},
		{Key: "multiple_choice", Name: "Multiple choice", ResponseModes: []string{"multiple_choice", "list"}},
		{Key: "image", Name: "Image", ResponseModes: []string{"image"}},
		{Key: "date", Name: "Date", ResponseModes: []string{"free_text"}},
		{Key: "yes_no", Name: "Yes or no", ResponseModes: []string{"buttons", "single_choice"}},
		{Key: "product_selector", Name: "Product selector", ResponseModes: []string{"product_selection", "list"}},
		{Key: "order_selector", Name: "Order selector", ResponseModes: []string{"order_selection", "list"}},
	}
}
