# Handle complaints (`COMPLAINT`)

1. **What:** Lets customers report a problem.
2. **Why:** Capture the issue and route to support.
3. **Problem:** Bad orders and service issues.
4. **Who:** Support team + owner.
5. **Where:** Your assistant → Handle complaints. Tickets appear with Conversations/Customers.
6. **Requires:** Published module. Usually Human Support too.
7. **Configure:** Enable + publish.
8. **Runtime:** `create_complaint` writes a support ticket. Often followed by handoff.
9. **Interacts with:** Customers, orders (if collected), Conversations.
10. **Test:** Run the complaint path; confirm a ticket and Waiting inbox item.
11. **Troubleshoot:** No ticket → module off or unpublished.
12. **Mistakes:** Using FAQ for “my order is wrong”.
13. **Careful:** PII in free text.
14. **Developer-only:** Ticket fields.
15. **Never:** Delete tickets to hide a complaint.
