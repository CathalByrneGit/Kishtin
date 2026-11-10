#!/bin/sh
# Starts a kishtind with a new, synthetic kitchen for the browser tests:
# a manager, a cook, a device key, a small menu and one open order.
# Usage: start-server.sh PORT   (run from the repository root)
set -eu
PORT="$1"
DATA="$(pwd)/e2e/.data"
rm -rf "$DATA"
mkdir -p "$DATA"
export KISHTIN_DATA_DIR="$DATA/data" KISHTIN_TABLETS=false KISHTIN_LISTEN="127.0.0.1:$PORT"

KISHTIN_SETUP_PASSWORD=synthetic-password bin/kishtind setup --name "Máire Manager" --pin 9999 --json > "$DATA/setup.json"
KEY="$(sed -n 's/.*"secret": *"\([^"]*\)".*/\1/p' "$DATA/setup.json" | head -1)"

bin/kishtind &
SERVER=$!
trap 'kill $SERVER 2>/dev/null || true' EXIT INT TERM

export KISHTIN_URL="http://127.0.0.1:$PORT" KISHTIN_API_KEY="$KEY"
i=0
until bin/kishtin whoami >/dev/null 2>&1; do
  i=$((i + 1))
  [ "$i" -gt 100 ] && { echo "kishtind did not start" >&2; exit 1; }
  sleep 0.1
done

bin/kishtin keys create --kind device --name "Test tablet" --json | sed -n 's/.*"secret": *"\([^"]*\)".*/\1/p' | head -1 > "$DATA/devkey"
bin/kishtin staff add --name "Liam Cook" --role line_cook --pin 1234 >/dev/null
bin/kishtin staff add --name "Aoife Chef" --role head_chef --pin 2222 >/dev/null
bin/kishtin menu station add --name Grill --position 1 >/dev/null
bin/kishtin menu equipment add --station Grill --name Flat-top --slots 3 >/dev/null
bin/kishtin menu dish add --name "Synthetic burger" --station Grill --equipment Flat-top --cook 8m --rest 2m --plate 1m >/dev/null
bin/kishtin order new --table 4 --item "2x Synthetic burger" >/dev/null
touch "$DATA/ready"
wait $SERVER
