# Answer questions (`FAQ`)

1. **What:** Matches customer text to Knowledge answers.
2. **Why:** Hours, location, and policy questions should not need a human.
3. **Problem:** Repeat questions.
4. **Who:** Owner (content), all merchants (module).
5. **Where:** Your assistant + Knowledge.
6. **Requires:** At least one active FAQ.
7. **Configure:** Enable module, add FAQs, publish the **module**. FAQ rows are org-live.
8. **Runtime:** `match_faq` scores question/keywords. Below threshold → not found.
9. **Interacts with:** Knowledge, Welcome menu.
10. **Test:** Knowledge test box, then assistant, then WhatsApp.
11. **Troubleshoot:** No match → rewrite the question in customer language.
12. **Mistakes:** Expecting ChatGPT. This is not an LLM.
13. **Careful:** Overlapping FAQs.
14. **Developer-only:** Score threshold in `MatchFAQ`.
15. **Never:** Call embeddings services from this path without a dedicated design.
