# Welcome

1. **What:** Greets the customer and shows the main menu.
2. **Why:** Every chat needs a start.
3. **Problem:** Customers say `hi` and should land on a useful menu.
4. **Who:** All merchants.
5. **Where:** Your assistant (usually always on).
6. **Requires:** Enabled customer modules to fill the menu.
7. **Configure:** Welcome copy is in the self-service bot / steps. Menu order comes from enabled module sort order.
8. **Runtime:** Greetings and `menu` / `restart` return to this entry. Invalid options retry, then return to menu (see runtime lifecycle).
9. **Interacts with:** Every other customer-facing module.
10. **Test:** Simulator: send `hi`.
11. **Troubleshoot:** Empty menu → no modules enabled.
12. **Mistakes:** Putting hours in Welcome instead of Knowledge.
13. **Careful:** Changing start_step_key in Advanced.
14. **Developer-only:** Welcome steps in the graph.
15. **Never:** Hardcode a merchant name in Go.
