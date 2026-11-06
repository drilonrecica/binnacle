// SPDX-License-Identifier: AGPL-3.0-only

// Package acceptance holds the release acceptance suites. They start the real
// binnacle binary as a child process and drive it over HTTP, so they live
// behind the "acceptance" build tag and run with `make acceptance-auth` and
// `make acceptance-portability` rather than in `go test ./...`.
package acceptance
