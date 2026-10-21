---
# > [!REX-STUB]
title: "Open Redirect via [PARAMETER NAME]"
# > [!REX-STUB]
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:N/I:L/A:N"
status: "Open"
# > [!REX-STUB]
component: "[COMPONENT — e.g. Login redirect handler]"
cwe: "CWE-601"
owasp: "A01:2021"
mitre: "T1189"
mitigations: "M1054"
# > [!REX-STUB]
impact: "Low"
# > [!REX-STUB]
sophistication: "Low"
# > [!REX-STUB]
material_impact: "Reputational Damage"
# > [!REX-STUB]
business_risk:
  - Reputational
# > [!REX-STUB]
affected_urls:
  - "https://[TARGET]/[REDIRECT-ENDPOINT]?redirect=[URL]"
# > [!REX-STUB]
timeline:
  - date: "2024-01-01T00:00:00+00:00"
    description: "Identified an unvalidated redirect parameter usable for phishing"
    include: true
---

## Description

> [!REX-STUB]
[A parameter that controls a post-action redirect (e.g. after login, logout,
or an external link-out feature) accepts an arbitrary external URL without
validating that it stays within the application's own domain. An attacker
can craft a link that appears to point to the trusted application but
silently redirects the victim to an attacker-controlled site.]

## Impact

> [!REX-STUB]
[Describe the phishing/credential-harvesting risk this enables — e.g. an
attacker sending a link that looks like `https://[TARGET]/...` but redirects
to a lookalike login page, increasing the credibility of a phishing
campaign. Note any secondary abuse, such as OAuth token leakage if the
redirect target is used in an OAuth flow.]

## Replication

> [!REX-STUB]
[1. Identify a parameter that triggers a redirect (e.g. `?redirect=`,
   `?next=`, `?returnUrl=`).
2. Set the parameter to an external URL, e.g.
   `https://[TARGET]/[REDIRECT-ENDPOINT]?redirect=https://attacker.example`.
3. Visit the crafted URL and observe the browser is redirected to the
   attacker-controlled domain after passing through the trusted application
   domain.]

## Remediation

> [!REX-STUB]
[Validate redirect targets against an allow-list of relative paths or known
internal hosts. Reject any redirect target containing a scheme or host that
does not match the application's own domain. If external redirects are a
legitimate feature, interstitially warn the user they are leaving the site
and display the destination domain.]
