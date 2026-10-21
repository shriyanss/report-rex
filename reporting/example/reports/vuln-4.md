---
title: "Lorem Ipsum Finding — Missing HTTP Security Headers"
cvss_vector: "CVSS:3.1/AV:N/AC:H/PR:N/UI:R/S:U/C:L/I:N/A:N"
status: "Open"
component: "Web Server (nginx)"
cwe: "CWE-693"
owasp: "A05:2021"
mitre: "T1189"
mitigations: "M1050"
impact: "Low"
sophistication: "High"
material_impact: "Business Interruption"
business_risk:
  - Operational
affected_urls:
  - "https://portal.loremipsum.example"
timeline:
  - date: "1970-01-01T15:45:00+00:00"
    description: "Lorem ipsum dolor sit amet — confirmed several recommended HTTP security response headers are missing from the web server"
    include: true
---

## Description

Lorem ipsum dolor sit amet, consectetur adipiscing elit. The application does not set several recommended HTTP security response headers, increasing the attack surface for certain browser-based attacks:

| Header | Expected | Actual |
|--------|----------|--------|
| `Content-Security-Policy` | Restrictive policy | Not set |
| `X-Frame-Options` | `DENY` or `SAMEORIGIN` | Not set |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | Not set |
| `X-Content-Type-Options` | `nosniff` | Not set |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Not set |
| `Permissions-Policy` | Restrictive policy | Not set |

## Impact

Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. The missing headers individually provide limited impact but collectively increase exposure, ut enim ad minim veniam, quis nostrud exercitation ullamco laboris.

## Replication

Lorem ipsum dolor sit amet, run the following command against the placeholder host to confirm no security headers are present:

```bash
curl -I https://portal.loremipsum.example/ | grep -iE 'content-security|x-frame|strict-transport|x-content-type|referrer|permissions'
# (no output — none of the headers are present)
```

![Lorem ipsum placeholder screenshot for this finding](../assets/screenshots/missing-headers.png){width=90%}

## Remediation

Lorem ipsum dolor sit amet, add the following headers in your web server or application middleware:

**Nginx:**

```nginx
add_header Content-Security-Policy "default-src 'self'; script-src 'self'; object-src 'none';" always;
add_header X-Frame-Options "DENY" always;
add_header Strict-Transport-Security "max-age=31536000; includeSubDomains; preload" always;
add_header X-Content-Type-Options "nosniff" always;
add_header Referrer-Policy "strict-origin-when-cross-origin" always;
add_header Permissions-Policy "geolocation=(), microphone=(), camera=()" always;
```

Consectetur adipiscing elit: the CSP header requires careful tuning to avoid breaking application functionality. Start with report-only mode (`Content-Security-Policy-Report-Only`) before enforcing.
