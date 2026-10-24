#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
compose_file="${script_dir}/compose.yml"
compose_args=(--project-directory "${script_dir}" -f "${compose_file}")

# Compose interpolation handles the required value; this wrapper additionally
# rejects mutable tags and images outside this fork's GHCR repository.
images="$(docker compose "${compose_args[@]}" config --images)"
if [[ "${images}" != ghcr.io/gonecrzy/pulse@sha256:* && ! "${images}" =~ ^ghcr\.io/gonecrzy/pulse:[0-9a-f]{40}$ ]]; then
  echo "PULSE_IMAGE must be ghcr.io/gonecrzy/pulse:<full-source-SHA> or ghcr.io/gonecrzy/pulse@sha256:<digest>." >&2
  exit 2
fi
if [[ "${images}" == ghcr.io/gonecrzy/pulse@sha256:* && ! "${images}" =~ ^ghcr\.io/gonecrzy/pulse@sha256:[0-9a-f]{64}$ ]]; then
  echo "PULSE_IMAGE digest must contain exactly 64 lowercase hexadecimal characters." >&2
  exit 2
fi

exec docker compose "${compose_args[@]}" "$@"
