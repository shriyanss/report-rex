---
# > [!REX-STUB]
title: "Missing Rate Limiting on [ENDPOINT NAME] Enables Brute Force"
# > [!REX-STUB]
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:L/I:N/A:N"
status: "Open"
# > [!REX-STUB]
component: "[COMPONENT — e.g. Login / password-reset endpoint]"
cwe: "CWE-307"
owasp: "A07:2021"
mitre: "T1110"
mitigations: "M1036, M1032"
# > [!REX-STUB]
impact: "Medium"
# > [!REX-STUB]
sophistication: "Low"
# > [!REX-STUB]
material_impact: "Unauthorized Account Access"
# > [!REX-STUB]
business_risk:
  - Operational
# > [!REX-STUB]
affected_urls:
  - "https://[TARGET]/[LOGIN-OR-OTP-ENDPOINT]"
# > [!REX-STUB]
timeline:
  - date: "2024-01-01T00:00:00+00:00"
    description: "Confirmed no rate limiting or lockout on repeated failed authentication attempts"
    include: true
---

## Description

> [!REX-STUB]
[An authentication-related endpoint (login, password reset, OTP/MFA
verification, or an API key check) does not enforce rate limiting or account
lockout after repeated failed attempts. An attacker can submit an unlimited
number of guesses in an automated fashion.]

## Impact

> [!REX-STUB]
[Describe the practical risk — e.g. credential-stuffing or brute-force
attacks becoming feasible against user accounts, or an OTP/short numeric
code becoming guessable within a realistic time window given no attempt
limit.]

## Replication

> [!REX-STUB]
[1. Submit a request to the target endpoint with a deliberately incorrect
   credential/code.
2. Repeat the request rapidly (e.g. via Burp Intruder or a simple script)
   dozens or hundreds of times in succession.
3. Confirm that none of the requests are throttled, blocked, or trigger a
   CAPTCHA/lockout — every attempt returns a normal response.]

## Remediation

> [!REX-STUB]
[Implement rate limiting (per-IP and per-account) on authentication-related
endpoints, with progressive delays or temporary lockout after a threshold of
failed attempts. Add CAPTCHA or similar friction after repeated failures.
Alert on/monitor for high-volume authentication failure patterns.]
