# Logs and process diagnostics

Binnacle v0.4 adds session-only, read-only diagnostics. Logs live in the
**Logs** tab of each resource (Resources › a resource › Logs), opened from the
resource, from an event's "logs" link, or from "Logs for …" in the command
palette. The tab reads Docker logs for the resource or one of its containers.
It supports 5 minute, 30 minute, and 1 hour ranges, literal search, and live
follow. Messages are redacted before search or delivery and are never stored
by Binnacle. The old `/logs` address opens the Logs tab of the resource viewed
last in that browser, or the Resources list with a prompt to choose one.

The browser keeps at most the newest 5,000 lines of a view in a fixed-size
ring and renders the newest 400 at a time ("Show earlier lines" reveals more),
so a busy container streaming quickly does not slow the page. Leaving the tab
cancels an unfinished read and stops following.

Responses default to 500 lines and cannot exceed the configured 5,000-line or
1 MiB ceilings. A resource can contain at most 32 log components. Follow
sessions stop after 30 minutes, as soon as the browser disconnects, or when
the browser session is signed out or expires; the session is re-checked on
every 20-second heartbeat, or when Binnacle shuts down. Following a resource
streams all of its components at once, from the start of the selected range
with no end time. At most four log reads, follows included, run at
once across all users; further reads wait, so a resource with more than four
components follows the first four until a slot frees. Log reads do not share the Docker
call slots used for metric collection, so open log views cannot stall it.

Redaction covers common authorization headers, password/token/API-key
assignments, credential-bearing URLs, and private-key blocks. A PEM block that
spans several log lines is tracked across those lines: the `BEGIN` line is
replaced, the lines up to the `END` marker are dropped, and a block that never
ends stops hiding lines after 256. Terminal escape sequences (colours, cursor
moves, titles) are stripped on the server before redaction, so they can neither
split a secret nor reach the browser or the clipboard. Redaction is
best-effort, not a guarantee. Applications should never write secrets to logs.
Up to 16 additional Go RE2 expressions can be supplied with
`BINNACLE_LOGS_REDACTION_PATTERNS`, separated by `||`.

The Server page samples host processes only when requested. It uses two bounded
reads of the host `/proc`, serializes scans, returns 25 rows by default (100
maximum), and does not persist results. `/etc/passwd` is mounted read-only only
to display usernames; numeric UIDs are used when it is unavailable. Process
command lines pass through the same redaction rules as logs, so credential
URLs and `--password=` style arguments are masked; the glued short forms
`mysql -pSECRET`, `mariadb -pSECRET` and `redis-cli -aSECRET` are masked too.

There are deliberately no process signals, exec, shell, renice, container
mutation, server-wide log search, indexing, or log persistence endpoints.
