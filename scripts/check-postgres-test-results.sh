#!/bin/sh
set -eu

jq -n -e '
  ["TestEngagementActionCompletionUpgrade/postgres",
   "TestPostgresEngagementNameUpgrade",
   "TestPostgresWorkspaceCurrenciesUpgrade",
   "TestPostgresRemoveItemsUpgrade"] as $required |
  reduce inputs as $event ({};
    if $event.Package == "example.com/dynamis-code/apps-template/internal/platform/database"
       and ($event.Test | IN($required[]))
       and ($event.Action | IN("pass", "fail", "skip"))
    then .[$event.Test] = $event.Action else . end
  ) |
  [. as $results | $required[] | select($results[.] != "pass") | "\(.): \($results[.] // "missing")"] |
  if length == 0 then true else .[] | halt_error(1) end
' "$1"
