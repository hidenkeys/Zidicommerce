# Track orders (`TRACK_ORDER`)

1. **What:** Lets customers check an existing order.
2. **Why:** Reduce “where is my order?” handoffs.
3. **Problem:** Status without sharing UUIDs.
4. **Who:** Any merchant taking orders.
5. **Where:** Your assistant → Track orders.
6. **Requires:** Orders for that WhatsApp customer.
7. **Configure:** Enable + publish.
8. **Runtime:** `get_customer_orders`. Zero orders → recovery choices (not “pick an order number”). One order → shown. Many → numbered **merchant order numbers**.
9. **Interacts with:** Orders, customers, lifecycle recovery in `internal/runtime/lifecycle.go`.
10. **Test:** Place an order, then track. Also test a number with no orders.
11. **Troubleshoot:** Empty → different phone than the order, or cancelled-only history depending on filters.
12. **Mistakes:** Asking customers for UUIDs.
13. **Careful:** Changing get_customer_orders output mapping.
14. **Developer-only:** Recovery copy in lifecycle (generic, not merchant-specific).
15. **Never:** Add Bing Chun-only branches in the tracker.
