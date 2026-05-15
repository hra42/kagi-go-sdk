# Security Policy

## Supported versions

`kagi-go-sdk` is pre-1.0 and ships from `main`. Security fixes land on `main` and in the next tagged release; older tags are not patched.

| Version | Supported |
|---|---|
| `main` / latest tag | Yes |
| Older tags | No — please update |

## Reporting a vulnerability

**Please do not open a public GitHub issue for security reports.**

Report vulnerabilities privately through GitHub's security advisory flow:

> <https://github.com/hra42/kagi-go-sdk/security/advisories/new>

This keeps the report private and lets us coordinate a fix and disclosure in-thread.

Please include:

- A description of the issue and its impact.
- Steps to reproduce (a minimal Go program is ideal).
- The affected commit SHA or tag.
- Your Go version and OS, if relevant.

You can expect an initial acknowledgement within a few days. Once a fix is ready we will coordinate a disclosure timeline with you and credit you in the advisory unless you prefer otherwise.

## Scope

In scope:

- Vulnerabilities in the SDK code itself — request construction, response parsing, retry/transport behavior, error handling.
- Issues that could cause the SDK to leak credentials, mishandle TLS, or be coerced into requests against unintended hosts.

Out of scope (please report directly to Kagi at <https://kagi.com/contact>):

- Vulnerabilities in the Kagi Search API service.
- Account-level issues, billing, or abuse on `kagi.com`.

## Dependencies

The SDK depends on the Go standard library only. There is no third-party dependency surface to audit; `go.mod` has no `require` entries beyond the Go version itself.
