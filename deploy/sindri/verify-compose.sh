#!/usr/bin/env bash
set -euo pipefail

image_ref="${1:?usage: verify-compose.sh SHA_TAG_IMAGE SHA256_DIGEST SOURCE_SHA}"
expected_digest="${2:?usage: verify-compose.sh SHA_TAG_IMAGE SHA256_DIGEST SOURCE_SHA}"
source_sha="${3:?usage: verify-compose.sh SHA_TAG_IMAGE SHA256_DIGEST SOURCE_SHA}"

if [[ ! "${image_ref}" =~ ^ghcr\.io/gonecrzy/pulse:([0-9a-f]{40})$ ]]; then
  echo "IMAGE_REF must use the exact source SHA tag provided as SOURCE_SHA." >&2
  exit 2
fi
if [[ "${BASH_REMATCH[1]}" != "${source_sha}" ]]; then
  echo "IMAGE_REF tag does not match SOURCE_SHA." >&2
  exit 2
fi
if [[ ! "${expected_digest}" =~ ^sha256:[0-9a-f]{64}$ ]]; then
  echo "DIGEST must be sha256 followed by 64 lowercase hexadecimal characters." >&2
  exit 2
fi

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "${script_dir}"
project_name="pulse-sindri-verify-${GITHUB_RUN_ID:-local}-${BASHPID}"
host_port="$(python3 -c 'import socket; sock = socket.socket(); sock.bind(("127.0.0.1", 0)); print(sock.getsockname()[1]); sock.close()')"
digest_ref="ghcr.io/gonecrzy/pulse@${expected_digest}"

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
  compose "${image_ref}" down --volumes --remove-orphans >/dev/null 2>&1 || true
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
  local expected_image_id="$2"
  local container_id
  container_id="$(compose "${expected_ref}" ps -q pulse)"
  if [[ -z "${container_id}" ]]; then
    echo "Compose did not create the Pulse container." >&2
    return 1
  fi
  local actual_ref actual_image_id
  actual_ref="$(docker inspect --format '{{.Config.Image}}' "${container_id}")"
  actual_image_id="$(docker inspect --format '{{.Image}}' "${container_id}")"
  if [[ "${actual_ref}" != "${expected_ref}" ]]; then
    echo "Container uses ${actual_ref}; expected ${expected_ref}." >&2
    return 1
  fi
  if [[ -n "${expected_image_id}" && "${actual_image_id}" != "${expected_image_id}" ]]; then
    echo "Container image ID changed unexpectedly during pinned-reference update." >&2
    return 1
  fi
  wait_healthy "${container_id}"
  printf '%s\n' "${container_id}"
}

compose "${image_ref}" pull
compose "${image_ref}" up -d
container_id="$(assert_deployment "${image_ref}" "")"
initial_image_id="$(docker inspect --format '{{.Image}}' "${container_id}")"
revision="$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "${image_ref}")"
if [[ "${revision}" != "${source_sha}" ]]; then
  echo "Published image revision is ${revision}; expected ${source_sha}." >&2
  exit 1
fi

volume_name="$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' "${container_id}")"
expected_volume="${project_name}_pulse-data"
if [[ "${volume_name}" != "${expected_volume}" ]]; then
  echo "Unexpected data volume ${volume_name}; expected isolated volume ${expected_volume}." >&2
  exit 1
fi
docker volume inspect "${volume_name}" >/dev/null
marker=".sindri-compose-smoke-${source_sha}"
docker exec --user 1000:1000 "${container_id}" sh -ec \
  "printf '%s\\n' '${source_sha}' > '/data/${marker}'"

compose "${image_ref}" restart pulse
container_id="$(assert_deployment "${image_ref}" "${initial_image_id}")"
test "$(docker exec "${container_id}" cat "/data/${marker}")" = "${source_sha}"

# Change from the source-SHA tag to its immutable digest reference. Both must
# resolve to the same verified OCI image and retain the same project volume.
compose "${digest_ref}" pull
compose "${digest_ref}" up -d --force-recreate
container_id="$(assert_deployment "${digest_ref}" "${initial_image_id}")"
test "$(docker exec "${container_id}" cat "/data/${marker}")" = "${source_sha}"

# Roll back to the previously verified source-SHA reference without deleting
# the isolated volume, then prove the container and data are still intact.
compose "${image_ref}" pull
compose "${image_ref}" up -d --force-recreate
container_id="$(assert_deployment "${image_ref}" "${initial_image_id}")"
test "$(docker exec "${container_id}" cat "/data/${marker}")" = "${source_sha}"
docker volume inspect "${volume_name}" >/dev/null

echo "Compose qualification passed: healthy ${image_ref}; persisted data survived restart, tag-to-digest redeployment, and rollback. Isolated volume: ${volume_name}."
