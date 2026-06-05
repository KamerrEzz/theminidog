# Guía de internos para contribuidores

Esta guía no explica qué es observabilidad. Asume que ya sabés Go y que entendés el problema de dominio. Lo que enseña es cómo está construido MiniObserv por dentro: dónde vive cada cosa, por qué está así, y cómo extenderlo sin romper nada.

---

## 1. Mapa del código

```
theminidog/
│
├── cmd/
│   ├── agent/main.go          — Composition root del agente: carga config, cablea deps, arranca Agent.Run
│   └── server/main.go         — Composition root del servidor: migraciones, pool pgx, router, graceful shutdown
│
├── internal/
│   ├── model/
│   │   ├── metric.go          — Struct Metric + MetricBatch + Validate(); la allowlist de nombres canónicos
│   │   └── log.go             — Struct LogEntry y su validación
│   │
│   ├── config/
│   │   ├── agent.go           — LoadAgent(): config del agente desde env vars
│   │   └── server.go          — LoadServerConfig(): config del servidor desde env vars
│   │
│   ├── agent/
│   │   ├── agent.go           — Agent: dos goroutines (collectLoop + senderLoop) + channel de batches
│   │   ├── collector/
│   │   │   ├── collector.go   — Interfaz Collector + Registry.CollectAll
│   │   │   ├── cpu.go         — CPUCollector con statFn inyectable
│   │   │   ├── memory.go      — MemoryCollector con statFn inyectable
│   │   │   ├── disk.go        — DiskCollector con statFn inyectable
│   │   │   └── network.go     — NetworkCollector con statFn inyectable
│   │   ├── sender/
│   │   │   └── sender.go      — HTTPSender: POST con backoff exponencial + jitter
│   │   └── logtail/
│   │       └── parser.go      — Parser de archivos de log línea por línea
│   │
│   └── server/
│       ├── server.go          — Wrapper sobre net/http con graceful shutdown
│       ├── api/
│       │   ├── router.go      — Wiring chi: middleware global + grupo JWT + rutas
│       │   ├── middleware.go  — JWTMiddleware con WithValidMethods (bloquea alg=none)
│       │   ├── metrics.go     — HandleIngest + HandleQuery (handlers funcionales)
│       │   ├── health.go      — HandleHealthz + HandleReadyz
│       │   └── errors.go      — writeError: helper para respuestas JSON de error
│       └── storage/
│           └── metrics.go     — Interfaz MetricRepository + implementación pgxMetricRepository
```

**Regla de navegación:** si querés cambiar cómo se recolecta una métrica → `internal/agent/collector/`. Si querés cambiar cómo se persiste → `internal/server/storage/metrics.go`. Si querés agregar un endpoint → `internal/server/api/metrics.go` + `router.go`. Si querés cambiar qué es válido → `internal/model/metric.go`.

---

## 2. La regla de capas

Las dependencias siguen esta jerarquía estricta:

```
cmd/*  (composition root — sabe de todo, no es importado por nadie)
  └─ internal/*/
       ├─ model/    — cero deps de este proyecto, solo stdlib
       ├─ config/   — solo stdlib (os, time, net/url, strconv)
       ├─ agent/    — puede importar: model, config, gopsutil
       └─ server/   — puede importar: model, config, pgx, chi, jwt
```

La regla crítica: **`server` no puede importar `agent`, y `agent` no puede importar `server`**. Si se cruzan, hay una dependencia circular y `go build` falla. Cada binario tiene un composition root distinto en `cmd/` que importa ambos y los conecta.

Para verificar qué importa cada paquete en cualquier momento:

```bash
go list -f '{{ .ImportPath }}: {{ join .Imports " " }}' ./...
```

La razón de fondo: `model/` define los tipos que viajan entre capas (`Metric`, `MetricBatch`). Que no tenga deps internas garantiza que cualquier capa puede importarlo sin crear ciclos. Es el contrato compartido del sistema.

---

## 3. El modelo estrecho

`internal/model/metric.go` — léelo completo:

```go
var validMetricNames = map[string]struct{}{
    "cpu.usage_pct":    {},
    "mem.used_pct":     {},
    "mem.used_bytes":   {},
    "mem.total_bytes":  {},
    "disk.used_pct":    {},
    "disk.used_bytes":  {},
    "disk.total_bytes": {},
    "net.bytes_in":     {},
    "net.bytes_out":    {},
}

type Metric struct {
    Time   time.Time         `json:"time"`
    Host   string            `json:"host"`
    Name   string            `json:"name"`
    Value  float64           `json:"value"`
    Labels map[string]string `json:"labels,omitempty"`
}

func (m Metric) Validate() error {
    if strings.TrimSpace(m.Host) == "" {
        return fmt.Errorf("metric host must not be empty")
    }
    if _, ok := validMetricNames[m.Name]; !ok {
        return fmt.Errorf("unknown metric name %q", m.Name)
    }
    if m.Time.IsZero() {
        return fmt.Errorf("metric time must not be zero")
    }
    if math.IsNaN(m.Value) || math.IsInf(m.Value, 0) {
        return fmt.Errorf("metric value must be finite")
    }
    for k, v := range m.Labels {
        if k == "" || v == "" {
            return fmt.Errorf("metric label key and value must not be empty")
        }
    }
    return nil
}
```

**Por qué la allowlist de 9 nombres.** Sin allowlist, un agente mal configurado podría enviar `cpu_usage_percent` o `CPU.Usage` y el dato llegaría a la base de datos pero nunca aparecería en ninguna query. El error sería silencioso en producción. La allowlist lo convierte en un error explícito inmediato, tanto en el sender del agente como en el handler del servidor.

**Por qué `Labels map[string]string`.** Las labels permiten distinguir métricas del mismo tipo: `core=total` vs `core=0` en CPU, `mount=/` vs `mount=/data` en disco. Usar un mapa sin esquema fijo significa que no necesitás una columna por cada dimensión posible. En PostgreSQL/TimescaleDB se guarda como JSONB, lo que permite queries del tipo `WHERE labels->>'core' = 'total'` sin necesidad de joins extra ni migraciones para agregar nuevas dimensiones.

**Por qué `Validate()` está en el modelo y no en el handler.** El mismo `Validate()` se llama en dos lugares distintos del sistema:
- En `cmd/agent`: el sender valida antes de serializar (ver `MetricBatch.Validate()` en `sender.go`)
- En `internal/server/api/metrics.go`: el handler valida el batch recibido por HTTP

Si la validación estuviera solo en el handler, el agente podría construir y encolar batches inválidos localmente sin enterarse hasta recibir un 400 del servidor. Al vivir en el modelo, ambos lados del sistema comparten la misma lógica sin duplicación.

**Nota importante:** `internal/server/storage/metrics.go` también tiene su propia `validMetricNames`. Ambos mapas deben mantenerse sincronizados cuando se agregan métricas nuevas. Es la única duplicación deliberada del proyecto, y hay un comentario que lo documenta.

---

## 4. El patrón Collector: inyección de statFn

`internal/agent/collector/cpu.go`:

```go
type CPUCollector struct {
    host   string
    statFn func(ctx context.Context, interval time.Duration, percpu bool) ([]float64, error)
}

func NewCPUCollector(host string) *CPUCollector {
    return &CPUCollector{
        host:   host,
        statFn: gopsutilcpu.PercentWithContext, // llamada real al OS
    }
}

func (c *CPUCollector) Collect(ctx context.Context) ([]model.Metric, error) {
    totals, err := c.statFn(ctx, 0, false)   // aggregate
    // ...
    perCore, err := c.statFn(ctx, 0, true)   // per-core
    // ...
}
```

**Por qué `statFn` es un campo del struct y no una llamada directa a gopsutil.** Si `Collect()` llamara directamente a `gopsutilcpu.PercentWithContext`, cada test estaría ejecutando syscalls reales del OS. En una máquina de CI sin acceso a `/proc/stat` o en un contenedor restringido, esos tests fallarían por razones ajenas a la lógica del código.

Al inyectar la función, los tests reemplazan el syscall por un stub determinístico:

```go
// En el test (cpu_test.go):
statFn := makeCPUStatFn(
    []float64{40.0}, nil,      // aggregate: 40%, sin error
    []float64{30.0, 50.0}, nil, // per-core: core0=30%, core1=50%
)
c := &CPUCollector{host: "test-host", statFn: statFn}

metrics, err := c.Collect(context.Background())
// metrics tiene 3 elementos: total + core0 + core1
// Cero syscalls. Cero deps del OS. Corre en microsegundos.
```

Lo que se está testeando es la lógica de **transformación**: que el collector construya los `model.Metric` correctos con los labels adecuados a partir de los floats que devuelve la función de OS. Esa lógica es la única que le pertenece al collector; el dato crudo del OS no le pertenece a él.

### Cómo agregar un nuevo collector

1. **Crear `internal/agent/collector/tunombre.go`**

```go
package collector

import (
    "context"
    "time"
    "github.com/kamerrezz/theminidog/internal/model"
)

type TuNombreCollector struct {
    host   string
    statFn func(ctx context.Context) (TuTipoDeDato, error)
}

func NewTuNombreCollector(host string) *TuNombreCollector {
    return &TuNombreCollector{
        host:   host,
        statFn: tuBibliotecaReal.FuncionReal, // gopsutil o lo que uses
    }
}

func (c *TuNombreCollector) Name() string { return "tunombre" }

func (c *TuNombreCollector) Collect(ctx context.Context) ([]model.Metric, error) {
    dato, err := c.statFn(ctx)
    if err != nil {
        return nil, fmt.Errorf("tunombre: %w", err)
    }
    return []model.Metric{
        {
            Time:  time.Now().UTC(),
            Host:  c.host,
            Name:  "tunombre.metrica",
            Value: float64(dato.Valor),
        },
    }, nil
}
```

2. **Agregar el nombre de métrica a `internal/model/metric.go`** dentro de `validMetricNames` (y también en el mapa equivalente de `internal/server/storage/metrics.go`).

3. **Wirear en `cmd/agent/main.go`** dentro de `collector.NewRegistry(...)`:

```go
reg := collector.NewRegistry(
    collector.NewCPUCollector(cfg.AgentHost),
    collector.NewMemoryCollector(cfg.AgentHost),
    collector.NewDiskCollector(cfg.AgentHost, cfg.DiskMounts),
    collector.NewNetworkCollector(cfg.AgentHost, cfg.NetIfaces),
    collector.NewTuNombreCollector(cfg.AgentHost), // ← acá
)
```

4. **Escribir el test** con un stub que reemplace `statFn`, sin syscalls reales.

---

## 5. El agente de dos goroutines

`internal/agent/agent.go` — la función `Run`:

```go
func (a *Agent) Run(ctx context.Context) {
    var wg sync.WaitGroup
    wg.Add(2)

    go func() {
        defer wg.Done()
        defer close(a.batches) // señaliza a senderLoop que no hay más datos
        a.collectLoop(ctx)
    }()

    go func() {
        defer wg.Done()
        a.senderLoop(ctx)
    }()

    wg.Wait()
}
```

Las dos goroutines y el channel entre ellas:

```
collectLoop                │  channel (buf=10)   │  senderLoop
───────────────────────────┼─────────────────────┼────────────────────
ticker.C dispara           │                     │
CollectAll(ctx)            │                     │
                           │                     │
select {                   │                     │
  case batches <- batch:   │ ──────────────────► │ for batch := range a.batches {
  default: drop (log warn) │ ¿lleno? drop newest │     sender.Send(ctx, batch)
}                          │                     │ }
```

**El buffer de 10 y la política drop-newest.** Cuando el servidor está caído, `senderLoop` se bloquea reintentando. `collectLoop` sigue corriendo para no detener la recolección. Si el channel se llena (10 batches acumulados), el select no-bloqueante cae al `default` y descarta el batch más reciente. ¿Por qué el más reciente y no el más viejo? Porque cuando el servidor vuelve a estar disponible, los datos más importantes para diagnosticar la caída son los del inicio del problema, no los más nuevos.

**El cierre del channel y el drenaje gracioso.** Cuando el contexto se cancela:
1. `collectLoop` sale del `for/select` y ejecuta su `defer close(a.batches)`
2. `senderLoop` tiene un `for range a.batches` que termina naturalmente cuando el channel se cierra Y está vacío — drena lo que quede antes de salir
3. Ambas goroutines completan, `wg.Wait()` retorna, `Run` retorna

Si `close(a.batches)` no estuviera, `senderLoop` quedaría bloqueado para siempre esperando datos que nunca llegan.

---

## 6. Backoff exponencial con jitter

`internal/agent/sender/sender.go` — la función `waitFor`:

```go
func waitFor(attempt int, cfg BackoffConfig, randFn func() float64) time.Duration {
    if attempt == 0 {
        return 0
    }
    exp := math.Min(float64(cfg.Max), float64(cfg.Base)*math.Pow(2, float64(attempt-1)))
    jitter := 1.0 + cfg.Jitter*(randFn()*2-1)
    d := time.Duration(exp * jitter)
    if d > cfg.Max {
        d = cfg.Max
    }
    return d
}
```

Con los defaults de producción (`Base=1s, Max=60s, Jitter=0.25`):

| attempt | base exponencial | rango con jitter ±25%  |
|---------|-----------------|------------------------|
| 0       | 0               | 0 (primer intento inmediato) |
| 1       | 1s              | [0.75s, 1.25s]         |
| 2       | 2s              | [1.5s, 2.5s]           |
| 3       | 4s              | [3s, 5s]               |
| 7+      | ≥64s → cap 60s  | [45s, 75s]             |

**Por qué jitter.** Si 100 agentes arrancan al mismo tiempo (reinicio masivo, deploy), sin jitter todos calculan exactamente el mismo tiempo de espera y reintentarían al mismo segundo — thundering herd. El servidor recibe 100 requests simultáneos justo cuando está tratando de recuperarse. El jitter ±25% distribuye los reintentos en una ventana de tiempo, reduciendo la carga pico.

**Por qué `randFn` es inyectable.** Lo mismo que `statFn` en los collectors: los tests no quieren esperar segundos reales. Con `withSleepFn(noopSleep)` y `withRandFn(constRand)` el test `TestWaitFor_table` verifica el comportamiento del backoff sin delays:

```go
// sender_test.go
func TestSend_503x2_then_202(t *testing.T) {
    srv, count := responseSequence(t, []int{503, 503, 202})

    cfg := BackoffConfig{Base: time.Millisecond, Max: 10 * time.Millisecond, Jitter: 0}
    s := NewHTTPSender(srv.URL, cfg, nil).
        withRandFn(constRand).   // jitter = 0, determinístico
        withSleepFn(noopSleep)   // sin delays reales

    err := s.Send(context.Background(), makeBatch())
    // err == nil, count == 3
}
```

**Errores permanentes vs transientes.** Un 4xx no se reintenta — el servidor rechazó el batch definitivamente. Un 5xx o error de red sí se reintenta. La distinción la hace el tipo `permanentError`:

```go
case resp.StatusCode >= 400 && resp.StatusCode < 500:
    return permanentError{fmt.Errorf("server rejected batch: %d", resp.StatusCode)}
```

---

## 7. El middleware JWT

`internal/server/api/middleware.go` — completo:

```go
func JWTMiddleware(secret []byte) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            authHeader := r.Header.Get("Authorization")
            if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
                writeError(w, http.StatusUnauthorized, "unauthorized")
                return
            }
            tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
            if tokenStr == "" {
                writeError(w, http.StatusUnauthorized, "unauthorized")
                return
            }
            _, err := jwt.ParseWithClaims(
                tokenStr,
                &jwt.RegisteredClaims{},
                func(t *jwt.Token) (any, error) { return secret, nil },
                jwt.WithValidMethods([]string{"HS256"}), // CRÍTICO — ver abajo
            )
            if err != nil {
                writeError(w, http.StatusUnauthorized, "unauthorized")
                return
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

**`strings.HasPrefix(authHeader, "Bearer ")` antes del parsing.** Es un fast path. Verificar el prefijo en memoria es trivial; parsear el JWT implica decodificar base64 y verificar la firma HMAC. Rechazar mal-formados antes del parsing evita carga innecesaria.

**`jwt.WithValidMethods([]string{"HS256"})` — por qué es obligatorio.** El estándar JWT permite que el header del token especifique el algoritmo. Sin esta opción, una librería vulnerable aceptaría:

```
Header: {"alg":"none","typ":"JWT"}
```

Un JWT con `alg=none` no tiene firma. El atacante puede fabricar cualquier payload con cualquier `sub`/`role` y el servidor lo aceptaría como válido. Con `WithValidMethods`, la librería rechaza cualquier token cuyo `alg` no sea `HS256`, con independencia de lo que diga el header:

```
Sin WithValidMethods: JWT con alg=none → acepta (firma omitida, claims se creen)
Con  WithValidMethods: JWT con alg=none → rechaza → 401
```

Es el ataque de substitución de algoritmo (algorithm confusion). Una línea de código, impacto crítico en seguridad.

---

## 8. El patrón Repository

`internal/server/storage/metrics.go`:

```go
// La interfaz vive en el mismo paquete que la implementación.
type MetricRepository interface {
    Insert(ctx context.Context, batch model.MetricBatch) (int, error)
    Query(ctx context.Context, params QueryParams) ([]QueryPoint, error)
    Ping(ctx context.Context) error
}

// La implementación es unexported.
type pgxMetricRepository struct {
    pool *pgxpool.Pool
}

// El constructor devuelve la interfaz, no el struct concreto.
func NewMetricRepository(pool *pgxpool.Pool) MetricRepository {
    return &pgxMetricRepository{pool: pool}
}
```

**Tres decisiones de diseño en 10 líneas.**

Primero: la interfaz vive donde se usa, no donde se implementa. Esto es idiomático en Go — las interfaces se definen junto al consumidor. `api/metrics.go` y `api/router.go` dependen de `storage.MetricRepository`; si mañana hubiera una implementación alternativa (Redis, SQLite, in-memory para tests de integración), se podría inyectar sin cambiar el código del handler.

Segundo: `pgxMetricRepository` es unexported. El código que llama a `NewMetricRepository` recibe un `MetricRepository` — una interfaz. No puede acceder a los campos del struct ni a métodos privados. La encapsulación es real, no cosmética.

Tercero: los handlers dependen de la interfaz, lo que permite el `fakeRepo` de los tests:

```go
// api/testhelpers_test.go
type fakeRepo struct {
    insertN   int
    insertErr error
    queryPts  []storage.QueryPoint
    queryErr  error
}

func (f *fakeRepo) Insert(_ context.Context, _ model.MetricBatch) (int, error) {
    return f.insertN, f.insertErr
}
// ... Ping, Query
```

**`pgx.Batch` para el insert.** Cada `model.Metric` en el batch se encola con `b.Queue(...)` y todo se envía al servidor en un único round-trip TCP con `r.pool.SendBatch(ctx, b)`. Sin Batch, insertar 100 métricas requeriría 100 round-trips.

**`defer br.Close()` — el gotcha más común de pgx.** El `BatchResults` mantiene una conexión abierta del pool mientras existe. Si no se llama a `Close()`:

```go
br := r.pool.SendBatch(ctx, b)
// ← si olvidás defer br.Close() y hay un return temprano por error:
for i := 0; i < b.Len(); i++ {
    if _, err := br.Exec(); err != nil {
        return i, err // ← salida temprana, la conexión NUNCA se libera
    }
}
```

Bajo carga, el pool se agota. Todos los requests nuevos quedan esperando una conexión disponible que nunca llega. El servicio parece "colgado" sin errores obvios. `defer br.Close()` garantiza el release independientemente de cómo salga la función.

**Por qué sin ORM.** La query de lectura usa `time_bucket`, función específica de TimescaleDB:

```sql
SELECT time_bucket('5 minutes', time) AS bucket,
       avg(value) AS value
FROM metrics
WHERE host = $1 AND name = $2 AND time >= $3 AND time <= $4
GROUP BY bucket
ORDER BY bucket DESC
```

Ningún ORM mainstream (GORM, sqlx) soporta `time_bucket`. Un ORM haría el mismo trabajo con más complejidad y menos control.

**Interpolación segura de bucket y agg.** Puede llamar la atención que `bucketLiteral` y `aggFn` se interpolen directamente en el string de la query con `fmt.Sprintf`. Esto es seguro porque ambos valores vienen exclusivamente de los mapas `validBuckets` y `validAggs` — mapas cuyo contenido está hardcodeado en el código fuente. Nunca se interpola input del usuario directamente. Los parámetros del usuario (`host`, `name`, `from`, `to`) siempre van como `$1`, `$2`, `$3`, `$4`.

---

## 9. Queries dinámicas con builder seguro

Si en el futuro se necesitan filtros opcionales (por ejemplo, en un endpoint de logs con `host?`, `level?`, `from?`), el patrón a usar es el del query builder. El mismo patrón se puede ver como referencia conceptual:

```go
var conds []string
var args []any
n := 0

add := func(cond string, val any) {
    n++
    conds = append(conds, fmt.Sprintf(cond, n))
    args = append(args, val)
}

if params.Host != "" {
    add("host = $%d", params.Host)
}
if params.Level != "" {
    add("level = $%d", params.Level)
}

query := "SELECT * FROM logs"
if len(conds) > 0 {
    query += " WHERE " + strings.Join(conds, " AND ")
}

rows, err := pool.Query(ctx, query, args...)
```

**Por qué es seguro frente a SQL injection.** La estructura del WHERE se construye en Go con `$N` — placeholders, no valores. Los valores reales van en `args`. El driver pgx envía query y args por separado; el servidor PostgreSQL nunca los concatena textualmente. Un usuario que enviara `'; DROP TABLE metrics; --` como `host` recibiría ese string en `$1` como un dato opaco, no como SQL.

---

## 10. Filosofía de testing

El proyecto tiene tres capas de tests, cada una con un propósito y una velocidad distintos.

### Capa 1: unit puro (sin I/O)

Sin base de datos, sin red, sin syscalls del OS. Corren en milisegundos.

- `internal/model/` — validan la lógica de `Validate()` directamente
- `internal/config/` — validan parsing de env vars con `os.Setenv` en el test
- `internal/agent/collector/` — usan `statFn` inyectable, cero OS real
- `internal/agent/sender/` — usan `withSleepFn(noopSleep)` y un `httptest.Server`

Ejemplo del collector:

```go
// cpu_test.go — test puro, sin gopsutil
func TestCPUCollector_Collect_ReturnsAggregateAndPerCore(t *testing.T) {
    statFn := makeCPUStatFn(
        []float64{40.0}, nil,
        []float64{30.0, 50.0}, nil,
    )
    c := &CPUCollector{host: "test-host", statFn: statFn}

    metrics, err := c.Collect(context.Background())
    if err != nil {
        t.Fatalf("expected no error, got: %v", err)
    }
    if len(metrics) != 3 {
        t.Fatalf("expected 3 metrics (total + 2 cores), got %d", len(metrics))
    }
}
```

### Capa 2: unit con httptest

Los handlers HTTP se testean con `httptest.NewRecorder()`. Sin HTTP real, sin base de datos real — el repo se reemplaza por `fakeRepo`.

```go
// api/metrics_test.go — test del handler, sin red ni DB
func TestHandleIngest_ValidBatch(t *testing.T) {
    repo := &fakeRepo{insertN: 3}
    handler := api.HandleIngest(repo)

    batch := makeBatch("web-01", 3)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/metrics", encodeJSON(t, batch))
    req.Header.Set("Content-Type", "application/json")
    rr := httptest.NewRecorder()
    handler.ServeHTTP(rr, req)

    if rr.Code != http.StatusAccepted {
        t.Fatalf("expected 202, got %d", rr.Code)
    }
}
```

### Capa 3: integración (desactivada por defecto)

```go
//go:build integration

package storage_test

func TestMetricRepository_Integration(t *testing.T) {
    dbURL := os.Getenv("TEST_DATABASE_URL")
    if dbURL == "" {
        t.Skip("TEST_DATABASE_URL not set — skipping integration test")
    }
    // TimescaleDB real, pgx real, SQL real
}
```

La build tag `//go:build integration` hace que este archivo sea invisible para `go test ./...`. Solo se compila con `-tags=integration`:

```bash
# Suite rápida (sin DB) — siempre pasa en local
go test ./...

# Suite completa (con TimescaleDB en Docker)
TEST_DATABASE_URL=postgres://... go test -tags=integration ./...
```

En CI se levanta un contenedor de TimescaleDB para la suite de integración. Los tests de capas 1 y 2 no necesitan ningún servicio externo.

---

## 11. Agregar un nuevo endpoint paso a paso

Ejemplo concreto: `GET /api/v1/metrics/hosts` que devuelve todos los hosts únicos que tienen datos.

### Paso 1: agregar el método a la interfaz

`internal/server/storage/metrics.go`:

```go
type MetricRepository interface {
    Insert(ctx context.Context, batch model.MetricBatch) (int, error)
    Query(ctx context.Context, params QueryParams) ([]QueryPoint, error)
    Ping(ctx context.Context) error
    Hosts(ctx context.Context) ([]string, error) // ← nuevo
}
```

### Paso 2: implementar en pgxMetricRepository

`internal/server/storage/metrics.go`:

```go
func (r *pgxMetricRepository) Hosts(ctx context.Context) ([]string, error) {
    rows, err := r.pool.Query(ctx, `SELECT DISTINCT host FROM metrics ORDER BY host`)
    if err != nil {
        return nil, fmt.Errorf("query hosts: %w", err)
    }
    defer rows.Close()

    var hosts []string
    for rows.Next() {
        var h string
        if err := rows.Scan(&h); err != nil {
            return nil, fmt.Errorf("scan host: %w", err)
        }
        hosts = append(hosts, h)
    }
    if err := rows.Err(); err != nil {
        return nil, fmt.Errorf("iterate hosts: %w", err)
    }
    return hosts, nil
}
```

### Paso 3: crear el handler

`internal/server/api/metrics.go`:

```go
func HandleHosts(repo storage.MetricRepository) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        hosts, err := repo.Hosts(r.Context())
        if err != nil {
            writeError(w, http.StatusInternalServerError, "query error")
            return
        }
        if hosts == nil {
            hosts = []string{} // devolver [] no null
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]any{"hosts": hosts})
    }
}
```

### Paso 4: registrar la ruta

`internal/server/api/router.go`:

```go
r.Group(func(r chi.Router) {
    r.Use(JWTMiddleware(jwtSecret))
    r.Post("/api/v1/metrics", HandleIngest(repo))
    r.Get("/api/v1/metrics/query", HandleQuery(repo))
    r.Get("/api/v1/metrics/hosts", HandleHosts(repo)) // ← nuevo
})
```

### Paso 5: agregar el método al fakeRepo

`internal/server/api/testhelpers_test.go`:

```go
type fakeRepo struct {
    pingErr   error
    insertN   int
    insertErr error
    queryPts  []storage.QueryPoint
    queryErr  error
    hosts     []string // ← nuevo
    hostsErr  error    // ← nuevo
}

func (f *fakeRepo) Hosts(_ context.Context) ([]string, error) {
    return f.hosts, f.hostsErr
}
```

### Paso 6: escribir el test del handler

`internal/server/api/metrics_test.go`:

```go
func TestHandleHosts_ReturnsList(t *testing.T) {
    repo := &fakeRepo{hosts: []string{"web-01", "web-02"}}
    handler := api.HandleHosts(repo)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/hosts", nil)
    rr := httptest.NewRecorder()
    handler.ServeHTTP(rr, req)

    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rr.Code)
    }
    var resp map[string][]string
    mustDecode(t, rr.Body, &resp)
    if len(resp["hosts"]) != 2 {
        t.Fatalf("expected 2 hosts, got %d", len(resp["hosts"]))
    }
}

func TestHandleHosts_EmptyList(t *testing.T) {
    repo := &fakeRepo{hosts: nil}
    handler := api.HandleHosts(repo)

    req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/hosts", nil)
    rr := httptest.NewRecorder()
    handler.ServeHTTP(rr, req)

    if rr.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", rr.Code)
    }
    var resp map[string][]string
    mustDecode(t, rr.Body, &resp)
    if resp["hosts"] == nil || len(resp["hosts"]) != 0 {
        t.Fatalf("expected empty array, got %v", resp["hosts"])
    }
}
```

Ese es el camino completo: interfaz → implementación → handler → router → stub → test. Cada paso es mecánico una vez que entendés el patrón.

---

## 12. El patrón de configuración

`internal/config/agent.go` — `LoadAgent()`:

```go
func LoadAgent() (AgentConfig, error) {
    // Variable requerida: falla con error (el caller hace os.Exit(1))
    rawURL := os.Getenv("SERVER_URL")
    if rawURL == "" {
        return AgentConfig{}, fmt.Errorf("SERVER_URL is required but not set")
    }
    u, err := url.Parse(rawURL)
    if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
        return AgentConfig{}, fmt.Errorf("SERVER_URL must be a valid http/https URL, got %q", rawURL)
    }

    // Variable opcional: cae silenciosamente al default si está ausente o es inválida
    interval := 10 * time.Second
    if raw := os.Getenv("COLLECT_INTERVAL"); raw != "" {
        if d, parseErr := time.ParseDuration(raw); parseErr == nil && d >= time.Second && d <= 300*time.Second {
            interval = d
        }
        // out-of-range o error de parse: silently fall back to default
    }
    // ...
}
```

**Todo desde env vars (12-factor app).** La configuración no vive en archivos ni en código hardcodeado. Esto permite que el mismo binario corra en desarrollo (con vars en el shell) y en producción (con secrets inyectados por el orchestrador de containers) sin recompilar.

**Requeridas vs opcionales.** `SERVER_URL` es requerida: sin ella el agente no sabe adónde enviar datos, no tiene sentido arrancar. El error se propaga al `main` que hace `os.Exit(1)` — no `panic`. `COLLECT_INTERVAL` es opcional: si no está o es inválida, se usa el default de 10s. El proceso arranca igual.

**Los límites de duración importan.** La validación `d >= time.Second && d <= 300*time.Second` no es decorativa. Si alguien pusiera `COLLECT_INTERVAL=0`, `time.NewTicker(0)` entraría en pánico con `non-positive interval`. Si pusiera `COLLECT_INTERVAL=1ms`, el ticker dispararía a 1000 veces por segundo, haciendo 1000 llamadas a gopsutil y 1000 envíos HTTP por segundo — 100% de CPU y saturación del servidor. El límite inferior de 1 segundo es un mecanismo de protección, no un capricho.

---

## Flujo completo de un dato, de punta a punta

Para que el mapa mental quede completo: cuando el agente recolecta una métrica de CPU y llega al servidor, este es el camino exacto del dato:

```
time.NewTicker(interval).C
  → agent.collectLoop
      → registry.CollectAll(ctx)
          → cpu.Collect(ctx)
              → statFn(ctx, 0, false)   [gopsutil → syscall del OS]
              → model.Metric{Name: "cpu.usage_pct", Value: 42.5, Labels: {core: total}}
  → channel a.batches <- MetricBatch
  → agent.senderLoop
      → sender.Send(ctx, batch)
          → batch.Validate()            [model.Validate — misma lógica que el servidor]
          → json.Marshal(batch)
          → HTTP POST /api/v1/metrics
              → JWTMiddleware           [verifica firma HS256, rechaza alg=none]
              → HandleIngest(repo)
                  → batch.Validate()   [segunda verificación, misma función]
                  → repo.Insert(ctx, batch)
                      → pgx.Batch
                          → INSERT INTO metrics (time, host, name, value, labels)
                          → TimescaleDB
```

Cada capa tiene una responsabilidad única. Ninguna sabe cómo está implementada la siguiente.
