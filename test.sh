set -uo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
TOTAL="${TOTAL:-20}"
EXPECTED_LIMIT="${EXPECTED_LIMIT:-10}"
CLEAN_REDIS="${CLEAN_REDIS:-true}"

BASE_URL="${BASE_URL%/}"

if [ -t 1 ]; then
	GREEN=$'\033[0;32m'
	RED=$'\033[0;31m'
	YELLOW=$'\033[0;33m'
	BOLD=$'\033[1m'
	NC=$'\033[0m'
else
	GREEN=''
	RED=''
	YELLOW=''
	BOLD=''
	NC=''
fi

info() { printf '%s\n' "$*"; }
success() { printf '%s%s%s\n' "$GREEN" "$*" "$NC"; }
warn() { printf '%s%s%s\n' "$YELLOW" "$*" "$NC"; }
fail() {
	printf '%s%s%s\n' "$RED" "$*" "$NC" >&2
	exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl não encontrado."

EXPECTED_BLOCKED=$((TOTAL - EXPECTED_LIMIT))

clean_redis() {
	if [ "$CLEAN_REDIS" != "true" ]; then
		warn "Limpeza do Redis desativada (CLEAN_REDIS=false)."
		return 0
	fi

	if command -v docker >/dev/null 2>&1; then
		info "Limpando chaves ratelimit:* no Redis via docker compose..."
		if docker compose exec -T redis sh -c \
			'redis-cli --scan --pattern "ratelimit:*" | while read -r key; do redis-cli DEL "$key" >/dev/null; done' 2>/dev/null; then
			return 0
		fi
		warn "Não foi possível limpar o Redis via docker compose."
	fi

	if command -v redis-cli >/dev/null 2>&1; then
		info "Limpando chaves ratelimit:* no Redis local..."
		redis-cli --scan --pattern 'ratelimit:*' |
			while read -r key; do
				redis-cli DEL "$key" >/dev/null
			done
		return 0
	fi

	warn "Redis não encontrado para limpeza. Se o IP já estiver bloqueado, aguarde o bloqueio expirar."
}

wait_for_app() {
	local host port hostport

	hostport="${BASE_URL#*://}"
	hostport="${hostport%%/*}"

	if [[ "$hostport" == *:* ]]; then
		host="${hostport%:*}"
		host="${host#[}"
		host="${host%]}"
		port="${hostport##*:}"
	else
		host="$hostport"
		if [[ "$BASE_URL" == https://* ]]; then
			port=443
		else
			port=80
		fi
	fi

	for _ in $(seq 1 30); do
		if (exec 3<>"/dev/tcp/$host/$port") 2>/dev/null; then
			return 0
		fi
		sleep 1
	done

	return 1
}

info "${BOLD}Teste do rate limiter${NC}"
info "  Aplicação:      $BASE_URL"
info "  Requisições:    $TOTAL"
info "  Limite esperado: $EXPECTED_LIMIT"
info ""

info "Aguardando a aplicação responder..."
wait_for_app || fail "A aplicação não respondeu em $BASE_URL."

clean_redis

status_file=$(mktemp)
trap 'rm -f "$status_file"' EXIT

# Requisições concorrentes para garantir que todas ocorram dentro da janela de 1 segundo.
# Um único processo curl com --parallel evita o custo de iniciar um processo por requisição.
info "Enviando $TOTAL requisições..."
curl_args=()
for _ in $(seq 1 "$TOTAL"); do
	curl_args+=(-o /dev/null -w '%{http_code}\n' "$BASE_URL/")
done

curl -s --parallel --parallel-max "$TOTAL" "${curl_args[@]}" >"$status_file"

stats=$(awk '
	$0 == "200" { allowed++; next }
	$0 == "429" { blocked++; next }
	{ other++ }
	END { printf "%d %d %d\n", allowed, blocked, other }
' "$status_file")
read -r allowed blocked other <<<"$stats"

info ""
info "${BOLD}Resultado${NC}"
info "  HTTP 200 (permitidas):          $allowed"
info "  HTTP 429 (bloqueadas):          $blocked"
info "  Outros status:                  $other"
info ""
info "Distribuição dos status:"
sort "$status_file" | uniq -c | awk '{ printf "  %s x %s\n", $1, $2 }'

if [ "$blocked" -gt 0 ]; then
	info ""
	info "${BOLD}Resposta de bloqueio (requisição extra de verificação)${NC}"
	curl -s -i "$BASE_URL/" | sed -n '1,8p'
fi

info ""
if [ "$allowed" -eq "$EXPECTED_LIMIT" ] &&
	[ "$blocked" -eq "$EXPECTED_BLOCKED" ] &&
	[ "$other" -eq 0 ]; then
	success "OK: $allowed requisições permitidas e $blocked bloqueadas, como esperado."
	exit 0
fi

fail "Resultado inesperado: esperado $EXPECTED_LIMIT permitidas e $EXPECTED_BLOCKED bloqueadas."