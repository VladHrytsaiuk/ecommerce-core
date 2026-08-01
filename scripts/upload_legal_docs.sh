#!/usr/bin/env bash
#
# Завантажує юридичні документи (Політика конфіденційності, Публічна оферта)
# через існуючі адмін-роути API:
#   POST /api/admin/auth/login          -> отримати access_token
#   POST /api/admin/documents           -> створити документ
#   POST /api/admin/documents/{id}/versions -> додати нову активну версію
#
# Ідемпотентний: якщо документ із таким slug уже існує (409), скрипт
# додає НОВУ версію замість падіння.
#
# Контент береться з scripts/legal/<slug>.<lang>.json (формат EditorJS:
# {time, blocks, version}) і зберігається у полі content[<lang>] як JSON-об'єкт
# (узгоджено з наявними документами в системі). Файл *.en.json опціональний —
# якщо його немає, завантажується лише uk (публічний роут робить фолбек на uk).
#
# Використання:
#   ADMIN_EMAIL=admin@example.com ADMIN_PASSWORD=secret \
#   BASE_URL=http://localhost:8080 ./scripts/upload_legal_docs.sh
#
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
ADMIN_EMAIL="${ADMIN_EMAIL:-}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-}"
LEGAL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/legal"

# slug | title_uk | title_en | basename (файли legal/<basename>.<lang>.html)
DOCUMENTS=(
  "privacy-policy|Політика конфіденційності|Privacy Policy|privacy-policy"
  "public-offer|Договір публічної оферти|Public Offer Agreement|public-offer"
)

# --- Перевірки оточення -----------------------------------------------------
for bin in curl jq; do
  command -v "$bin" >/dev/null || { echo "✗ Потрібен '$bin'"; exit 1; }
done
[[ -n "$ADMIN_EMAIL" && -n "$ADMIN_PASSWORD" ]] || {
  echo "✗ Задайте ADMIN_EMAIL та ADMIN_PASSWORD (env)"; exit 1; }

# curl-обгортка: друкує тіло, у останньому рядку — HTTP-код
req() { # METHOD URL [BODY]
  local method="$1" url="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -sS -X "$method" "$url" \
      -H "Content-Type: application/json" \
      ${TOKEN:+-H "Authorization: Bearer $TOKEN"} \
      -d "$body" -w $'\n%{http_code}'
  else
    curl -sS -X "$method" "$url" \
      ${TOKEN:+-H "Authorization: Bearer $TOKEN"} \
      -w $'\n%{http_code}'
  fi
}

# --- 1. Логін адміна --------------------------------------------------------
echo "→ Логін: $BASE_URL/api/admin/auth/login"
LOGIN_BODY="$(jq -n --arg e "$ADMIN_EMAIL" --arg p "$ADMIN_PASSWORD" \
  '{email:$e, password:$p}')"
TOKEN=""
resp="$(req POST "$BASE_URL/api/admin/auth/login" "$LOGIN_BODY")"
code="$(tail -n1 <<<"$resp")"; payload="$(sed '$d' <<<"$resp")"
[[ "$code" == "200" ]] || { echo "✗ Логін не вдався ($code): $payload"; exit 1; }
TOKEN="$(jq -r '.access_token' <<<"$payload")"
[[ -n "$TOKEN" && "$TOKEN" != "null" ]] || { echo "✗ Немає access_token"; exit 1; }
echo "✓ Токен отримано"

# Знайти id документа за slug (limit максимум 100; документів небагато)
doc_id_by_slug() {
  local slug="$1" r
  r="$(req GET "$BASE_URL/api/admin/documents?limit=100")"
  sed '$d' <<<"$r" | jq -r --arg s "$slug" \
    'try (.data[] | select(.slug==$s) | .id) catch empty' | head -n1
}

# Зібрати content-об'єкт із наявних EditorJS-файлів *.uk.json / *.en.json
build_content() { # basename
  local base="$1" uk="$LEGAL_DIR/$1.uk.json" en="$LEGAL_DIR/$1.en.json"
  [[ -f "$uk" ]] || { echo "✗ Немає файлу $uk" >&2; return 1; }
  if [[ -f "$en" ]]; then
    jq -n --slurpfile uk "$uk" --slurpfile en "$en" '{uk:$uk[0], en:$en[0]}'
  else
    jq -n --slurpfile uk "$uk" '{uk:$uk[0]}'
  fi
}

# --- 2. Завантаження документів --------------------------------------------
for row in "${DOCUMENTS[@]}"; do
  IFS='|' read -r slug title_uk title_en base <<<"$row"
  echo; echo "── $slug ──"

  content="$(build_content "$base")" || exit 1

  create_payload="$(jq -n \
    --arg slug "$slug" --arg tuk "$title_uk" --arg ten "$title_en" \
    --argjson content "$content" \
    '{slug:$slug, title:{uk:$tuk, en:$ten}, content:$content,
      changelog:"Initial import"}')"

  resp="$(req POST "$BASE_URL/api/admin/documents" "$create_payload")"
  code="$(tail -n1 <<<"$resp")"; payload="$(sed '$d' <<<"$resp")"

  case "$code" in
    201)
      echo "✓ Створено (id=$(jq -r '.id' <<<"$payload"))" ;;
    409)
      echo "• Уже існує — додаю нову версію…"
      id="$(doc_id_by_slug "$slug")"
      [[ -n "$id" ]] || { echo "✗ Не знайшов id для $slug"; exit 1; }
      ver_payload="$(jq -n --argjson content "$content" \
        '{content:$content, changelog:"Re-import"}')"
      r2="$(req POST "$BASE_URL/api/admin/documents/$id/versions" "$ver_payload")"
      c2="$(tail -n1 <<<"$r2")"; p2="$(sed '$d' <<<"$r2")"
      [[ "$c2" == "201" ]] && echo "✓ Нова версія активована (id=$id)" \
        || { echo "✗ Версія не збереглась ($c2): $p2"; exit 1; } ;;
    *)
      echo "✗ Помилка ($code): $payload"; exit 1 ;;
  esac
done

echo; echo "Готово. Перевірка:"
echo "  curl $BASE_URL/api/uk/documents/by-slug/privacy-policy | jq"
echo "  curl $BASE_URL/api/uk/documents/by-slug/public-offer  | jq"
