# Talk to a person (`HUMAN_HANDOFF`)

1. **What:** Sends the customer to the team inbox.
2. **Why:** The bot should stop guessing.
3. **Problem:** Escalation.
4. **Who:** Support agents, owners.
5. **Where:** Your assistant → Talk to a person. Inbox: Conversations.
6. **Requires:** Staff who watch Conversations.
7. **Configure:** Enable + publish.
8. **Runtime:** `handoff_to_agent` opens a handoff. Session status becomes handoff. Staff claim, reply, resolve, or reopen.
9. **Interacts with:** Conversations APIs, complaints, WhatsApp outbound replies.
10. **Test:** Trigger handoff, claim, reply (simulator will not hit WhatsApp; live will).
11. **Troubleshoot:** No Waiting row → handoff not created. Reply fails → channel disconnected.
12. **Mistakes:** Leaving handoffs open forever (Setup “Support” check stays incomplete).
13. **Careful:** Resolving without a customer reply.
14. **Developer-only:** Handoff records, notes, assigned user id.
15. **Never:** Reassign `session_id` in SQL.
