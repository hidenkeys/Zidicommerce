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
	}
}

func SystemActions() []ActionSpec {
	return []ActionSpec{
		{Key: "get_store", Name: "Get Store", Category: "Commerce", Inputs: []string{"store_id"}, Outputs: []string{"store.id", "store.status"}},
		{Key: "get_nearby_store", Name: "Get Nearby Store", Category: "Commerce", Inputs: []string{"customer.location"}, Outputs: []string{"store.id", "store.distance"}},
		{Key: "get_categories", Name: "Get Categories", Category: "Commerce", Outputs: []string{"categories"}},
		{Key: "get_products", Name: "Get Products", Category: "Commerce", Inputs: []string{"category_id", "store_id"}, Outputs: []string{"products"}},
		{Key: "get_inventory", Name: "Check Inventory", Category: "Commerce", Inputs: []string{"store_id", "product_id"}, Outputs: []string{"inventory.available"}},
		{Key: "create_cart", Name: "Create Cart", Category: "Commerce", Inputs: []string{"customer.id", "store.id"}, Outputs: []string{"cart.id"}},
		{Key: "add_to_cart", Name: "Add To Cart", Category: "Commerce", Inputs: []string{"cart.id", "product_id", "quantity"}, Outputs: []string{"cart.total"}},
		{Key: "calculate_cart", Name: "Calculate Cart", Category: "Commerce", Inputs: []string{"cart.id"}, Outputs: []string{"cart.total"}},
		{Key: "create_order", Name: "Create Order", Category: "Commerce", Inputs: []string{"cart.id", "store.id", "fulfilment_mode"}, Outputs: []string{"order.id", "order.total", "order.status"}},
		{Key: "initialize_payment", Name: "Initialize Payment", Category: "Payments", Inputs: []string{"order.id", "customer.email"}, Outputs: []string{"payment.link", "payment.reference"}},
		{Key: "verify_payment", Name: "Verify Payment", Category: "Payments", Inputs: []string{"payment.reference"}, Outputs: []string{"payment.status"}},
		{Key: "get_order", Name: "Get Order", Category: "Commerce", Inputs: []string{"order.id"}, Outputs: []string{"order.status", "order.total"}},
		{Key: "create_complaint", Name: "Create Complaint", Category: "Support", Inputs: []string{"customer.id", "complaint.category", "complaint.description"}, Outputs: []string{"complaint.id"}},
		{Key: "notify_staff", Name: "Notify Staff", Category: "Support", Inputs: []string{"message", "store.id"}, Outputs: []string{"notification.status"}},
		{Key: "handoff_to_agent", Name: "Handoff To Agent", Category: "Support", Inputs: []string{"customer.id", "reason"}, Outputs: []string{"handoff.status"}},
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
