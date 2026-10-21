---
# > [!REX-STUB]
title: "Server-Side Request Forgery (SSRF) via [FEATURE NAME]"
# > [!REX-STUB]
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:C/C:H/I:L/A:N"
status: "Open"
# > [!REX-STUB]
component: "[COMPONENT — e.g. URL-fetching feature]"
cwe: "CWE-918"
owasp: "A10:2021"
mitre: "T1190"
mitigations: "M1030, M1042"
# > [!REX-STUB]
impact: "High"
# > [!REX-STUB]
sophistication: "Medium"
# > [!REX-STUB]
material_impact: "Proprietary Data Loss"
# > [!REX-STUB]
business_risk:
  - Operational
  - Reputational
# > [!REX-STUB]
affected_urls:
  - "https://[TARGET]/[URL-FETCHING-ENDPOINT]"
# > [!REX-STUB]
timeline:
  - date: "2024-01-01T00:00:00+00:00"
    description: "Identified server-side request forgery via a URL-fetching feature"
    include: true
---

## Description

> [!REX-STUB]
[A feature that fetches a server-provided URL on the user's behalf (e.g. a
webhook, PDF renderer, image proxy, or link preview) does not validate or
restrict the destination. An attacker can supply an internal address —
including cloud metadata endpoints, internal service ports, or the
`file://`/`gopher://` schemes — and cause the server to make requests to
attacker-chosen internal destinations.]

## Impact

> [!REX-STUB]
[Describe the internal resources reached — e.g. cloud instance metadata
service (potentially exposing IAM credentials), internal admin panels, or
other services not intended to be internet-reachable — and what an attacker
gained access to as a result.]

## Replication

> [!REX-STUB]
[1. Identify a feature that accepts a URL and fetches it server-side.
2. Submit an internal/loopback address (e.g. `http://169.254.169.254/latest/
   meta-data/`, `http://localhost:PORT/`, or an internal RFC1918 address) as
   the target URL.
3. Observe that the server issues the request and returns the response
   (fully or partially) to the client.
4. Use the response to enumerate internal services or extract sensitive
   data.]

## Remediation

> [!REX-STUB]
[Validate destination URLs against an allow-list of expected hosts/schemes
rather than a deny-list. Resolve the hostname and reject requests to
private/loopback/link-local IP ranges (including after redirects). Disable
unnecessary URL schemes. Run the fetching component with the minimum network
access necessary (network segmentation), and disable cloud metadata access
from application workloads where possible (e.g. IMDSv2 with hop-limit
restrictions).]
