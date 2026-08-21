# Bing Chun Pilot Configuration

This folder holds tenant data templates for the Bing Chun Nigeria pilot.

Use these files as import input for:

```text
POST /v1/merchant-imports/configuration
```

Do not commit real provider secrets. The template contains controlled pilot prices and stock for the documented NGN 4,800 test order. Replace the placeholder channel identifiers and add approved product images before a public launch.

Bing Chun must remain tenant data only. Do not add Bing Chun-specific branches to backend/runtime code.
