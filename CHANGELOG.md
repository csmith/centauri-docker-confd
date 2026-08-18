# Changelog

## Unreleased

- The `com.chameth.errors.<status>` label can now be used to map an error status
  code to an upstream that should generate the response for it (emitted as an
  `on_error` directive).

## 1.4.0 - 2026-06-18

- The `com.chameth.subject` label can now be used to specify the certificate
  subject name for a particular route.

## 1.3.0 - 2026-06-16

- The `com.chameth.provider` label can now be used to specify the certificate
  provider for a particular route.
- A warning is now logged if multiple containers define a route with different
  alternate names, or different providers.

## 1.2.0 - 2026-04-22

- Log warning if an empty config is generated
- Log warning if a client connects when no config is available
- Add more detail to debug logs
- If no valid containers are found, the empty config is now sent to Centauri.

## 1.1.0 - 2026-04-04

- Dependency updates

## 1.0.0 - 2025-12-21

_Initial version._
