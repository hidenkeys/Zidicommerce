package bot

func JourneyTemplates() []map[string]string {
	return []map[string]string{
		{"id": "order", "name": "Product ordering", "description": "Store, catalogue, cart, verified payment and fulfilment."},
		{"id": "track_order", "name": "Order tracking", "description": "Let customers check their own orders."},
		{"id": "complaint", "name": "Complaint submission", "description": "Collect an issue and hand it to your team."},
		{"id": "support", "name": "Human support", "description": "Connect customers with a team member."},
		{"id": "catalogue", "name": "Catalogue browsing", "description": "Explore current products and available variants."},
		{"id": "custom", "name": "Custom journey", "description": "Start with a message and a question."},
		{"id": "entry_menu", "name": "Welcome menu", "description": "Route to published journeys, grounded AI or your team."},
	}
}

func JourneyTemplate(category string) (JourneyDefinition, bool) {
	if category == "entry_menu" {
		d, _ := JourneyTemplate("custom")
		d.Name, d.Purpose, d.Trigger = "Welcome menu", "Route customers to published journeys, grounded AI or human support.", "default"
		d.Steps[1].Kind, d.Steps[1].Source, d.Steps[1].Text, d.Steps[1].Title = "entry_menu", "journeys", "What would you like to do?", "Welcome menu"
		return d, true
	}
	d := JourneyDefinition{Name: "Customer journey", Category: category, Mode: "guided_chat", Channel: "whatsapp", Trigger: category, Start: "welcome", TTLMinutes: 1440}
	message := func(key, title, text, next string) JourneyStep {
		return JourneyStep{Key: key, Kind: "message", Title: title, Text: text, Next: next}
	}
	question := func(key, title, text, kind, destination, source, next string) JourneyStep {
		return JourneyStep{Key: key, Title: title, Text: text, Kind: kind, Required: true, Destination: destination, Source: source, AnswerType: "text", Presentation: "list", Next: next, Failure: "handoff", MaxRetries: 3, MaxLength: 1000, RetryMessage: "Please check your answer and try again."}
	}
	action := func(key, title, kind, next string) JourneyStep {
		return JourneyStep{Key: key, Kind: kind, Title: title, Next: next, Failure: "handoff", Locked: true}
	}
	condition := func(key, title, field, value, next, failure string) JourneyStep {
		return JourneyStep{Key: key, Kind: "condition", Title: title, ConditionField: field, ConditionOperator: "equals", ConditionValue: value, Next: next, Failure: failure, Locked: true}
	}
	handoff := JourneyStep{Key: "handoff", Kind: "handoff", Title: "Human support", Text: "A team member will help you with this request."}
	end := JourneyStep{Key: "end", Kind: "end", Title: "Finish", Text: "Thank you. You can send menu to start again."}
	switch category {
	case "order":
		d.Name = "Product ordering"
		d.Purpose = "Help customers place and pay for an order safely."
		d.Steps = []JourneyStep{
			message("welcome", "Welcome", "Welcome! Let's put your order together.", "store"),
			question("store", "Store", "Which store would you like to order from?", "store_selector", "store_id", "stores", "category"),
			question("category", "Category", "What would you like to browse?", "category_selector", "category_id", "categories", "product"),
			question("product", "Product", "Choose a product.", "product_selector", "product_id", "products", "variant"),
			question("variant", "Variant", "Choose your preferred variant.", "variant_selector", "variant_id", "variants", "quantity"),
			question("quantity", "Quantity", "How many would you like?", "quantity", "quantity", "", "stock"),
			action("stock", "Check current stock", "check_inventory", "cart"),
			action("cart", "Add item to cart", "add_to_cart", "another"),
			question("another", "Another item", "Would you like another item?", "buttons", "another", "", "fulfilment"),
			question("fulfilment", "Fulfilment", "How would you like to receive your order?", "fulfilment_selector", "fulfilment_mode", "fulfilment", "needs_address"),
			condition("needs_address", "Delivery address needed", "fulfilment_mode", "merchant_rider", "address", "customer_name"),
			question("address", "Delivery address", "What is your delivery address?", "address", "delivery_address", "", "customer_name"),
			question("customer_name", "Customer name", "What name should we use for your order?", "question", "customer_name", "", "customer_phone"),
			question("customer_phone", "Phone number", "What phone number can we reach you on?", "question", "customer_phone", "", "customer_email"),
			question("customer_email", "Email", "What email should receive your payment receipt?", "question", "customer_email", "", "review"),
			action("review", "Review cart and current prices", "review_cart", "confirm"),
			question("confirm", "Confirm order", "Ready to place this order?", "buttons", "confirmation", "", "confirmed"),
			condition("confirmed", "Explicit order confirmation", "confirmation", "confirm", "create_order", "handoff"),
			action("create_order", "Create order safely", "create_order", "payment"),
			action("payment", "Initialize secure payment", "payment", "payment_choice"),
			question("payment_choice", "Payment status", "Use the secure link above, then check payment status.", "buttons", "payment_choice", "", "verify_payment"),
			action("verify_payment", "Verify with payment provider", "payment_status", "is_paid"),
			condition("is_paid", "Payment verified", "payment_status", "paid", "notify_store", "payment_choice"),
			action("notify_store", "Notify store", "notification", "notify_customer"),
			action("notify_customer", "Notify customer", "notification", "final"),
			message("final", "Order confirmation", "Your verified payment and order are confirmed.", "end"), handoff, end,
		}
		for i := range d.Steps {
			j := &d.Steps[i]
			switch j.Key {
			case "quantity":
				min, max := 1.0, 99.0
				j.Min = &min
				j.Max = &max
			case "another":
				j.Options = []JourneyOption{{ID: "more", Label: "Add another item", Value: "more", Next: "category"}, {ID: "continue", Label: "Continue", Value: "continue", Next: "fulfilment"}}
			case "confirm":
				j.Options = []JourneyOption{{ID: "confirm", Label: "Place order", Value: "confirm"}, {ID: "cancel", Label: "Contact support", Value: "cancel", Next: "handoff"}}
			case "payment_choice":
				j.Options = []JourneyOption{{ID: "check", Label: "Check payment", Value: "check"}, {ID: "support", Label: "Contact support", Value: "support", Next: "handoff"}}
			case "customer_phone":
				j.AnswerType = "phone"
			case "customer_email":
				j.AnswerType = "email"
			case "notify_store":
				j.Source = "store"
			}
		}
	case "track_order":
		d.Name = "Order tracking"
		d.Purpose = "Show the current status of a customer's own order."
		d.Steps = []JourneyStep{message("welcome", "Welcome", "Let's check your order.", "order"), question("order", "Your orders", "Which order would you like to check?", "question", "order_id", "orders", "status"), action("status", "Current order status", "track_action", "end"), handoff, end}
	case "complaint":
		d.Name = "Complaint submission"
		d.Purpose = "Collect a complaint and request human support."
		d.Steps = []JourneyStep{message("welcome", "Welcome", "Tell us what went wrong so our team can help.", "category"), question("category", "Issue type", "What kind of issue is this?", "list", "complaint_category", "complaint_categories", "details"), question("details", "Issue details", "Please describe the issue.", "question", "complaint_message", "", "submit"), action("submit", "Record complaint", "complaint_action", "handoff"), handoff}
	case "support":
		d.Name = "Human support"
		d.Purpose = "Collect a support request and connect a team member."
		d.Steps = []JourneyStep{message("welcome", "Welcome", "Our team can help.", "details"), question("details", "How can we help?", "What would you like help with?", "question", "support_message", "", "handoff"), handoff}
	case "catalogue":
		d.Name = "Catalogue browsing"
		d.Purpose = "Browse live store products."
		d.Steps = []JourneyStep{message("welcome", "Welcome", "Explore our current catalogue.", "store"), question("store", "Store", "Choose a store.", "store_selector", "store_id", "stores", "category"), question("category", "Category", "Choose a category.", "category_selector", "category_id", "categories", "product"), question("product", "Product", "Choose a product.", "product_selector", "product_id", "products", "variant"), question("variant", "Variant", "Choose a variant to see its current price.", "variant_selector", "variant_id", "variants", "end"), handoff, end}
	case "custom":
		d.Name = "Custom journey"
		d.Steps = []JourneyStep{message("welcome", "Welcome", "Welcome! How can we help?", "question"), question("question", "Question", "Tell us what you need.", "question", "request", "", "end"), handoff, end}
		d.Steps[1].Failure = "handoff"
	default:
		return d, false
	}
	return d, true
}
