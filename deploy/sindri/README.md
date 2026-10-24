# Isolated Pulse fork deployment

This Compose project runs the fork-owned Pulse image in an isolated project by
default. It does not set a fixed container name, use the root Compose project's
`pulse-data` volume, expose the Docker socket, or request extra privileges.
Keep the default project name for testing. Choose another project name only
when intentionally creating a separate stack.

## Pin an image

Copy the example and set `PULSE_IMAGE` to either an exact full-source-SHA tag
or a digest reference recorded by the GitHub Actions build:

```sh
cd /root/Pulse/deploy/sindri
cp .env.example .env
${EDITOR:-vi} .env
./compose.sh config
```

The wrapper rejects other registries, floating tags such as `latest`, and
short-SHA tags. A missing image reference fails Compose interpolation. The
default project binds to `127.0.0.1:17655`, uses the isolated
`pulse-sindri-test_pulse-data` volume, and disables the server's in-app
automatic updates for pinned deployments.

The publisher uses only the workflow's `GITHUB_TOKEN` with `packages:write`.
If GHCR denies the first publish, grant the repository's Actions workflow
package-write access and confirm the package's repository association in
GitHub package settings. Private-package pulls require an operator-managed
Docker login; keep any such credential out of this directory and `.env`.

## Start and inspect

```sh
./compose.sh pull
./compose.sh up -d
./compose.sh ps
curl --fail http://127.0.0.1:17655/api/health
docker inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$(./compose.sh ps -q pulse)"
```

Change `PULSE_PORT` or `PULSE_BIND_ADDRESS` in `.env` to choose another host
binding. Keep loopback binding unless remote access is intended. Change
`COMPOSE_PROJECT_NAME` to create a different deployment project and volume.
Changing a project name creates a new data volume; it does not migrate the old
one.

## Update and rollback

Use a newly qualified source-SHA tag to update. To pin a known digest, replace
the image value in `.env` with the corresponding digest reference. Pull and
recreate explicitly:

```sh
./compose.sh pull
./compose.sh up -d --force-recreate
curl --fail http://127.0.0.1:17655/api/health
```

For rollback, restore the previous verified SHA tag or digest in `.env`, then
run the same `pull`, `up -d --force-recreate`, and health commands. Both
operations keep the project's `/data` volume. Do not use `down --volumes` for
an operator deployment; that deletes the stack's data. Back up `/data` before
any separately authorized production cutover.

`./verify-compose.sh <sha-tag-image> <sha256:digest> <source-sha>` runs the
disposable validation sequence used by CI: health, data across restart, tag to
digest redeployment, rollback to the previous verified reference, and volume
retention. It creates a unique project and removes only that disposable
project and volume on exit.

## Branch layout

Recommended eventual layout: retain `main` as the upstream baseline, use
`sindri` as the future tested/default branch, and keep
`feature/sindri-integration` for active development. Do not detach the fork,
create/promote `sindri`, or change the default branch until the build and
Compose qualification have passed independent review.
