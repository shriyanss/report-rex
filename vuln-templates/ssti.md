---
# > [!REX-STUB]
title: "Server-Side Template Injection (SSTI) in [COMPONENT] Leading to RCE"
# > [!REX-STUB]
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"
status: "Open"
# > [!REX-STUB]
component: "[COMPONENT — e.g. Report/PDF generation feature]"
cwe: "CWE-1336"
owasp: "A03:2021"
mitre: "T1190"
mitigations: "M1038, M1048"
# > [!REX-STUB]
impact: "Critical"
# > [!REX-STUB]
sophistication: "Medium"
# > [!REX-STUB]
material_impact: "Proprietary Data Loss"
# > [!REX-STUB]
business_risk:
  - Operational
  - Financial
# > [!REX-STUB]
affected_urls:
  - "https://[TARGET]/[TEMPLATE-RENDERING-ENDPOINT]"
# > [!REX-STUB]
timeline:
  - date: "2024-01-01T00:00:00+00:00"
    description: "Achieved remote code execution via server-side template injection"
    include: true
---

## Description

> [!REX-STUB]
[A feature that renders user-controllable input through a server-side
templating engine (e.g. Jinja2, Twig, FreeMarker, Velocity) does so without
sanitizing template syntax out of the input. An attacker can inject template
expressions that execute in the context of the templating engine, which in
many engines allows arbitrary code execution on the server.]

## Impact

> [!REX-STUB]
[Describe the impact achieved — typically full remote code execution on the
application server, allowing an attacker to read/write files, pivot to
internal systems, or exfiltrate secrets/environment variables.]

## Replication

> [!REX-STUB]
[1. Identify user input that is rendered through a server-side template
   (e.g. a "name" field reflected on a generated report/page).
2. Submit a template-syntax probe appropriate to the suspected engine, e.g.
   `{{7*7}}` (Jinja2/Twig) or `${7*7}` (FreeMarker/Velocity), and confirm
   the expression is evaluated (the response shows `49` rather than the
   literal string).
3. Escalate the payload to achieve code execution, e.g. for Jinja2:
   `{{ self.__init__.__globals__.__builtins__.__import__('os').popen('id').read() }}`.
4. Confirm command output is reflected in the response.]

## Remediation

> [!REX-STUB]
[Never render user-controllable input as a template itself — pass it as
data/context variables to a pre-defined template instead. If dynamic
templates are unavoidable, use the engine's sandboxed-execution mode and
strip/escape template syntax from user input. Run the rendering process with
minimal privileges.]
