# Packaging

Supported distribution is the container image, deployed through Coolify or
Docker Compose (ADR 004):

- `coolify/` — Coolify service template.
- `docker/` — Dockerfile and the Compose file.

Native `.deb`, `.rpm` and systemd packaging was considered and dropped for
v1.0. Binnacle needs the Docker socket proxy, a read-only host root mount and
a managed master key, which the container deployment provides and a bare
service would leave to each operator. A native package would be a new
supported platform and needs an ADR first (see `GOVERNANCE.md`).
