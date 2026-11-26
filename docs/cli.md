# Remote CLI (Agent profile)

This command is included when the Clients profile is selected. Clients requires
the Agent profile. Generated applications without Clients omit `cmd/appctl`
because no supported CLI commands remain.

`appctl` is a REST-only client for the versioned client and client
custom field APIs. It has no database, shell, or local application-service
dependency. Its only internal import is the short-ID codec, and a test fails if
it imports anything else from the application.

`--workspace` takes the workspace short ID: 4 to 32 characters from
`23456789abcdefghjkmnpqrstuvwxyz`. The CLI checks only that shape, because the
value becomes a URL path segment. A malformed or unknown value gets the same
`not-found` from the server as a missing workspace.

Configure a remote origin and token through environment variables. Prefer the
environment for the token because command-line values may appear in process
listings.

```sh
export APPCTL_BASE_URL=http://127.0.0.1:8080
export APPCTL_TOKEN='<token shown once by token creation>'

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

Transactions supports `categories`, `category-create --name`, `category-rename
--category --name`, and `category-archive --category`. `transactions create` accepts
`--category` for money-out entries. These commands use the REST API.

Transactions methods use `appctl transactions methods --workspace "$WORKSPACE_ID"` to list.
Create with `appctl transactions method-create --workspace "$WORKSPACE_ID" --name 'Wire'`.
Rename or change state with `appctl transactions method-update --workspace "$WORKSPACE_ID" --id "$METHOD_ID" --name 'Bank wire' --enabled=false`, using the ID returned by creation or listing.
Updates require the current name and explicit enabled state. IDs stay stable after renaming. Owners and admins see disabled methods and may manage them; members see enabled methods only.

`transactions list`, `transactions methods`, and `transactions create` use the transactions REST API.
`transactions create` requires `--description`, a nonblank title of at most
200 Unicode characters after trimming; `--note` remains optional. Responses
include `description`, which is empty for historical entries without one.
`transactions list` accepts `--client`, `--category`, `--payment-method`,
`--direction` (`in` or `out`), `--from`, and `--to`; filters combine with AND.
Dates are inclusive `YYYY-MM-DD` bounds. Use `--page` for 1-based pages of
100 entries. Viewers cannot read transactions entries.
To void an entry, use `transactions void --workspace "$WORKSPACE_ID" --entry
"$ENTRY_ID" --reason "Correction"`. To create a linked replacement, pass
`--replaces-entry "$ENTRY_ID"` to `transactions create`. For a money-out refund, use
`--direction out --refund-of-entry "$ENTRY_ID"` with the usual creation flags.
Owners and admins can void or replace entries; members can also record refunds.
Refunds cannot exceed the unrefunded remainder.

Owners and admins can run `appctl transactions totals --workspace "$WORKSPACE_ID"`.
Use `--period last_month` or `--period custom --start 2024-01-01 --end 2024-01-31`
to select a period. The output groups minor-unit balances by currency.

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

## Local demo seed

`go run ./cmd/demo` creates or reuses a local demo workspace. Set
`DEMO_OWNER_PASSWORD` explicitly. It prints the workspace ID and is not a
production migration tool.
