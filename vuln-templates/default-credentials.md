---
# > [!REX-STUB]
title: "Default or Weak Credentials on [SERVICE NAME]"
# > [!REX-STUB]
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"
status: "Open"
# > [!REX-STUB]
component: "[COMPONENT — e.g. Admin login panel]"
cwe: "CWE-1392"
owasp: "A07:2021"
mitre: "T1078.001"
mitigations: "M1027, M1032"
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
  - "https://[TARGET]/[LOGIN-ENDPOINT]"
# > [!REX-STUB]
timeline:
  - date: "2024-01-01T00:00:00+00:00"
    description: "Gained access to a service using default or weak credentials"
    include: true
---

## Description

> [!REX-STUB]
[A service or administrative interface is accessible using default,
vendor-supplied, or otherwise weak/guessable credentials that were never
changed or enforced to be strong at setup time.]

## Impact

> [!REX-STUB]
[Describe what an attacker gains by authenticating with these credentials —
e.g. full administrative control of the service, access to sensitive data,
or a foothold for further lateral movement into the environment.]

## Replication

> [!REX-STUB]
[1. Identify the exposed login interface for the service.
2. Attempt authentication using known default credentials for this
   product/vendor (e.g. `admin`/`admin`, `admin`/`password`) or a small,
   commonly-guessed credential set.
3. Confirm successful authentication and note the level of access granted.]

## Remediation

> [!REX-STUB]
[Change all default credentials immediately and enforce a strong password
policy (see the missing-rate-limiting and weak-password findings for related
controls). Where possible, disable default accounts entirely rather than
just changing their passwords. Require multi-factor authentication for
administrative interfaces. Periodically audit externally-reachable services
for default-credential exposure.]
