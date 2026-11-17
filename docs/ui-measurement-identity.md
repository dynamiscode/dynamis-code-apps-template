# Account, authentication, and public measurement evidence

Issue #449 covers account, authentication, public share and consent screens.
This slice uses the [common measurement method](ui-measurement.md); it sets no
performance budget. Run the isolated synthetic fixture from the repository root:

```sh
npm ci
UI_MEASURE_IDENTITY=1 ./scripts/playwright-smoke.sh
```

The command uses fresh SQLite databases, synthetic owner credentials, loopback
servers, Chrome 1280×900 at device scale 1, `en-US`, light theme, reduced motion,
and unthrottled local requests. It reports one cold and one warm navigation for
setup, login, password recovery, account, security, sessions, notifications,
and the unavailable public-share page. It reports request counts, redirects,
completed encoded bytes, unfinished requests, HTTP errors by safe class,
transport failures, cache events, DOMContentLoaded and load timings. These are
single observations; automation overhead and host load are not controlled.
The output excludes URLs, headers, bodies, cookies, credentials and tokens.

## Single isolated run

On 2026-11-17T18:47:49.456Z, base commit
`1c2ff7f59d106988976daf1370e52463f60d0896`, Darwin 27.2.0 arm64, Node
26.10.0, Chrome 157.0.8103.79, and Playwright 1.64.1, the command above
completed. The fixture used one synthetic owner and workspace. Counts are one
unthrottled observation; timings are Navigation Timing milliseconds. The share
route intentionally returned 404 for its invalid token. Cold means the HTTP
cache was cleared before navigation; warm means immediate repeat in the same
context. Setup uses its own isolated server and database.

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
| Login | Cold/warm page navigation | Valid/invalid submission, MFA challenge, OIDC, and auth failures |
| Password recovery | Cold/warm request page navigation | Submit, delivery, reset completion, invalid/expired token, and failures |
| Account | Cold/warm authenticated navigation | Profile/password/email changes, validation, and deletion confirmation |
| Security | Cold/warm authenticated navigation | MFA enrollment/removal, OIDC link, reauthentication, and errors |
| Sessions | Cold/warm authenticated navigation | Revocation interaction, expired/revoked rows, and errors |
| Notifications | Cold/warm empty inbox navigation and SSE connection state | Event delivery, unread update, replay, reconnect, loading feedback, and failures |
| Public share | Cold/warm unavailable-token navigation | Valid, expired, revoked, rate-limited, and failure states |
| MCP consent | None | Authenticated consent page, approve/deny, and failures |

Issue #449 remains partial. The harness does not create MFA factors, a registered
MCP client, or a valid public share token. Browser tests cover many states and
security rules, but are not performance measurements. Add isolated synthetic
state setup before measuring those interactions; map any product defects to
their owning route issue. Do not infer budgets or claim full #439 coverage from
this slice.
