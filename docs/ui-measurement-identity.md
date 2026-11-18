# Account, authentication, and public measurement evidence

Issue #449 covers account, authentication, public share and consent screens.
This slice uses the [common measurement method](ui-measurement.md); it sets no
performance budget. The identity runner uses the shared isolated SQLite fixture
and Playwright Chrome setup:

```sh
npm ci
UI_MEASURE=identity ./scripts/playwright-smoke.sh
```

`UI_MEASURE=1` still measures workspace-home navigation. The identity command
measures cold and warm setup, login, recovery, account, security, sessions,
notifications and unavailable-share navigation. It also measures invalid-login
and recovery feedback plus notification SSE connection state. Output contains
aggregate counters and environment metadata, not URLs, bodies, credentials or
headers. The regular Playwright journey suite checks observable behavior, not
performance.

## Reproducible isolated run

`UI_MEASURE=identity ./scripts/playwright-smoke.sh` completed on
2026-11-18T02:00:32.471Z. The runner uses the existing fresh SQLite fixture
and reports the checked-out commit, environment, cold/warm navigation counters,
interaction counters and timing values. This run used commit
`b85c9dcc834c23012bf41bd8737e768ebd35284a` as reported by the runner, Darwin
27.2.0 arm64, Node 26.10.0, Chrome 157.0.8103.79 and Playwright 1.64.1.
Viewport was 1280×900, locale `en-US`, light theme, reduced motion and
unthrottled loopback. Values are single observations; no budgets follow.

| Interaction | Status/state | Requests | Encoded bytes | Feedback/connect ms |
|---|---:|---:|---:|---:|
| Invalid login | 401 | 4 | 95744 | 140.6 |
| Password recovery request | 200 | 4 | 95187 | 255.4 |
| Notification SSE connection | open, 0 events | unavailable | unavailable | 27.1 |

The runner measured cold and warm navigation for setup, login, password
recovery, account, security, sessions, notifications and unavailable public
share. Each captured the Navigation Timing DOMContentLoaded and load values,
request count and completed encoded bytes. Notifications had one unfinished
request, its SSE connection. Cold pages reported the favicon 404; the public
share route also returned the expected document 404. Invalid login retained
the form and showed its accessible alert. Recovery showed its generic status;
email delivery was disabled. The runner emits no submitted values.

## Earlier isolated run

On 2026-11-17T20:43:26.714Z, commit `f2abeb30ffa537b9fa0255e04239499801c090e0`,
Darwin 27.2.0 arm64, Node 26.10.0, Chrome 157.0.8103.79, and Playwright 1.64.1,
the command completed using the fixture and browser conditions above. Counts
and timings remain single unthrottled observations; timing values are
milliseconds. This run supersedes the navigation-only values below for screens
it covers. It captured login feedback at 177.6 ms and password recovery success
feedback at 189.2 ms.

| Interaction | Status | Requests | Encoded bytes | Feedback ms | HTTP errors |
|---|---:|---:|---:|---:|---|
| Invalid login | 401 | 4 | 95744 | 177.6 | One expected invalid-login response |
| Password recovery request | 200 | 4 | 95187 | 189.2 | None |

Invalid login retained the form and displayed its accessible error alert. The
recovery request displayed the generic success status; email delivery was
disabled. The runner prints no submitted values. The cold favicon returned 404
in the same isolated run; the invalid-login submission itself returned 401 as
expected.

## Earlier navigation sample

The navigation sample from 2026-11-17T18:47:49.456Z used base commit
`1c2ff7f59d106988976daf1370e52463f60d0896`, Darwin 27.2.0 arm64, Node
26.10.0, Chrome 157.0.8103.79, and Playwright 1.64.1. The fixture used one
synthetic owner and workspace. Counts are one unthrottled observation; timings
are Navigation Timing milliseconds. The share route intentionally returned
404 for its invalid token. Cold means the HTTP cache was cleared before
navigation; warm means immediate repeat in the same context. Setup uses its own
isolated server and database.

| Screen | Requests cold/warm | Encoded bytes cold/warm | Load ms cold/warm |
|---|---:|---:|---:|
| Setup | 5 / 4 | 97108 / 96596 | 29.2 / 13.5 |
| Login | 5 / 4 | 96251 / 95739 | 28.2 / 13.3 |
| Password recovery | 5 / 4 | 96000 / 95488 | 12.0 / 10.7 |
| Account | 4 / 3 | 63247 / 62735 | 17.0 / 12.2 |
| Security | 5 / 4 | 66610 / 66098 | 294.9 / 10.7 |
| Sessions | 6 / 5 | 114235 / 113723 | 13.7 / 13.3 |
| Notifications | 7 / 6 | 98665 / 98153 | 14.3 / 13.2 |
| Public share unavailable | 4 / 3 | 58246 / 57734 | 13.1 / 11.1 |

All cold navigations also recorded the favicon HTTP 404. The unavailable share
document returned 404 on both runs. Other page navigations had no HTTP errors,
redirects, or transport failures. Each navigation recorded zero cache hits.
Notifications had one unfinished request, its SSE connection; it opened 14.4 ms
after the separate connection timer began and delivered zero events. Security's
cold timing was 294.9 ms; this single result needs repeat runs before attribution.

Notifications navigation includes its long-lived SSE request in `unfinished`;
that count is not a failed load. A separate observation records stream open or
closed state, time to that state, and event count during the run. The fixture
starts with no notifications, so it does not measure event delivery latency.
The browser's loading and failure feedback is not timed by this harness.

| Screen or state | Evidence | Unmeasured |
|---|---|---|
| Setup | Cold/warm page navigation | Submission, validation, unavailable setup, and completion |
| Login | Cold/warm navigation; invalid submission status, request metrics, and accessible error feedback time | Valid submission, MFA challenge, OIDC, and other auth failures |
| Password recovery | Cold/warm navigation; synthetic owner request status, request metrics, and generic success feedback time with delivery disabled | Delivery, reset completion, invalid/expired token, and failures |
| Account | Cold/warm authenticated navigation | Profile/password/email changes, validation, and deletion confirmation |
| Security | Cold/warm authenticated navigation | MFA enrollment/removal, OIDC link, reauthentication, and errors |
| Sessions | Cold/warm authenticated navigation | Revocation interaction, expired/revoked rows, and errors |
| Notifications | Cold/warm empty inbox navigation and SSE connection state | Event delivery, unread update, replay, reconnect, loading feedback, and failures |
| Public share | Cold/warm unavailable-token navigation | Valid, expired, revoked, rate-limited, and failure states |
| MCP consent | None | Authenticated consent page, approve/deny, and failures |

Issue #449 remains partial. The harness does not create MFA factors, a
registered MCP client, or a valid public share token. Browser tests cover many
states and security rules, but are not performance measurements. The remaining
measurement work maps to these route issues:

| Gap | Owning route issue |
|---|---|
| Login MFA challenge and auth failures | [#420](https://github.com/dynamiscode/dynamis-code-apps-template/issues/420) |
| MFA enrollment, removal, reauthentication, and linked identity | [#444](https://github.com/dynamiscode/dynamis-code-apps-template/issues/444) |
| Account changes, validation, and deletion | [#443](https://github.com/dynamiscode/dynamis-code-apps-template/issues/443) |
| Session revocation and expired/revoked states | [#445](https://github.com/dynamiscode/dynamis-code-apps-template/issues/445) |
| Notification delivery, replay, reconnect, and failure feedback | [#429](https://github.com/dynamiscode/dynamis-code-apps-template/issues/429) |
| Valid, expired, and revoked public share states | [#430](https://github.com/dynamiscode/dynamis-code-apps-template/issues/430) |
| MCP consent approve/deny and failures | [#430](https://github.com/dynamiscode/dynamis-code-apps-template/issues/430) |

Add isolated synthetic state setup before measuring these interactions. Map
any product defects to their owning route issue. Do not infer budgets or claim
full #439 coverage from this slice.
