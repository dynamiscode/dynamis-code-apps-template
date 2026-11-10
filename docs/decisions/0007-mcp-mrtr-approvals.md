# 0007: MCP Multi Round-Trip Approvals

- Status: accepted
- Date: 2026-11-10
- Owners: template maintainers

## Context

`items_update_v1` and `items_delete_v1` required the client host to send
`Mcp-Human-Approval: true`. That header is client-asserted: the server cannot
tell whether a human confirmed the call, and it never authorized anything by
itself. Bearer scopes and item authorization remain the only server-side
boundary.

The MCP `2026-07-28` revision and the official Go SDK v1.8.0 provide multi
round-trip requests (MRTR, SEP-2322). A tool handler returns `input_required`
with `inputRequests` and `requestState`; the client fulfills the requests and
retries the same call with `inputResponses` and the echoed state. The endpoint
is stateless and never issues `Mcp-Session-Id`, so the server must not hold
pending approvals between rounds.

The SDK's server-side MRTR middleware, for pre-`2026-07-28` sessions, would
call `elicitation/create` from server to client. A stateless HTTP endpoint
cannot deliver that request, so such sessions cannot confirm through MRTR. No
released clients depend on the header, so it is removed rather than kept as a
fallback.

## Decision

- Update and delete return an `input_required` result with one input request,
  `human-approval`: a form elicitation with a boolean `confirm` field. The
  message names the item and version, never the title.
- `requestState` is the SHA-256 of the tool name and canonical arguments. A
  retry proceeds only when the state matches the echoed value and the
  `human-approval` response is `accept` with `confirm: true`. Any other
  response returns `approval_declined`. Changed arguments or a missing or
  mismatched state prompt again. No mutation occurs before a matching accepted
  response.
- `Mcp-Human-Approval` is removed. A header never approves a call.
- The state is not a signed token and needs no server secret. It binds a retry
  to the arguments that were prompted; it does not prove a human acted. Like
  the header before it, approval is a client-host confirmation and never
  replaces scope or workspace authorization.
- Prompting rounds are not audited, because they do not change state. Each
  final outcome (success, `approval_declined`, or a failure) is audited as
  before.

## Consequences

- Stateless: any instance can verify a retry from the request and echoed state.
  No session store, cache, or broker is added.
- Pre-`2026-07-28` protocol requests cannot update or delete items. Their
  attempt returns a protocol error, and no item changes.
- Replay: an accepted response can be reused for the same arguments by any
  client that holds it. Accepted limitation, the same as the header. Server
  authorization still decides every mutation.
- Rejected: keeping the header as a fallback, which leaves the weaker gate
  available; a server-side pending-approval store, which breaks the stateless
  invariant; SDK-automatic server-initiated elicitation for older sessions,
  which a stateless endpoint cannot deliver; a signed request state, which needs
  a new secret and buys nothing while approval is not authorization.

## Revisit when

- A deployment needs server-verifiable proof of a human decision. Then add a
  signed approval token or an out-of-band confirmation path with its own record.
- The SDK changes how MRTR retries or server-initiated requests behave.
