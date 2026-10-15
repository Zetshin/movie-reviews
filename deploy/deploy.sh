#!/bin/sh
# Runs on the server. Pulls a new image, restarts the stack and rolls back
# automatically if the app does not become healthy.
#
#   deploy.sh <image-ref>      e.g. ghcr.io/zetshin/movie-reviews:sha-abc1234
#
# Expects to run from the deployment directory that contains .env,
# docker-compose.yml, docker-compose.prod.yml and deploy/Caddyfile.
set -eu

NEW_IMAGE="${1:?usage: deploy.sh <image-ref>}"
COMPOSE="docker compose -f docker-compose.yml -f docker-compose.prod.yml"

[ -f .env ] || { echo "missing .env in $(pwd)" >&2; exit 1; }

# Remember what is running now so we can roll back.
PREVIOUS_IMAGE=$(sed -n 's/^APP_IMAGE=//p' .env | tail -n 1)

set_image() {
  if grep -q '^APP_IMAGE=' .env; then
    sed -i "s|^APP_IMAGE=.*|APP_IMAGE=$1|" .env
  else
    printf '\nAPP_IMAGE=%s\n' "$1" >> .env
  fi
}

echo "Deploying $NEW_IMAGE (previous: ${PREVIOUS_IMAGE:-none})"
set_image "$NEW_IMAGE"
$COMPOSE pull app

# --wait blocks until every service with a healthcheck reports healthy.
if $COMPOSE up -d --remove-orphans --wait --wait-timeout 180; then
  echo "Deploy succeeded: $NEW_IMAGE"
  docker image prune -f >/dev/null 2>&1 || true
  exit 0
fi

echo "New version is unhealthy; recent app logs:" >&2
$COMPOSE logs --tail 50 app >&2 || true

if [ -n "$PREVIOUS_IMAGE" ] && [ "$PREVIOUS_IMAGE" != "$NEW_IMAGE" ]; then
  echo "Rolling back to $PREVIOUS_IMAGE" >&2
  set_image "$PREVIOUS_IMAGE"
  $COMPOSE up -d --remove-orphans --wait --wait-timeout 180 || true
fi
exit 1
