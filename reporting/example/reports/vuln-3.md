---
title: "Lorem Ipsum Finding — Insecure Direct Object Reference (IDOR) on Invoice Download"
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:N/A:N"
status: "Open"
component: "Billing / Invoices"
cwe: "CWE-639"
owasp: "A01:2021"
mitre: "T1213"
mitigations: "M1018"
impact: "Medium"
sophistication: "Low"
material_impact: "Information Privacy"
business_risk:
  - Compliance
  - Reputational
affected_urls:
  - "https://portal.loremipsum.example/invoices/download?id=1042"
timeline:
  - date: "1970-01-01T10:15:00+00:00"
    description: "Lorem ipsum dolor sit amet — discovered IDOR on the invoice download endpoint via sequential ID enumeration"
    include: true
---

## Description

Lorem ipsum dolor sit amet, consectetur adipiscing elit. The invoice download endpoint accepts a sequential numeric `id` parameter without verifying that the requesting user owns the referenced invoice: sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.

## Impact

Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip. Any authenticated user can download invoices for other customers, exposing placeholder billing details, contact information, and contract terms — duis aute irure dolor in reprehenderit.

## Replication

1. Lorem ipsum: log in with a low-privileged test account.
2. Dolor sit amet: generate or view one of your own invoices; note the `id` parameter (e.g., `id=1042`).
3. Consectetur adipiscing: modify the `id` to an adjacent value (e.g., `id=1041`, `id=1043`).
4. Sed do eiusmod: observe that the server returns the other customer's invoice without authorization checks.

**Request:**

```http
GET /invoices/download?id=1041 HTTP/1.1
Host: portal.loremipsum.example
Cookie: session=lorem-ipsum-placeholder-session
```

**Response:** HTTP 200 with another customer's placeholder invoice PDF.

![Lorem ipsum placeholder screenshot for this finding](../assets/screenshots/idor-invoice.png){width=90%}

## Remediation

Lorem ipsum dolor sit amet: implement server-side ownership checks on every object access:

```python
invoice = Invoice.get(id=request.args['id'])
if invoice.customer_id != current_user.customer_id:
    abort(403)
```

Consectetur adipiscing elit: consider replacing sequential integer IDs with unguessable UUIDs as a defense-in-depth measure.
