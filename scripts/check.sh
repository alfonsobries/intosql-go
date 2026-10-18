#!/usr/bin/env bash
# Install, lint and test the Go SDK with the 100% coverage gate. Run from anywhere.
set -euo pipefail

cd "$(dirname "$0")/.."

staticcheck_version="v0.8.2"

echo "==> go mod"
go mod download
go mod verify
go mod tidy -diff

echo "==> gofmt"
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
    echo "Not formatted with gofmt:"
    echo "$unformatted"
    exit 1
fi

echo "==> go vet"
go vet ./...

echo "==> staticcheck"
go run "honnef.co/go/tools/cmd/staticcheck@${staticcheck_version}" -checks all ./...

echo "==> go test"
profile=$(mktemp)
trap 'rm -f "$profile"' EXIT
go test -count=1 -covermode=atomic -coverprofile="$profile" ./...

total=$(go tool cover -func="$profile" | awk '/^total:/ { print $NF }')
echo "Coverage: $total"

if [ "$total" != "100.0%" ]; then
    go tool cover -func="$profile" | grep -v '100.0%$'
    echo "Coverage is $total; it must be 100.0%."
    exit 1
fi
