---
# > [!REX-STUB]
title: "Insecure / Unrestricted File Upload in [FEATURE NAME]"
# > [!REX-STUB]
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:H/A:H"
status: "Open"
# > [!REX-STUB]
component: "[COMPONENT — e.g. Avatar/document upload feature]"
cwe: "CWE-434"
owasp: "A04:2021"
mitre: "T1190"
mitigations: "M1038, M1042"
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
  - "https://[TARGET]/[UPLOAD-ENDPOINT]"
# > [!REX-STUB]
timeline:
  - date: "2024-01-01T00:00:00+00:00"
    description: "Uploaded a malicious file and achieved code execution / stored an unsafe payload"
    include: true
---

## Description

> [!REX-STUB]
[A file-upload feature does not adequately validate the type, content, or
storage location of uploaded files. An attacker can upload a file with an
executable extension (or one whose content differs from its declared MIME
type) to a location the web server will execute, or store a file that is
unsafe when later served to other users.]

## Impact

> [!REX-STUB]
[Describe the resulting impact — e.g. remote code execution via an uploaded
web shell, stored XSS via an uploaded HTML/SVG file served without a safe
`Content-Type`, or denial of service via unrestricted file size.]

## Replication

> [!REX-STUB]
[1. Locate the file-upload feature and identify what file types/extensions
   the client-side validation permits.
2. Craft a file with a disallowed extension or malicious content (e.g. a
   web shell with a `.php`/`.jsp`/`.asp` extension, or content-type
   mismatch) and upload it, bypassing client-side checks by intercepting
   the request with a proxy.
3. Determine the uploaded file's storage path and access it directly.
4. Confirm the file executes/renders as uploaded, demonstrating the
   resulting impact (e.g. command execution via the web shell).]

## Remediation

> [!REX-STUB]
[Validate file type server-side using content inspection (magic bytes), not
just the file extension or client-supplied MIME type. Store uploads outside
the webroot, or in a location configured to never execute scripts.
Randomize stored filenames. Enforce a maximum file size and, where
applicable, scan uploads for malware. Serve user-uploaded content from a
separate, cookie-less domain to reduce the impact of stored XSS.]
