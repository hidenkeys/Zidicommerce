# Assistant modules

Modules are capabilities on a bot **version**. Merchants turn them on in **Your assistant**. Developers can still edit the graph in **Advanced → Bot Builder**.

After changing modules, **publish**. In-flight WhatsApp sessions keep the snapshot they started with.

| Customer language | Module key | Guide |
| --- | --- | --- |
| Welcome | `WELCOME` | [welcome.md](welcome.md) |
| Take orders | `ORDER` | [order.md](order.md) |
| Track orders | `TRACK_ORDER` | [track-order.md](track-order.md) |
| Answer questions | `FAQ` | [faq.md](faq.md) |
| Handle complaints | `COMPLAINT` | [complaint.md](complaint.md) |
| Share support options | `CONTACT_SUPPORT` | [contact-support.md](contact-support.md) |
| Talk to a person | `HUMAN_HANDOFF` | [human-handoff.md](human-handoff.md) |

Self-service bots are created with `POST /v1/bots/self-service` and already include these modules.
