# Isolated Komodo Phase 2F test instance

This optional Compose project starts a separate Pulse container with a fixed
container name, bridge network, loopback-only port, and dedicated persistent
bind mount. It does not read or modify production Pulse configuration or data.
The image is selected by immutable GHCR digest; the Compose file contains no
Komodo credential or other secret.

## Prerequisites

- Docker Engine and Docker Compose v2.
- The successful `Komodo Phase 2F test image` workflow summary, which lists the
  immutable image digest and commit-specific tag.
- To qualify cross-provider links, an existing approved read-only Proxmox
  connection must also be available. Skip live Proxmox association checks if it
  is not; do not create a privileged credential for this test.
- A host that can resolve `komodo.home.goneczy.com` and validate its HTTPS
  certificate normally. Do not disable TLS verification or use the qualification
  harness's test-only HTTP endpoint for a running Pulse instance.
- If the GHCR package is private, an operator with existing package-read access
  must authenticate Docker to GHCR. Do not put a token in this directory.

## Start

Copy `.env.example` to `.env`. Set `PULSE_IMAGE` to the full
`ghcr.io/gonecrzy/pulse@sha256:<64-hex-digest>` value from the workflow summary.
Set `PULSE_TEST_DATA_DIR` to a new absolute path used only by this test instance;
do not reuse a production Pulse data/config directory or a production volume.
Choose an unused loopback host port if `17656` is occupied. The environment
file contains no credentials; keep it local and out of version control.

```sh
cd deploy/komodo-phase2f
cp .env.example .env
${EDITOR:-vi} .env
mkdir -p /srv/pulse-komodo-phase2f-test/data
sudo chown 1000:1000 /srv/pulse-komodo-phase2f-test/data
docker compose --env-file .env -f compose.yml config
docker compose --env-file .env -f compose.yml up -d
curl --fail http://127.0.0.1:17656/api/health
```

Open `http://127.0.0.1:17656` locally. Add the already approved read-only
Komodo connection through the Pulse UI using `https://komodo.home.goneczy.com`.
Enter its credentials only in Pulse's protected connection form. Do not add them
to `.env`, Compose environment, shell command arguments, image build inputs, or
this repository. Current collection exposes only allowlisted read methods;
Phase 2F adds no execution or remediation capability.

The bind directory persists across container restarts and image updates. The
fixed network permits the Pulse container's required outbound HTTPS access but
does not attach it to a production Pulse network. Only the loopback host port is
published.

## Back up and roll back

For a fresh test data directory, no production backup is involved or needed.
Before changing the image digest after configuring connections, make a cold
backup of this test data only. From `deploy/komodo-phase2f`, first preserve the
image/path settings and then archive the dedicated directory while stopped:

```sh
cp .env .env.before-upgrade
docker compose --env-file .env -f compose.yml stop
sudo tar --numeric-owner -cpf /srv/pulse-komodo-phase2f-test/phase2f-data-before-upgrade.tar \
  -C /srv/pulse-komodo-phase2f-test/data .
docker compose --env-file .env -f compose.yml start
```

Keep the archive outside the mounted data directory. If reusing a copied test database for identity
and relationship verification, first make a consistent SQLite backup using
Pulse's supported backup procedure and preserve the matching encryption-key
and configuration pair. Never mount the live writable production database.

To roll back, stop this Compose project, preserve the post-upgrade directory for
inspection, restore the pre-upgrade test-only archive, restore the previous
image digest from `.env.before-upgrade`, and recreate the test container:

```sh
docker compose --env-file .env -f compose.yml down
mv /srv/pulse-komodo-phase2f-test/data /srv/pulse-komodo-phase2f-test/data-after-upgrade
mkdir -p /srv/pulse-komodo-phase2f-test/data
sudo tar --numeric-owner -xpf /srv/pulse-komodo-phase2f-test/phase2f-data-before-upgrade.tar \
  -C /srv/pulse-komodo-phase2f-test/data
cp .env.before-upgrade .env
docker compose --env-file .env -f compose.yml up -d --force-recreate
```

The image has no Phase 2F database migration. If data was migrated by newer
code, restore the pre-update test backup before starting an older image; do not
rely on a schema downgrade. With a fresh disposable directory, rollback may
instead stop and remove only this test container and test directory. Do not run
cleanup against production containers, volumes, or data.

## Acceptance checklist

- [ ] The container uses the digest-pinned image and reports healthy at `/api/health`.
- [ ] The container, bridge network, loopback port, and data path are distinct from production.
- [ ] `https://komodo.home.goneczy.com` resolves and its trusted TLS connection succeeds.
- [ ] Komodo Core reports version 2.3.4; the existing approved read-only connection passes Test and shows redacted status.
- [ ] Inventory appears without duplicate resources; compare against the observed 11 Servers and 32 Stacks baseline while treating counts as changeable.
- [ ] Server/Stack identities stay stable over polling and restart, and the observed Stack parent relationships resolve.
- [ ] Proxmox connection behavior remains healthy. Link only operator-confirmed Proxmox VM and Komodo Server IDs; verify the Proxmox metric target/history and both provider identities survive polling and restart.
- [ ] Verify the Jellyfin-DMZ and Public-DMZ identity links, Jellyfin workload/availability-check associations, and operator-authored service dependencies. Add Traefik dependency/routing only when verified inventory evidence or explicit operator links support it.
- [ ] Verify resource health, canonical alert/recovery, NotificationManager delivery, and sanitized Patrol context. Disable Sindri AI and confirm polling, alert evaluation, and notifications continue.
- [ ] For failures, use fixtures or a controlled test-only resource/endpoint. Do not interrupt production infrastructure or mutate Komodo resources.
- [ ] Stop and remove only this test Compose project after acceptance; retain its data only when it is needed for review.

Connection test, identity folding, relationship persistence, alert lifecycle,
notification routing, Patrol sanitization, and AI-independent collection already
have repository fixture coverage. This live checklist is required to validate
the operator's actual Proxmox associations and current Komodo state; automated
CI never receives infrastructure credentials and never contacts Komodo.
