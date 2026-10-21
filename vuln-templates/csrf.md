---
# > [!REX-STUB]
title: "Cross-Site Request Forgery (CSRF) in [ACTION NAME]"
# > [!REX-STUB]
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:N/I:L/A:N"
status: "Open"
# > [!REX-STUB]
component: "[COMPONENT / ENDPOINT NAME]"
cwe: "CWE-352"
owasp: "A01:2021"
mitre: "T1189"
mitigations: "M1054, M1038"
# > [!REX-STUB]
impact: "Medium"
# > [!REX-STUB]
sophistication: "Low"
# > [!REX-STUB]
material_impact: "Unauthorized Account/Data Modification"
# > [!REX-STUB]
business_risk:
  - Operational
# > [!REX-STUB]
affected_urls:
  - "https://[TARGET]/[STATE-CHANGING-ENDPOINT]"
# > [!REX-STUB]
timeline:
  - date: "2024-01-01T00:00:00+00:00"
    description: "Identified missing CSRF protections on a state-changing request"
    include: true
---

## Description

> [!REX-STUB]
[The application does not implement anti-CSRF protections (e.g. a per-session
CSRF token, `SameSite` cookie attribute, or origin/referrer validation) on
one or more state-changing requests. An attacker can craft a malicious page
that, when visited by an authenticated victim, causes their browser to submit
a forged request to the application, performing the action on the victim's
behalf without their knowledge or consent.]

## Impact

> [!REX-STUB]
[Describe what a forged request can achieve in this application — e.g.
changing account email/password, transferring funds, modifying settings, or
deleting data — and any preconditions (e.g. the victim must be authenticated
and lured to a malicious page/link).]

## Replication

> [!REX-STUB]
[1. Authenticate as a victim user in one browser tab.
2. Identify a state-changing request (e.g. a POST request) that lacks a CSRF
   token or other origin validation.
3. Build an auto-submitting HTML form or fetch() call on an attacker-controlled
   page that reproduces the request, targeting the vulnerable endpoint.
4. Have the victim (while still authenticated) visit the attacker-controlled
   page.
5. Confirm the forged request executed successfully using the victim's
   session, without their explicit action on the real application.]

## Remediation

> [!REX-STUB]
[Implement anti-CSRF tokens (synchronizer token pattern) on all state-changing
requests and validate them server-side. Set the `SameSite=Lax` or
`SameSite=Strict` attribute on session cookies. Validate the `Origin`/
`Referer` header as defense in depth. Guidance is available in the OWASP
Cross-Site Request Forgery Prevention Cheat Sheet.]
