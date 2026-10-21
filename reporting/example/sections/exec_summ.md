---
title: "Executive Summary"
---

Lorem ipsum dolor sit amet, consectetur adipiscing elit. Lorem Ipsum Holdings engaged Report Rex to conduct a web application penetration test against their placeholder customer portal at `portal.loremipsum.example` between two placeholder dates. The assessment identified **:::findings:total::: vulnerabilities**: :::findings:critical::: Critical, :::findings:high::: High, :::findings:medium::: Medium and :::findings:low::: Low.

## Findings at a Glance

Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. The chart below summarises the findings by severity. Severity is derived automatically from each finding's CVSS v3.1 vector.

:::findings_graph:donut:severity:::

Ut enim ad minim veniam, the findings map to the following OWASP Top 10 (2021) categories:

:::findings_graph:matrix:owasp:::

See [Appendix B — Findings Coverage](#appendix-b-findings-coverage) for the full OWASP, MITRE ATT&CK and business-risk breakdown.

## Business Risk

Quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Each finding is tagged with the business-risk categories it affects. The distribution across the engagement is shown below.

:::findings_graph:radar:business:::

## Overall Risk Rating

The overall risk rating is **:::findings:highest:::**, the highest severity among the findings. Duis aute irure dolor in reprehenderit in voluptate velit esse: remediate the highest-rated findings before the portal can be considered safe for production use.

## Recommendations Summary

1. Lorem ipsum: parameterize all database queries to eliminate SQL Injection — see [the SQL Injection finding](./vuln-1.md).
2. Dolor sit amet: sanitize user input and set `HttpOnly` cookies to prevent Stored XSS — see [the XSS finding](./vuln-2.md).
3. Consectetur adipiscing: enforce server-side ownership checks on all object access to fix the IDOR — see [the IDOR finding](./vuln-3.md).
4. Sed do eiusmod: configure the recommended HTTP security headers across all responses — see [the headers finding](./vuln-4.md).
