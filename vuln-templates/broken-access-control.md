---
# > [!REX-STUB]
title: "Broken Access Control — Vertical Privilege Escalation in [COMPONENT]"
# > [!REX-STUB]
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H"
status: "Open"
# > [!REX-STUB]
component: "[COMPONENT — e.g. Admin API]"
cwe: "CWE-269"
owasp: "A01:2021"
mitre: "T1078"
mitigations: "M1026, M1018"
# > [!REX-STUB]
impact: "Critical"
# > [!REX-STUB]
sophistication: "Low"
# > [!REX-STUB]
material_impact: "Proprietary Data Loss"
# > [!REX-STUB]
business_risk:
  - Operational
  - Financial
# > [!REX-STUB]
affected_urls:
  - "https://[TARGET]/[PRIVILEGED-ENDPOINT]"
# > [!REX-STUB]
timeline:
  - date: "2024-01-01T00:00:00+00:00"
    description: "Identified vertical privilege escalation allowing a low-privileged user to reach admin functionality"
    include: true
---

## Description

> [!REX-STUB]
[A low-privileged (or unauthenticated) user is able to access an endpoint or
function that should be restricted to a higher-privileged role (e.g. an
administrator). The server-side authorization check is missing, relies
solely on client-side/UI restrictions, or can be bypassed by directly
calling the underlying API.]

## Impact

> [!REX-STUB]
[Describe what a low-privileged user gains access to — e.g. administrative
functions, other users' data, or the ability to modify application
configuration — and the resulting business impact.]

## Replication

> [!REX-STUB]
[1. Authenticate as a low-privileged user.
2. Identify a privileged endpoint normally hidden from this role (e.g. via
   proxy history captured while testing as an admin, or documentation/JS
   bundle analysis).
3. Call the privileged endpoint directly with the low-privileged user's
   session/token.
4. Observe that the request succeeds and returns privileged data or
   performs a privileged action, despite the UI not exposing this
   functionality to this role.]

## Remediation

> [!REX-STUB]
[Enforce authorization checks server-side on every request, based on the
authenticated user's role/permissions — never rely on hiding UI elements as
the only control. Apply the principle of least privilege and centralize
authorization logic (e.g. a single middleware/policy layer) rather than
re-implementing checks per endpoint. Add automated tests that assert
low-privileged roles receive 403/404 on privileged endpoints.]
