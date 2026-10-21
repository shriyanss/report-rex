---
# > [!REX-STUB]
title: "Outdated / Known-Vulnerable Component — [PRODUCT] [VERSION] ([CVE-ID])"
# > [!REX-STUB]
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"
status: "Open"
# > [!REX-STUB]
component: "[PRODUCT NAME AND VERSION]"
cwe: "CWE-1104"
owasp: "A06:2021"
mitre: "T1190"
mitigations: "M1051"
# > [!REX-STUB]
impact: "High"
# > [!REX-STUB]
sophistication: "Medium"
# > [!REX-STUB]
material_impact: "Business Interruption"
# > [!REX-STUB]
business_risk:
  - Operational
# > [!REX-STUB]
affected_urls:
  - "https://[TARGET]:[PORT]"
# > [!REX-STUB]
timeline:
  - date: "2024-01-01T00:00:00+00:00"
    description: "Identified an outdated, known-vulnerable software component in use"
    include: true
---

## Description

> [!REX-STUB]
[A component (server software, library, framework, or plugin) is running an
outdated version with one or more publicly disclosed vulnerabilities
([CVE-ID]). Version/banner information was observed that identifies the
exact release in use, which is behind the vendor's current patched
version.]

## Impact

> [!REX-STUB]
[Describe the impact of the specific CVE(s) identified — e.g. remote code
execution, authentication bypass, or denial of service — referencing the
CVE's published CVSS score and any known public exploit availability.]

## Replication

> [!REX-STUB]
[1. Identify the running component's version via a banner, HTTP response
   header, or version-disclosure endpoint.
2. Cross-reference the identified version against public vulnerability
   databases (NVD, vendor advisories) to confirm it is affected by
   [CVE-ID].
3. (If in scope and authorized) demonstrate the vulnerability using a
   public or custom proof-of-concept, and capture evidence of the impact
   achieved.]

## Remediation

> [!REX-STUB]
[Upgrade the affected component to the latest patched version as soon as
possible. Establish a regular patch-management cadence and subscribe to the
vendor's security advisory feed. Where immediate patching isn't feasible,
apply any vendor-published mitigations or compensating controls (e.g. WAF
rules) as an interim measure.]
