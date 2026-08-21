# How to use ZidiCommerce

Practical walkthroughs for operators. For deeper behavior, see [operator-guide.md](operator-guide.md).

## How to onboard a new merchant

1. **Create the organization**  
   Register a merchant admin, or as platform admin create the org, then complete Settings → Business (name, currency `NGN`, timezone `Africa/Lagos`, contact).
2. **Add merchant details**  
   Same Business screen. This is the name customers should recognize.
3. **Add a store**  
   Commerce → Stores → Add store. Hours and pickup/delivery options.
4. **Add products**  
   Commerce → Catalogue. Categories, products, naira prices, photos, availability.
5. **Configure inventory**  
   Commerce → Inventory. Quantity per store. Set a low-stock alert.
6. **Configure payments**  
   Settings → Payments → Connect Paystack. Use test mode until you are ready. Do not paste secrets into Git or chat.
7. **Connect WhatsApp**  
   Settings → WhatsApp. Display number in the main form. Meta IDs and tokens under Advanced. Point Meta’s webhook at `/v1/runtime/webhooks/whatsapp` on the ZidiCommerce API host. Do not create a new Meta app/number if one already works.
8. **Configure the assistant**  
   Assistant → Your assistant. Create the guided assistant if none exists. Enable Take orders, Track orders, Answer questions, Handle complaints, Talk to a person as needed. Reorder the menu.
9. **Test the assistant**  
   Your assistant → Test. This does **not** send WhatsApp. Also add Knowledge FAQs and test matching.
10. **Publish**  
    Your assistant → Publish changes. Existing WhatsApp chats stay on the old snapshot until the customer starts a new session (`hi`, `menu`, `restart`).
11. **Test real WhatsApp**  
    Message the connected number. Place a small order. Confirm Overview and Orders.
12. **Monitor orders**  
    Commerce → Orders. Prepare, mark ready, hand to rider, complete.
13. **Handle support**  
    Assistant → Conversations. Claim, reply, note, resolve.

Home → Setup tracks the merchant-facing subset of these steps.

## How do I add a product?

Catalogue → Add product. Enter name, category, price in naira, optional image URL, available. Save. Then set stock in Inventory or the bot will say it is unavailable.

## How do I add another store?

Stores → Add store. Repeat hours and fulfilment. Set inventory for the new store (stock is not copied automatically). Assign staff in Team if they should only see that store.

## How do I connect WhatsApp?

Settings → WhatsApp. Enter the customer number. A technician fills Advanced (phone number ID, verify token, access token, app secret). Test WhatsApp checks that identifiers exist. Then send a real message.

## How do I change the bot?

Your assistant. Enable/disable capabilities, move them up/down, then **Publish**. Graph edits live under Advanced → Bot Builder and also require a draft + publish.

## How do I add an FAQ?

Knowledge → Question and Answer → Add FAQ. Test on that page, then Test assistant. FAQ **text** is live immediately. Turning the FAQ capability on/off still needs publish.

## How do I enable Track Order?

Your assistant → Track orders → Enable → Publish. Customers must be the same WhatsApp number that placed the order. They will see merchant order numbers, not UUIDs.

## How do I change the bot menu?

Your assistant: the numbered list is the customer menu. Use Up/Down, then Publish.

## How do I test a bot before publishing?

Your assistant → Test assistant. Simulator sessions do not call WhatsApp. After publish, test on a real phone.

## How do I publish changes?

Your assistant → Publish changes. If the version is already published, toggling a capability creates/uses a draft first, then publish.

## How do I handle an order?

Orders → select the order. Use Start preparing → Mark ready → Hand to rider → Mark delivered. Cancel only if the kitchen should stop.

## How do I refund / reconcile a payment?

Do not SQL-edit payment status. Use Advanced → Payment tools to verify a reference or reconcile. Refunds follow the existing payment/order transition rules. If Paystack shows paid and Admin does not, check the webhook URL and which secret (test vs live) Meta/Paystack is using.

## How do I handle a complaint?

Conversations: filter Waiting. Reply to the customer, add an internal note, Resolve when done. The customer profile also shows complaint counts. Enable Handle complaints on the assistant if that menu item is missing.

## How do I give a staff member access to one store?

Team → invite as Manager or Staff → tick that store. Or select an existing person, tick stores, Save access. Owners see all stores.

## How do I onboard another company?

They are a new **organization**, not a new bot type. Repeat the merchant onboarding list. Do not add merchant-specific `if` statements in Go. Use import JSON if they have a large catalogue (`merchant-config/` pattern).
