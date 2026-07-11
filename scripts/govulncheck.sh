#!/usr/bin/env bash
# Run govulncheck and fail only on vulnerabilities not listed in
# .govulncheck-ignore. Ignored findings are accepted risks documented in that
# file.
set -euo pipefail

IGNORE_FILE=".govulncheck-ignore"
GOVULNCHECK="${GOVULNCHECK:-$(go env GOPATH)/bin/govulncheck}"

if ! [ -x "$GOVULNCHECK" ]; then
	echo "Installing govulncheck..."
	go install golang.org/x/vuln/cmd/govulncheck@latest
	GOVULNCHECK="$(go env GOPATH)/bin/govulncheck"
fi

ignored_osvs=$(grep -v '^#' "$IGNORE_FILE" | grep -v '^$' | sort -u | jq -R . | jq -s .)

mapfile -t unignored < <(
	"$GOVULNCHECK" -format json ./... \
		| jq -r --argjson ignored "$ignored_osvs" '
			[ .[] | select(has("finding")) | .finding.osv ]
			| unique - $ignored
			| .[]
		'
)

if [ ${#unignored[@]} -eq 0 ]; then
	echo "govulncheck passed (all findings are listed in $IGNORE_FILE)"
	exit 0
fi

echo "govulncheck: unignored vulnerabilities found:"
printf '%s\n' "${unignored[@]}"
exit 1
