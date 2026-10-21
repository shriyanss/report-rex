## Summary

Lorem ipsum dolor sit amet, consectetur adipiscing elit. During today's testing session (Day 1), a **Critical** severity SQL Injection vulnerability was identified in the primary authentication endpoint. This notification is sent as an out-of-band alert per our rules of engagement for critical findings.

::: {.vulnerability severity="Critical" title="Lorem Ipsum Finding — SQL Injection in /api/login" cvss="9.8" status="Open" affected="https://portal.loremipsum.example/api/login"}
:::

## What Was Found

Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. The `username` parameter in the login endpoint is concatenated directly into the SQL query without sanitization. An unauthenticated attacker can:

- **Bypass authentication** entirely by submitting `' OR 1=1--`
- **Enumerate all user accounts** via `UNION SELECT` injection
- **Potentially achieve Remote Code Execution** if `xp_cmdshell` is enabled (MSSQL)

## Immediate Actions Requested

Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat:

1. **Apply a temporary WAF rule** blocking SQL meta-characters (`'`, `"`, `--`, `;`) in the `username` field.
2. **Audit authentication logs** for the past 30 days for anomalous login patterns.
3. **Alert your DBA** to review for signs of data exfiltration.
4. **Schedule a remediation call** — our team is available within the hour.

## Not in Scope Yet

Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Today's testing covered only the login flow. We have not yet tested the authenticated application.

## Next Steps

Excepteur sint occaecat cupidatat non proident: the complete technical write-up with proof-of-concept commands, screenshots, and detailed remediation guidance will appear in the final report.
