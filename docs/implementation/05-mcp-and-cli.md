# Phase 05: MCP and CLI

## Goal

Expose safe agent and automation surfaces without creating alternate business
logic or storage paths.

## Standards covered

`STANDARDS.md` Section 3 CLI requirements, Section 9 MCP requirements,
applicable Section 7 changes, and Agent acceptance in Section 13.

## Prerequisites

Identity scopes, REST authentication, public error behavior, and one sample
feature use case are complete.

## Required outcomes

- Expose authenticated MCP Streamable HTTP using shared scoped authorization
  and application use cases.
- Define deterministic tool names, versions, bounded structured inputs and
  outputs, pagination, and read-only/destructive/idempotent/open-world safety
  annotations.
- Enforce exact configured Origins when present, safe local binding defaults,
  principal-bound sessions when state is required, resource limits, and
  consistent HTTP/MCP error mapping.
- Treat tool input, retrieved content, prompts, and model output as untrusted.
  Prevent policy changes, self-approval, data exfiltration, SSRF, arbitrary
  SQL, shell, filesystem, or code execution.
- Require human approval signals for destructive tools while retaining server
  authorization.
- Keep server MCP persistent, bearer-authenticated, and authoritative. WebMCP
  is a separate optional browser-tab enhancement using the live session and
  DOM; it does not change MCP scopes, tools, transport, or CLI behavior.
- Audit tool calls with principal, workspace, credential/client reference,
  tool/version, trace/request ID, target, annotations, outcome, duration, and
  redacted error category.
- Build `appctl` as a remote REST client with bounded timeouts,
  machine-readable output, stdout/stderr separation, documented exit statuses,
  and credential-safe configuration/errors.
- Create final MCP and CLI documents after behavior exists.

## Evidence

- MCP component tests cover initialization, authentication, scopes, Origins,
  sessions, schemas, annotations, approval, bounds, errors, redaction, and
  wrong-workspace access.
- CLI integration tests run against the HTTP server and prove no storage or
  database dependency enters the CLI.
- Source/dependency checks confirm MCP and CLI contain no direct database or
  arbitrary shell path.
- Live smoke covers token creation, REST, CLI, MCP initialize, and one tool
  call through the sample use case.
- Full Go test, vet, and race gates pass.

## Exclusions

No A2A, internal AI, MCP database/shell tools, delegated OAuth server, dynamic
client registration, or long-running MCP task unless a real tool requires it.

## Completion gate

REST, MCP, and CLI produce consistent authorized behavior through shared use
cases, and every Agent profile evidence row is linked and passing.
