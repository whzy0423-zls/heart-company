#!/usr/bin/env bash
set -euo pipefail

# Sync the checked-in story skill catalog to the production admin API.
# Credentials are supplied at runtime and are never written to the repository.

: "${PROD_API_BASE:=https://xn--9iq9az5uo8fz16d.com/api}"
: "${PROD_ADMIN_USERNAME:?set PROD_ADMIN_USERNAME}"
: "${PROD_ADMIN_PASSWORD:?set PROD_ADMIN_PASSWORD}"

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
SKILL_ROOT="$ROOT_DIR/nx-backend/data/story-skills"
MANIFEST="$SKILL_ROOT/story-skills-upload-manifest.json"

command -v curl >/dev/null || { echo "curl is required" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }

login_response="$(curl --fail-with-body --silent --show-error \
  -X POST "$PROD_API_BASE/auth/login" \
  -H 'Content-Type: application/json' \
  --data "$(jq -cn --arg u "$PROD_ADMIN_USERNAME" --arg p "$PROD_ADMIN_PASSWORD" '{username:$u,password:$p}')")"
token="$(jq -er '.data.accessToken // .accessToken' <<<"$login_response")"
auth=( -H "Authorization: Bearer $token" )

existing="$(curl --fail-with-body --silent --show-error "${auth[@]}" "$PROD_API_BASE/story-skills")"
map_file="$(mktemp)"
trap 'rm -f "$map_file"' EXIT
while IFS=$'\t' read -r key id status; do
  [[ -n "$key" ]] && printf '%s\t%s\t%s\n' "$key" "$id" "$status" >>"$map_file"
done < <(jq -r '.data[]? | [.key,.id,.status] | @tsv' <<<"$existing")

total=0
uploaded=0
published=0
skipped=0
failed=0

while IFS= read -r item; do
  total=$((total + 1))
  category="$(jq -r '.category' <<<"$item")"
  key="$(jq -r '.key' <<<"$item")"
  name="$(jq -r '.name' <<<"$item")"
  summary="$(jq -r '.summary' <<<"$item")"
  relative="$(jq -r '.file' <<<"$item" | sed 's#^nx-backend/data/story-skills/##')"
  file="$SKILL_ROOT/$relative"
  if [[ ! -s "$file" ]]; then
    echo "MISSING $key $file" >&2
    failed=$((failed + 1))
    continue
  fi

  id=""
  state=""
  existing_row="$(awk -F '\t' -v key="$key" '$1 == key { print; exit }' "$map_file")"
  if [[ -n "$existing_row" ]]; then
    id="$(cut -f2 <<<"$existing_row")"
    state="$(cut -f3 <<<"$existing_row")"
  else
    response="$(curl --fail-with-body --silent --show-error \
      -X POST "$PROD_API_BASE/story-skills/upload" "${auth[@]}" \
      -F "category=$category" -F "key=$key" -F "name=$name" \
      -F "summary=$summary" -F 'version=1.0.0' -F "file=@$file;type=text/markdown")" || {
        echo "UPLOAD_FAILED $key" >&2
        failed=$((failed + 1))
        continue
      }
    id="$(jq -er '.data.id // .id' <<<"$response")"
    printf '%s\t%s\tdraft\n' "$key" "$id" >>"$map_file"
    uploaded=$((uploaded + 1))
    echo "UPLOADED $category/$key id=$id"
  fi

  if [[ "$state" == "published" || "$state" == "enabled" ]]; then
    skipped=$((skipped + 1))
    continue
  fi
  curl --fail-with-body --silent --show-error \
    -X POST "$PROD_API_BASE/story-skills/$id/publish" "${auth[@]}" \
    -H 'Content-Type: application/json' --data '{}' >/dev/null || {
      echo "PUBLISH_FAILED $key id=$id" >&2
      failed=$((failed + 1))
      continue
    }
  published=$((published + 1))
  echo "PUBLISHED $category/$key id=$id"
done < <(jq -c '.items[]' "$MANIFEST")

echo "SUMMARY total=$total uploaded=$uploaded published=$published skipped=$skipped failed=$failed"
(( failed == 0 ))
