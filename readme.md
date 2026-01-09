# Kudora

Kudora est une blockchain basée sur **Cosmos SDK** (CometBFT) avec support **EVM** et **CosmWasm**.

## Repères

- Binaire du nœud : `kudorad`
- Chain ID (dev par défaut) : `kudora_12000-1`
- Denom par défaut : `kud`
- Home par défaut : `~/.kudora`

## Prérequis

- Go `1.24+` (voir `go.mod`)
- [Ignite CLI](https://ignite.com/cli) (pour `ignite chain serve` et la génération proto)
- Outils utiles pour les scripts : `jq`, `curl`

## Démarrage rapide (dev)

### Option A — via Ignite (devnet)

La config du devnet (comptes, faucet, validators, paramètres genesis) est dans `config.yml`.

```bash
ignite chain serve
```

### Option B — builder/installer le binaire

Installe `kudorad` dans votre `GOBIN`/`GOPATH/bin` avec les `ldflags` de version.

```bash
make install
kudorad version
```

## EVM (JSON-RPC) en local

Un helper est fourni pour démarrer un nœud local avec JSON-RPC EVM activé :

```bash
./scripts/start_evm.sh --clean --fast --dev-account
```

Endpoints (par défaut) :

- Cosmos RPC : `http://localhost:26657`
- Cosmos REST : `http://localhost:1317`
- EVM JSON-RPC : `http://localhost:8545`
- EVM WebSocket : `ws://localhost:8546`

## Tests

### Unit / race / coverage

```bash
make test
make test-unit
make test-race
make test-cover
make bench
```

`make test` exécute aussi `go vet` et `govulncheck`.

### Script d’intégration (Cosmos + EVM)

Le script `./scripts/test_chain.sh` lance un test “end-to-end” et **réinitialise** le home de test (`~/.kudora`).

```bash
./scripts/test_chain.sh
```

Prérequis : `kudorad` dans le `PATH`, `jq`, `curl`.

## Lint & hygiène

```bash
make lint
make lint-fix
make govulncheck
```

## Protobuf

Si vous ne souhaitez pas utiliser la génération via `ignite chain serve`, vous pouvez régénérer les protos Go :

```bash
make proto-gen
```

## OpenAPI (spec & console)

- La spec OpenAPI est versionnée dans `docs/static/openapi.json`.
- Le package `docs` contient un handler (console + spec) qui peut être branché sur un router HTTP (`RegisterOpenAPIService`).

## Commandes utiles du binaire

Le CLI expose (entre autres) :

```bash
kudorad --help
kudorad start --help
kudorad query --help
kudorad tx --help
```

Pour des scénarios avancés :

- `kudorad in-place-testnet ...` (dériver un testnet local à partir d’un state)
- `kudorad multi-node ...` (générer des dossiers de config pour un testnet multi-validateurs)

## Bonnes pratiques (dev vs prod)

- Ne pas exposer JSON-RPC/WS (`8545/8546`) sur Internet en configuration dev.
- Éviter `--keyring-backend test` en environnement partagé / prod.
- Ajuster `minimum-gas-prices` pour éviter les transactions “free” hors dev.
- Garder `config.yml` et les scripts comme **outils de dev** ; pour un réseau réel, préparez un `genesis.json` et des configs `app.toml`/`config.toml` adaptés.

## Release

La version du binaire est dérivée d’un tag Git (sinon `branch-commit`). Pour préparer une release :

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Ressources

- Cosmos SDK : https://docs.cosmos.network
- Ignite CLI : https://docs.ignite.com
