# Commerce staging: owner testing guide

Admin: [open staging Admin](https://zidicommerce-staging-admin-staging.up.railway.app/login). WhatsApp business number: **+234 808 510 5382 (ZidiHQ)**. Send from the designated tester ending **2209** only.

Use a merchant owner account to configure experiences, payments, channels and team access. The invitation accepted during acceptance testing is Store Staff (shown as Storekeeper); those configuration screens are intentionally restricted. Owner access files are documented in [test-access.md](test-access.md). Passwords are never included in these documents.

## Four modes

| Mode | Send | What you should see |
|---|---|---|
| Automated Commerce (`automated_commerce`) | `order` | “Welcome to Bing Chun. Let’s place your staging test order.” Then a current store list. |
| Guided Questions (`guided_questions`) | `guided` | “Welcome to Bing Chun. Follow these questions to place your staging test order.” Then “How can we help today?” Choose **Place an order**. |
| WhatsApp Flow (`whatsapp_flow`) | `qaflow` | “Continue your request securely in WhatsApp.” Open **Open order form**. The first screen asks you to choose a pickup store. This newly published Flow was completed during this test. |
| AI Conversation (`ai`) | `menu`, then `What is the price of one Grape Smoothie at Bingchun Jara Mall?` | The Bing Chun greeting, followed by the grounded price and available stock. The observed price was **NGN 4,200**; stock changes as test orders are completed. |

`flow` remains the pre-existing native Flow keyword. `qaflow` is the newly verified acceptance Flow. The original AI journey remains the channel default. Published QA experiences and activation status are listed in [the report](staging-e2e-report.md).

For store lists rendered as text, send the **number shown beside the current store**. Sending its name as free text was rejected during testing. For buttons and lists, choose the displayed option. Do not reuse an old message’s buttons after starting another journey.

## Complete an ordering test

For Automated, Guided ordering, or the native ordering Flow:

1. Choose **Bingchun Jara Mall** or **Bing Chun Lekki** from the current store choices.
2. Choose **Smoothies → Grape Smoothie → Regular**. Enter quantity `1`.
3. The cart should show **Grape Smoothie x1**, total **NGN 4,200**. Choose **Continue**, then **Pickup**.
4. Enter the recipient name, designated tester phone and owner-authorised receipt inbox. Choose **Place order**.
5. Expect an order reference beginning `ZC-`, **Awaiting payment**, and a Paystack checkout link.
6. Open that link. Confirm the checkout displays **Test**. Choose **Use another card** and enter Paystack’s published success test card: `4084 0840 8408 4081`, CVV `408`, and a future expiry such as `11/27`. These details are from [Paystack’s official test-payment documentation](https://paystack.com/docs/payments/test-payments/).
7. After checkout returns, expect **Payment confirmed** and a pickup code in WhatsApp. Choose **Check payment** in the journey. A browser return page alone is not proof that the order was paid.
8. In Admin → **Orders**, open this exact new reference. Verify payment is **Paid**, then use the next action for **Processing → Ready → Completed**. Expect matching WhatsApp updates. The fulfilment record should finish as completed.

AI Conversation answers approved knowledge, catalogue, price and stock questions. It does not execute unrestricted commerce writes. Send `order` to enter the protected ordering workflow; do not treat a stock answer as an order confirmation.

To test cancellation, create a separate unpaid test order and use **Cancel order** in its Admin details, or send `cancel` while its journey is active. Verify that exact order is cancelled and its reserved inventory is released. Do not cancel an unrelated existing order.

To test human support, start `guided` and choose **Talk to staff**. In Admin → **Conversations**, open the designated tester, **Assign to me**, send a clearly labelled test reply, then **Release to AI**. The reply should arrive in WhatsApp and the conversation should return to AI handling.

## Build and test your own native Flow

1. Open **Flow Studio → New experience → WhatsApp Flow → Continue**.
2. Give it a clear name and purpose. Select the ZidiHQ staging connection. Choose **Blank journey**, then **Create draft**.
3. Open **Advanced · steps and routing**. In **Node library**, choose **Text**. This adds a question before the existing Finish step.
4. In the step inspector, enter a step name and customer message. Set **Required answer**, choose its answer type and set **Next step → Finish**. Close the panel.
5. In **Experience settings**, choose a unique lowercase entry keyword, such as `dessert_request`. Confirm the ZidiHQ connection. Close the panel and **Save draft → Validate**. Resolve every issue before publication.
6. Use **Test**. Native mode initially opens its **Flow** presentation. Complete the form. Close its form sheet to return to **Chat** and check the WhatsApp bubble rendering. Simulations do not prove real delivery or payment.
7. In **Experience settings → WhatsApp Flow deployment**, **Upload draft**, **Refresh validation**, then **Publish provider Flow** once validated. The existing staging endpoint key is already registered; do not rotate it for an ordinary edit.
8. Close settings, **Publish**, confirm, then **Activate on number** and confirm. Keep the original AI default unless you intentionally want this experience to start new generic conversations.
9. From the designated tester, send the exact entry keyword. Open the form, answer every required field and finish it. Verify the received answer and final acknowledgement in Conversations.
10. Published provider revisions are immutable. For changes, use **New draft**, save and validate it, upload/publish the new provider revision, then publish and activate the matching Zidi version.

The blank-built example from this acceptance test is **Bing Chun dessert preference form**, keyword `qascratch`. It asks “Which dessert would you like to try on your next visit?” The answer `Strawberry Sundae` was submitted on real WhatsApp, followed by the final acknowledgement. This example captures a preference; it does not place an order.

## Second workspace and current constraints

The separate workspace is **Bing Chuan Desserts (Staging)**, with a Lekki kitchen, smoothie/sundae/tea products, images, stock and Paystack test configuration. The older slug labelled “second organization” was actually Bing Chun.

The shared number remains routed to Bing Chun. Second-workspace real WhatsApp tests require an operator-approved connection/asset move and Meta callback override; see the exact connection IDs and callbacks in the report. Embedded Signup remains unavailable without `META_EMBEDDED_SIGNUP_CONFIGURATION_ID`. Field is tested locally because no staging Field service is deployed.
