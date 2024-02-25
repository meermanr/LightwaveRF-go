FROM golang:1.22-alpine

RUN --mount=type=cache,target=/var/cache/apk/ <<EOF
  set -x
  id
  set -e
  adduser  -u 1000 -s /bin/bash -D devcontainer
  apk add sudo
  echo "devcontainer ALL=(root) NOPASSWD:ALL" > /etc/sudoers.d/devcontainer
  chmod 0400 /etc/sudoers.d/devcontainer
EOF

USER devcontainer

RUN --mount=type=cache,target=/var/cache/apk/ <<EOF
  sudo xargs apk add <<'  EOL'
    bash
    git
    openssh-client-default
  EOL
EOF

# ^-- blank line for consumption by continuation characters
