# Share support options (`CONTACT_SUPPORT`)

1. **What:** Tells customers how to reach the team (copy/options).
2. **Why:** Not every chat should open a handoff immediately.
3. **Problem:** Share a phone or hours for support.
4. **Who:** Owner.
5. **Where:** Your assistant.
6. **Requires:** Sensible step copy in the graph (Advanced if you need to edit wording).
7. **Configure:** Enable + publish.
8. **Runtime:** Message/options only unless a later step calls handoff.
9. **Interacts with:** Human Support, Knowledge.
10. **Test:** Choose the support menu item in simulator.
11. **Troubleshoot:** Wrong phone in the message → edit the step in Advanced Bot Builder, then publish.
12. **Mistakes:** Confusing this with an actual claimed inbox conversation.
13. **Careful:** Hardcoding a personal number you cannot staff.
14. **Developer-only:** Step message templates.
15. **Never:** Put WhatsApp tokens in the customer-facing copy.
