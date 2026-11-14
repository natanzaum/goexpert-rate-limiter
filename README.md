# Rate Limiter — Go Expert

Middleware de **Rate Limiting** para Go, com persistência em Redis, suporte a limite por IP e por token (`API_KEY`) e bloqueio temporário de quem excede o limite.

Arquitetura:

- **Domínio** (`internal/domain`): regras de negócio, precedência Token > IP e o contrato da estratégia de persistência.
- **Estratégias** (`internal/strategy`): implementação de persistência (`redis`), substituível sem alterar o domínio.
- **Middleware** (`internal/middleware`): extração de IP/token e tradução da decisão em resposta HTTP.
- **Configuração** (`internal/config`): leitura de variáveis de ambiente e `.env`.
- **Servidor** (`cmd/server`): bootstrap, graceful shutdown e composição das camadas.

### Resposta em caso de bloqueio

- **Status:** `429 Too Many Requests`
- **Content-Type:** `text/plain; charset=utf-8`
- **Body (exato):**
  ```
  you have reached the maximum number of requests or actions allowed within a certain time frame
  ```

## Configuração

Todas as configurações são feitas por variáveis de ambiente (ou por um arquivo `.env` na raiz do projeto — veja [`.env.example`](.env.example)).

| Variável | Padrão | Descrição |
| --- | --- | --- |
| `PORT` | `8080` | Porta HTTP da aplicação. |
| `REDIS_ADDR` | `localhost:6379` | Endereço do Redis. No Docker Compose use `redis:6379`. |
| `REDIS_PASSWORD` | vazio | Senha do Redis, se houver. |
| `REDIS_DB` | `0` | Database do Redis. |
| `RATE_LIMIT_IP` | `10` | Máximo de requisições por segundo por IP. |
| `RATE_LIMIT_BLOCK_TIME` | `5m` | Duração do bloqueio após exceder o limite (ex.: `5m`, `30s`). |
| `API_KEY_HEADER` | `API_KEY` | Header usado para ler o token de acesso. |
| `RATE_LIMIT_TOKENS` | `{}` | JSON com o limite por token. Ex.: `{"token-100":100,"token-50":50}`. |

## Executando com Docker

Requisitos: Docker e Docker Compose.

```bash
docker compose up --build
```

Isso sobe:

- a aplicação em `http://localhost:8080`;
- o Redis em `localhost:6379` com healthcheck e volume persistente.

Para encerrar:  

```bash
docker compose down
```

## Executando localmente

1. Suba apenas o Redis:

   ```bash
   docker compose up -d redis
   ```

2. Configure o `.env` (copie de `.env.example`) apontando para `localhost:6379`.

3. Execute:

   ```bash
   go run ./cmd/server
   ```

## Endpoints

| Método | Rota | Descrição |
| --- | --- | --- |
| `GET` | `/` | Endpoint de exemplo (sujeito ao rate limiter). |

## Script de teste

O repositório inclui `test.sh`, que valida o limite configurado com 20 requisições:

```bash
docker compose up -d
./test.sh
```

O script:

1. aguarda a aplicação responder;
2. limpa as chaves `ratelimit:*` do Redis (para permitir repetir o teste);
3. envia 20 requisições concorrentes para `/`;
4. valida que **10** foram permitidas (`200`) e **10** bloqueadas (`429`);
5. encerra com código de saída diferente de zero se o resultado não for o esperado.

Variáveis suportadas:

```bash
BASE_URL=http://localhost:8080 TOTAL=20 EXPECTED_LIMIT=10 CLEAN_REDIS=true ./test.sh
```

Use `CLEAN_REDIS=false` para não limpar o Redis antes do teste.