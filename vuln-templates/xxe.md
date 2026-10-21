---
# > [!REX-STUB]
title: "XML External Entity (XXE) Injection in [COMPONENT]"
# > [!REX-STUB]
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:L"
status: "Open"
# > [!REX-STUB]
component: "[COMPONENT — e.g. XML file upload/import feature]"
cwe: "CWE-611"
owasp: "A05:2021"
mitre: "T1190"
mitigations: "M1042, M1048"
# > [!REX-STUB]
impact: "High"
# > [!REX-STUB]
sophistication: "Medium"
# > [!REX-STUB]
material_impact: "Proprietary Data Loss"
# > [!REX-STUB]
business_risk:
  - Operational
# > [!REX-STUB]
affected_urls:
  - "https://[TARGET]/[XML-PROCESSING-ENDPOINT]"
# > [!REX-STUB]
timeline:
  - date: "2024-01-01T00:00:00+00:00"
    description: "Achieved local file disclosure via XML external entity injection"
    include: true
---

## Description

> [!REX-STUB]
[A feature that parses XML input (a file upload, SOAP endpoint, or an XML-
based API/import feature) uses an XML parser configured to resolve external
entities. An attacker can define a custom `DOCTYPE`/entity in the submitted
XML that causes the parser to read local files, make outbound network
requests (SSRF), or exhaust resources (billion laughs / entity expansion).]

## Impact

> [!REX-STUB]
[Describe what was achieved — e.g. disclosure of local files such as
`/etc/passwd` or application configuration/credentials, internal network
reconnaissance via out-of-band entity resolution, or a denial-of-service
condition from entity expansion.]

## Replication

> [!REX-STUB]
[1. Locate an endpoint that accepts and parses XML input.
2. Submit a crafted XML document containing an external entity definition,
   e.g.:
   ```xml
   <?xml version="1.0"?>
   <!DOCTYPE foo [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
   <root>&xxe;</root>
   ```
3. Observe the parsed response (or an out-of-band callback, for blind XXE)
   containing the contents of the referenced local file.]

## Remediation

> [!REX-STUB]
[Disable DTD processing and external entity resolution in the XML parser
configuration (most parsers support this explicitly, e.g.
`FEATURE_SECURE_PROCESSING` in Java, `resolve_entities=False` in lxml, or
`libxml_disable_entity_loader` in PHP). Prefer a data format that doesn't
support entities (e.g. JSON) where XML isn't strictly required. Keep the XML
parsing library up to date.]
