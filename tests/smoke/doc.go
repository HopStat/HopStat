// Package smoke holds end-to-end checks that build the real binary, boot it against a
// throwaway database and drive it over HTTP.
//
// Everything here is behind the `smoke` build tag so the ordinary `go test ./...` run — and
// with it the 100% statement coverage gate — is untouched. This file carries no tag on
// purpose: a directory whose Go files are all excluded by a build constraint makes
// `go list ./...` fail with "build constraints exclude all Go files", which would break the
// normal test run. With this file present the package stays valid and simply reports
// "no test files" when the tag is off.
//
// Run them with:
//
//	go test -tags=smoke ./tests/smoke/...
package smoke
