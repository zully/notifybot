# Repository instructions

- Act as a senior software engineer; base findings on inspected code and checks.
- Prefer Rust, Go, then Python for new projects; this repository is a Go application.
- Keep all changes local. Do not commit, push, create branches, or deploy remotely.
- Maintain a working Dockerfile; keep credentials and local scripts out of build contexts.
- Keep documentation concise and code comments to one line where practical.
- Update README.md for configuration, metrics, or operational behavior changes.
- Use structured slog logging. Do not log raw IRC traffic or credentials; keep routine protocol events at DEBUG.
- Keep Prometheus labels bounded; never label metrics with nicknames, email addresses, or error text.
- Apply a cancellable delay after every failed connection or disconnect. Stop polling and close the socket before retrying.
- Run `go test -race ./...`, `go vet ./...`, and `git diff --check` after code changes.
- Test IRC behavior against the local Undernet ircu container using the README commands; mock SES so tests send no real email.
- Never source local-test.sh during checks: it contains private local configuration.
- Never add secrets, sensitive information, or generated testing artifacts to Git. Version test source and reusable test configuration; ignore credentials, logs, coverage reports, binaries, archives, and temporary output.
