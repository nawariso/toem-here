#!/usr/bin/env bash
set -euo pipefail
: "${TEST_DATABASE_URL:?TEST_DATABASE_URL is required}"
(
  cd services/api
  test -z "$(gofmt -l .)"
  go vet ./...
  go test -race ./...
)
(
  cd apps/mobile
  npm ci
  npm test
  npm run typecheck
  npm run lint
  npx expo install --check
)
