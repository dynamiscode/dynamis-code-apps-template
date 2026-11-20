# Remote CLI (Agent profile)

This command is included only when the application is generated with the Agent
profile. Core and Identity applications do not include `cmd/appctl`.

`appctl` is a REST-only client for the versioned item, client, and client
custom field APIs. It has no database, shell, or local application-service
dependency. Its only internal import is the short-ID codec, and a test fails if
it imports anything else from the application.

`--workspace` takes the workspace short ID: 4 to 32 characters from
`23456789abcdefghjkmnpqrstuvwxyz`. The CLI checks only that shape, because the
value becomes a URL path segment. A malformed or unknown value gets the same
`not-found` from the server as a missing workspace. `--item` takes the item
short ID with the same shape check and the same `not-found` rule. A value in the
retired 32-character hex form fails the server's strict decode and gets that
same `not-found`.

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

go run ./cmd/appctl clients create --workspace "$WORKSPACE_ID" \
  --display-name 'Acme' --email ops@example.com
go run ./cmd/appctl clients list --workspace "$WORKSPACE_ID" \
  --archived all --sort displayName --filter 'displayName:contains:Acme'
go run ./cmd/appctl clients show --workspace "$WORKSPACE_ID" --client "$CLIENT_ID"
go run ./cmd/appctl clients edit --workspace "$WORKSPACE_ID" \
  --client "$CLIENT_ID" --version 1 --set-display-name 'Acme Corp' --set-email ''
go run ./cmd/appctl clients archive --workspace "$WORKSPACE_ID" --client "$CLIENT_ID"
go run ./cmd/appctl clients unarchive --workspace "$WORKSPACE_ID" --client "$CLIENT_ID"
go run ./cmd/appctl fields define --workspace "$WORKSPACE_ID" \
  --label 'Source' --type text
go run ./cmd/appctl fields archive --workspace "$WORKSPACE_ID" --field "$FIELD_ID"
```

`clients` commands are `list`, `show`, `create`, `edit`, `archive`, and
`unarchive`. `fields` commands are `define` and `archive`; `fields define`
takes `--type` of `text`, `number`, `date`, `yes_no`, or `single_choice`.
`clients list --archived` takes `active` (the default), `archived`, or `all`.
`clients list --filter` may repeat, and each value uses the REST condition
syntax. `clients edit` takes `--version` from the client's last response and
sends it as `If-Match`; a `--set-` flag with an empty value clears that field.
Field values, options, restore, and permanent delete are not in `appctl` yet.

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

List filters, pages, ETags, idempotency, scopes, and public errors retain the
REST behavior documented in [HTTP and REST API](api.md).
`items list --search TERM` uses the same bounded, case-insensitive literal title
search as REST.

## Local demo seed

`go run ./cmd/demo` is a local operator command for making a generated
application testable immediately. Set `DEMO_OWNER_PASSWORD` explicitly; it
creates or reuses `Demo Workspace` and inserts three deterministic items using
scoped idempotency keys. It prints only workspace and item IDs. It operates on
the configured database directly and is not a production data migration tool.
