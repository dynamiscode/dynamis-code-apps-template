# Remote CLI

`appctl` is a REST-only client for the versioned item API. It has no database,
shell, or local application-service dependency.

Configure a remote origin and token through environment variables. Prefer the
environment for the token because command-line values may appear in process
listings.

```sh
export APPCTL_BASE_URL=http://127.0.0.1:8080
export APPCTL_TOKEN='<token shown once by token creation>'

go run ./cmd/appctl items list --workspace "$WORKSPACE_ID"
go run ./cmd/appctl items get --workspace "$WORKSPACE_ID" --item "$ITEM_ID"
go run ./cmd/appctl items create --workspace "$WORKSPACE_ID" \
  --title 'Example' --idempotency-key 'caller-unique-key'
go run ./cmd/appctl items update --workspace "$WORKSPACE_ID" \
  --item "$ITEM_ID" --version 1 --set-status complete
go run ./cmd/appctl items delete --workspace "$WORKSPACE_ID" \
  --item "$ITEM_ID" --version 2
```

`APPCTL_BASE_URL` defaults to `http://127.0.0.1:8080`.
`APPCTL_TIMEOUT` defaults to `30s` and must be from `1s` to `5m`. Equivalent
`--base-url`, `--token`, and `--timeout` flags are accepted after the command.
Redirects are not followed, responses are capped at 1 MiB, successful results
are JSON on stdout, and errors are JSON on stderr.

| Exit | Meaning |
|---:|---|
| `0` | success |
| `1` | network, server, redirect, or invalid response failure |
| `2` | usage or local configuration error |
| `3` | authentication or authorization failure |
| `4` | request, not-found, conflict, or precondition failure |
| `5` | rate limited |

List filters, cursors, ETags, idempotency, scopes, and public errors retain the
REST behavior documented in [HTTP and REST API](api.md).
