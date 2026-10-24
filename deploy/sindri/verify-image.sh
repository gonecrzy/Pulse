#!/usr/bin/env bash
set -euo pipefail

image_ref="${1:?usage: verify-image.sh IMAGE_REF SOURCE_SHA}"
expected_revision="${2:?usage: verify-image.sh IMAGE_REF SOURCE_SHA}"
if [[ ! "${expected_revision}" =~ ^[0-9a-f]{40}$ ]]; then
  echo "SOURCE_SHA must be a full 40-character lowercase Git SHA." >&2
  exit 2
fi

actual_arch="$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "${image_ref}")"
if [[ "${actual_arch}" != linux/amd64 ]]; then
  echo "Image architecture is ${actual_arch}; expected linux/amd64." >&2
  exit 1
fi

actual_revision="$(docker image inspect --format '{{ index .Config.Labels "org.opencontainers.image.revision" }}' "${image_ref}")"
if [[ "${actual_revision}" != "${expected_revision}" ]]; then
  echo "Image source revision is ${actual_revision}; expected ${expected_revision}." >&2
  exit 1
fi

actual_entrypoint="$(docker image inspect --format '{{json .Config.Entrypoint}}' "${image_ref}")"
if [[ "${actual_entrypoint}" != '["/docker-entrypoint.sh"]' ]]; then
  echo "Unexpected container entrypoint: ${actual_entrypoint}." >&2
  exit 1
fi

docker run --rm --entrypoint /bin/sh "${image_ref}" -ec \
  'test -x /app/pulse && test -x /docker-entrypoint.sh && test -x /docker-healthcheck.sh'

container_name="pulse-sindri-image-smoke-${BASHPID}"
volume_name="${container_name}-data"
host_port=""
cleanup() {
  docker rm -f "${container_name}" >/dev/null 2>&1 || true
  docker volume rm "${volume_name}" >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker volume create "${volume_name}" >/dev/null
docker run -d \
  --name "${container_name}" \
  --mount "type=volume,source=${volume_name},target=/data" \
  --publish 127.0.0.1::7655 \
  --env PULSE_DEPLOYMENT_METHOD=docker_compose \
  --env PULSE_DISABLE_AUTO_UPDATE=true \
  "${image_ref}" >/dev/null

port_mapping="$(docker port "${container_name}" 7655/tcp | head -n 1)"
host_port="${port_mapping##*:}"

wait_healthy() {
  local deadline=$((SECONDS + 180))
  local state=""
  while (( SECONDS < deadline )); do
    state="$(docker inspect --format '{{.State.Health.Status}}' "${container_name}")"
    if [[ "${state}" == healthy ]]; then
      return 0
    fi
    if [[ "${state}" == unhealthy ]] || [[ "$(docker inspect --format '{{.State.Running}}' "${container_name}")" != true ]]; then
      docker logs "${container_name}" >&2
      return 1
    fi
    sleep 2
  done
  docker logs "${container_name}" >&2
  echo "Container did not become healthy within 180 seconds." >&2
  return 1
}

wait_healthy
curl --fail --silent --show-error --max-time 5 "http://127.0.0.1:${host_port}/api/health" >/dev/null

marker=".sindri-image-smoke-${expected_revision}"
docker exec --user 1000:1000 "${container_name}" sh -ec \
  "printf '%s\\n' '${expected_revision}' > '/data/${marker}'"
docker restart "${container_name}" >/dev/null
wait_healthy
persisted_marker="$(docker exec "${container_name}" cat "/data/${marker}")"
if [[ "${persisted_marker}" != "${expected_revision}" ]]; then
  echo "Data volume did not preserve its marker across container restart." >&2
  exit 1
fi

echo "Image qualification passed: ${image_ref} (${actual_arch}, source ${actual_revision}); health and /data restart persistence verified."
