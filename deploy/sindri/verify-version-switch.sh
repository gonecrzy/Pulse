#!/usr/bin/env bash
set -euo pipefail

old_ref="${1:?usage: verify-version-switch.sh OLD_SHA_IMAGE NEW_SHA_IMAGE OLD_SOURCE_SHA NEW_SOURCE_SHA}"
new_ref="${2:?usage: verify-version-switch.sh OLD_SHA_IMAGE NEW_SHA_IMAGE OLD_SOURCE_SHA NEW_SOURCE_SHA}"
old_sha="${3:?usage: verify-version-switch.sh OLD_SHA_IMAGE NEW_SHA_IMAGE OLD_SOURCE_SHA NEW_SOURCE_SHA}"
new_sha="${4:?usage: verify-version-switch.sh OLD_SHA_IMAGE NEW_SHA_IMAGE OLD_SOURCE_SHA NEW_SOURCE_SHA}"

if [[ ! "${old_ref}" =~ ^ghcr\.io/gonecrzy/pulse:([0-9a-f]{40})$ ]]; then
  echo "OLD_SHA_IMAGE must be the fork GHCR tag matching OLD_SOURCE_SHA." >&2
  exit 2
fi
parsed_old_sha="${BASH_REMATCH[1]}"
if [[ "${parsed_old_sha}" != "${old_sha}" ]]; then
  echo "OLD_SHA_IMAGE must be the fork GHCR tag matching OLD_SOURCE_SHA." >&2
  exit 2
fi
if [[ ! "${new_ref}" =~ ^ghcr\.io/gonecrzy/pulse:([0-9a-f]{40})$ ]]; then
  echo "NEW_SHA_IMAGE must be the fork GHCR tag matching NEW_SOURCE_SHA." >&2
  exit 2
fi
parsed_new_sha="${BASH_REMATCH[1]}"
if [[ "${parsed_new_sha}" != "${new_sha}" ]]; then
  echo "NEW_SHA_IMAGE must be the fork GHCR tag matching NEW_SOURCE_SHA." >&2
  exit 2
fi
if [[ "${old_sha}" == "${new_sha}" ]]; then
  echo "The two source revisions must be distinct." >&2
  exit 2
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "${script_dir}"
project_name="pulse-sindri-switch-${GITHUB_RUN_ID:-local}-${BASHPID}"
host_port="$(python3 -c 'import socket; sock = socket.socket(); sock.bind(("127.0.0.1", 0)); print(sock.getsockname()[1]); sock.close()')"

compose() {
  local image="$1"
  shift
  PULSE_IMAGE="${image}" \
    COMPOSE_PROJECT_NAME="${project_name}" \
    PULSE_BIND_ADDRESS=127.0.0.1 \
    PULSE_PORT="${host_port}" \
    TZ=UTC \
    ./compose.sh "$@"
}

cleanup() {
  compose "${old_ref}" down --volumes --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

wait_healthy() {
  local container_id="$1"
  local deadline=$((SECONDS + 180))
  local state=""
  while (( SECONDS < deadline )); do
    state="$(docker inspect --format '{{.State.Health.Status}}' "${container_id}")"
    if [[ "${state}" == healthy ]]; then
      curl --fail --silent --show-error --max-time 5 "http://127.0.0.1:${host_port}/api/health" >/dev/null
      return 0
    fi
    if [[ "${state}" == unhealthy ]] || [[ "$(docker inspect --format '{{.State.Running}}' "${container_id}")" != true ]]; then
      docker logs "${container_id}" >&2
      return 1
    fi
    sleep 2
  done
  docker logs "${container_id}" >&2
  echo "Compose container did not become healthy within 180 seconds." >&2
  return 1
}

assert_deployment() {
  local expected_ref="$1"
  local expected_sha="$2"
  local expected_image_id="$3"
  local container_id
  container_id="$(compose "${expected_ref}" ps -q pulse)"
  if [[ -z "${container_id}" ]]; then
    echo "Compose did not create the Pulse container for ${expected_ref}." >&2
    return 1
  fi

  local actual_ref actual_image_id revision volume_name
  actual_ref="$(docker inspect --format '{{.Config.Image}}' "${container_id}")"
  actual_image_id="$(docker inspect --format '{{.Image}}' "${container_id}")"
  revision="$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "${expected_ref}")"
  volume_name="$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' "${container_id}")"
  if [[ "${actual_ref}" != "${expected_ref}" || "${revision}" != "${expected_sha}" || "${actual_image_id}" != "${expected_image_id}" ]]; then
    echo "Deployment identity mismatch: ref=${actual_ref}, revision=${revision}, image_id=${actual_image_id}." >&2
    return 1
  fi
  if [[ "${volume_name}" != "${project_name}_pulse-data" ]]; then
    echo "Unexpected /data volume ${volume_name}; expected ${project_name}_pulse-data." >&2
    return 1
  fi

  wait_healthy "${container_id}"
  printf '%s\n' "${container_id}"
}

compose "${old_ref}" pull
compose "${new_ref}" pull

old_image_id="$(docker image inspect --format '{{.Id}}' "${old_ref}")"
new_image_id="$(docker image inspect --format '{{.Id}}' "${new_ref}")"
old_revision="$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "${old_ref}")"
new_revision="$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "${new_ref}")"
if [[ "${old_revision}" != "${old_sha}" || "${new_revision}" != "${new_sha}" || "${old_image_id}" == "${new_image_id}" ]]; then
  echo "The pulled tags do not identify two distinct images with their expected source revisions." >&2
  exit 1
fi

compose "${old_ref}" up -d
container_id="$(assert_deployment "${old_ref}" "${old_sha}" "${old_image_id}")"
marker=".sindri-version-switch-${old_sha}-${new_sha}"
docker exec --user 1000:1000 "${container_id}" sh -ec \
  "printf '%s\\n' '${old_sha}:${new_sha}' > '/data/${marker}'"

compose "${new_ref}" up -d --force-recreate
container_id="$(assert_deployment "${new_ref}" "${new_sha}" "${new_image_id}")"
test "$(docker exec "${container_id}" cat "/data/${marker}")" = "${old_sha}:${new_sha}"

compose "${old_ref}" up -d --force-recreate
container_id="$(assert_deployment "${old_ref}" "${old_sha}" "${old_image_id}")"
test "$(docker exec "${container_id}" cat "/data/${marker}")" = "${old_sha}:${new_sha}"
docker volume inspect "${project_name}_pulse-data" >/dev/null

echo "Image-version switching passed: ${old_sha} -> ${new_sha} -> ${old_sha}; both revisions were healthy and the /data marker survived. Only the disposable ${project_name} project and volume are removed on exit. This does not qualify application schema downgrade compatibility."
