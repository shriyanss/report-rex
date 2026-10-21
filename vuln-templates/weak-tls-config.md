---
# > [!REX-STUB]
title: "Weak TLS/SSL Configuration on [SERVICE NAME]"
# > [!REX-STUB]
cvss_vector: "CVSS:3.1/AV:N/AC:H/PR:N/UI:N/S:U/C:L/I:N/A:N"
status: "Open"
# > [!REX-STUB]
component: "[COMPONENT — e.g. Web server TLS listener]"
cwe: "CWE-326"
owasp: "A02:2021"
mitre: "T1040"
mitigations: "M1041"
# > [!REX-STUB]
impact: "Medium"
# > [!REX-STUB]
sophistication: "High"
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
    description: "Identified weak TLS protocol/cipher support during a TLS configuration scan"
    include: true
---

## Description

> [!REX-STUB]
[The TLS/SSL configuration on this service permits outdated protocol
versions (e.g. TLS 1.0/1.1, SSLv3) and/or weak cipher suites (e.g. those
using RC4, 3DES, or export-grade ciphers), and/or uses a weak or expired
certificate. This weakens the confidentiality and integrity guarantees the
connection is supposed to provide.]

## Impact

> [!REX-STUB]
[Describe the practical risk — e.g. susceptibility to downgrade attacks,
known protocol-level attacks against the specific weak cipher/protocol
identified (e.g. POODLE, BEAST), or reduced assurance from an expired/
self-signed certificate that trains users to ignore browser warnings.]

## Replication

> [!REX-STUB]
[1. Run a TLS configuration scanner (e.g. `nmap --script ssl-enum-ciphers`,
   `testssl.sh`, or `sslyze`) against the target host and port.
2. Review the output for supported protocol versions below TLS 1.2 and any
   weak/export cipher suites.
3. Confirm the certificate's validity period, key strength, and signature
   algorithm.]

## Remediation

> [!REX-STUB]
[Disable SSLv3/TLS 1.0/TLS 1.1 and restrict cipher suites to modern,
forward-secrecy-supporting options (e.g. those recommended by Mozilla's TLS
configuration guidelines). Replace weak/expired certificates with a
currently-valid certificate signed with SHA-256 or stronger. Enable HSTS to
prevent protocol downgrade at the browser level.]
