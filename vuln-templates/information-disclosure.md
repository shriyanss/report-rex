---
# > [!REX-STUB]
title: "Verbose Error Messages / Information Disclosure in [COMPONENT]"
# > [!REX-STUB]
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N"
status: "Open"
# > [!REX-STUB]
component: "[COMPONENT — e.g. Error-handling middleware]"
cwe: "CWE-209"
owasp: "A05:2021"
mitre: "T1592"
mitigations: "M1047"
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
  - "https://[TARGET]/[ENDPOINT-THAT-ERRORS]"
# > [!REX-STUB]
timeline:
  - date: "2024-01-01T00:00:00+00:00"
    description: "Observed verbose stack traces / internal details in application error responses"
    include: true
---

## Description

> [!REX-STUB]
[The application returns overly verbose error messages to the client on
unhandled exceptions, malformed input, or invalid requests — including
stack traces, internal file paths, database query fragments, framework
version numbers, or other internal implementation details.]

## Impact

> [!REX-STUB]
[Describe how this information aids an attacker — e.g. framework/library
version numbers narrowing down applicable known exploits, internal file
paths revealing server structure, or database error fragments hinting at
injection points.]

## Replication

> [!REX-STUB]
[1. Submit malformed, unexpected, or boundary-case input to an endpoint
   (e.g. an invalid data type, an unescaped special character, or a request
   to a resource that doesn't exist).
2. Observe the application's error response, noting any stack trace,
   internal path, or framework/version disclosure.
3. Repeat across a few endpoints/input types to confirm the behavior is
   systemic rather than a one-off.]

## Remediation

> [!REX-STUB]
[Return generic, user-facing error messages in production and log detailed
diagnostic information server-side only. Disable framework debug/development
modes in production. Remove or suppress version-identifying response
headers and banners where feasible.]
