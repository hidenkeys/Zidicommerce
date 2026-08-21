# Take orders (`ORDER`)

1. **What:** Lets customers browse products and place orders.
2. **Why:** Primary commerce loop on WhatsApp.
3. **Problem:** Order without an app.
4. **Who:** Merchants who sell from catalogue.
5. **Where:** Your assistant → Take orders.
6. **Requires:** Active store, products with variants, inventory, usually Paystack if payment is required.
7. **Configure:** Enable the module. `require_payment` lives in module parameters (self-service default true). Publish.
8. **Runtime:** Lists stores, products, fulfilment; `get_or_create_cart`, cart items, `create_order`, optional `initialize_payment`. Prices come from variants. Inventory is reserved through commerce services.
9. **Interacts with:** Stores, catalogue, inventory, payments, Track Order.
10. **Test:** Simulator or WhatsApp: order one item. Confirm Orders in Admin.
11. **Troubleshoot:** No products → inactive catalogue or zero available stock. Payment link missing → Paystack not enabled.
12. **Mistakes:** Disabling payment in Advanced without a cash process.
13. **Careful:** Changing create_order mappings.
14. **Developer-only:** Action keys, payment flags in parameters JSON.
15. **Never:** Create orders in SQL to demo the kitchen screen.
