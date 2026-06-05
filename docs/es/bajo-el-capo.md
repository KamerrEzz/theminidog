# Bajo el capó — Cómo funciona el código realmente

Esta página recorre los problemas técnicos más interesantes de MiniObserv y muestra exactamente cómo los resuelve el código. Cada fragmento proviene de los archivos fuente reales — nada está simplificado para ilustrar.

El objetivo no es explicar cada línea. Es mostrarte las partes difíciles y el razonamiento detrás de ellas, para que puedas llevarte esas ideas a tus propios proyectos.

---

## 1. Cómo se mide realmente el uso de CPU

Aquí hay algo que la mayoría de los desarrolladores no sabe: **el porcentaje de CPU no existe en ningún lugar del sistema.** No hay ningún archivo que puedas abrir y leer `cpu_uso = 73.2%`.

Lo que el kernel de Linux expone en `/proc/stat` es un conjunto de **contadores acumulativos** — tiempo total transcurrido en cada modo de CPU desde que arrancó la máquina. Se miden en "jiffies" (ticks del kernel):

```
cpu  123456 0 67890 9876543 1234 0 567 0 0 0
     ^user  ^nice ^sys    ^idle  ^iowait ...
```

Para obtener un porcentaje con sentido hay que:

1. Leer los contadores en el tiempo T1
2. Esperar un momento
3. Leer los contadores de nuevo en T2
4. Restar: `delta_ocupado = (user+sys+nice)_T2 - (user+sys+nice)_T1`
5. Calcular la proporción: `ocupado / (ocupado + idle) * 100`

Por eso el agente recopila cada 10 segundos — necesitas dos puntos de datos para calcular una tasa. No hay atajo.

MiniObserv usa [gopsutil](https://github.com/shirou/gopsutil), una librería que maneja todo el análisis de `/proc/stat`. El collector simplemente la llama:

```go
// Collect implements Collector. It returns one aggregate cpu.usage_pct metric
// with label core=total, plus one per logical core with label core=<index>.
func (c *CPUCollector) Collect(ctx context.Context) ([]model.Metric, error) {
    now := time.Now().UTC()

    // Aggregate (percpu=false)
    totals, err := c.statFn(ctx, 0, false)
    if err != nil {
        return nil, fmt.Errorf("cpu aggregate: %w", err)
    }

    // Per-core (percpu=true)
    perCore, err := c.statFn(ctx, 0, true)
    if err != nil {
        return nil, fmt.Errorf("cpu per-core: %w", err)
    }
    // ...construir slice de métricas...
}
```

Observa que `statFn` es un campo, no una llamada directa a `gopsutil`. Ese detalle importa mucho — volvemos a él en la sección 3.

**Conclusión:** Cuando necesites una tasa — CPU%, ancho de banda, peticiones/seg — necesitas dos muestras y una ventana de tiempo. Las tasas son siempre derivadas, nunca valores que puedas observar directamente.

---

## 2. Métricas de red: misma idea, diferente trampa

`/proc/net/dev` expone los contadores de interfaces de red de la misma forma — bytes totales acumulados recibidos y enviados desde el arranque, no bytes por segundo:

```
Inter-|   Receive                                           |  Transmit
 face |bytes    packets errs drop ...                       | bytes    ...
    lo: 123456       89    0    0 ...                         123456 ...
  eth0: 987654321  7654    0    0 ...                         456789 ...
```

Entonces el NetworkCollector aplica el mismo patrón de delta. Pero aquí hay una trampa adicional que sorprende a mucha gente.

**En la primera llamada de recolección no hay muestra anterior de la que restar.** El código lo maneja explícitamente:

```go
// Collect implements Collector. On the first call it seeds prev and returns
// nil (no metrics). On subsequent calls it computes byte deltas per interface.
func (c *NetworkCollector) Collect(ctx context.Context) ([]model.Metric, error) {
    // ...construir snapshot actual...

    // First call: seed prev and return empty slice.
    if c.prev == nil {
        c.prev = curr
        c.prevAt = now
        return nil, nil  // <-- no se emiten métricas
    }

    // Compute deltas.
    for name, cs := range curr {
        ps, ok := c.prev[name]
        if !ok {
            continue // nueva interfaz apareció desde el último tick; saltar
        }
        bytesIn := int64(cs.BytesRecv) - int64(ps.BytesRecv)
        bytesOut := int64(cs.BytesSent) - int64(ps.BytesSent)
        if bytesIn < 0 { bytesIn = 0 }   // protección contra reinicio del contador
        if bytesOut < 0 { bytesOut = 0 }
        // ...agregar métricas...
    }

    c.prev = curr
    c.prevAt = now
    return metrics, nil
}
```

Mira las líneas del delta. Después de calcular la diferencia, el código fija en cero los valores negativos. Esto maneja el **reinicio de contadores** — si el kernel reinicia un contador (reboot, recarga del driver), la resta produce un número negativo enorme. Fijar en cero significa perder un punto de datos en lugar de emitir un pico absurdo. Es el tradeoff correcto.

Esto también explica por qué las métricas de red no muestran nada en el primer tick de recolección después de que el agente arranca. Ese comportamiento es intencional, no un error.

**Conclusión:** Los contadores acumulativos están en todas partes — bytes de red, I/O de disco, conteos de peticiones HTTP, totales de errores. Cuando trabajes con ellos, siempre almacena el valor anterior y siempre protégete contra deltas negativos.

---

## 3. Hacer testeable a los collectors — el patrón statFn

Aquí hay un problema: ¿cómo escribes un test unitario para un collector de CPU?

Si tu collector llama directamente a `gopsutil.CPUPercent()`, tu test leerá los valores reales de CPU de cualquier máquina donde se ejecute. Esos valores son diferentes en cada máquina, cambian cada segundo, y no te dicen nada sobre si tu código es correcto. Estarías testeando el sistema operativo, no tu lógica.

La solución es **inyección de dependencias al nivel más simple posible** — un campo de función:

```go
// CPUCollector collects CPU usage metrics using an injectable statFn for
// testability. In production, statFn is cpu.PercentWithContext from gopsutil.
type CPUCollector struct {
    host   string
    statFn func(ctx context.Context, interval time.Duration, percpu bool) ([]float64, error)
}

// NewCPUCollector returns a CPUCollector wired to gopsutil for real OS data.
func NewCPUCollector(host string) *CPUCollector {
    return &CPUCollector{
        host:   host,
        statFn: gopsutilcpu.PercentWithContext,  // implementación real
    }
}
```

En producción, `statFn` apunta a la función real de gopsutil. En los tests, la reemplazas con lo que quieras:

```go
// makeCPUStatFn returns a statFn stub that delegates based on the percpu flag.
func makeCPUStatFn(
    totalResult []float64, totalErr error,
    perCoreResult []float64, perCoreErr error,
) func(ctx context.Context, interval time.Duration, percpu bool) ([]float64, error) {
    return func(_ context.Context, _ time.Duration, percpu bool) ([]float64, error) {
        if percpu {
            return perCoreResult, perCoreErr
        }
        return totalResult, totalErr
    }
}
```

Y entonces un test se vuelve completamente determinista:

```go
func TestCPUCollector_Collect_ReturnsAggregateAndPerCore(t *testing.T) {
    // Stub: aggregate=40.0, per-core=[30.0, 50.0]
    statFn := makeCPUStatFn(
        []float64{40.0}, nil,
        []float64{30.0, 50.0}, nil,
    )
    c := &CPUCollector{host: "test-host", statFn: statFn}

    metrics, err := c.Collect(context.Background())
    // ...verificar exactamente 3 métricas, valores correctos, labels correctos...
}
```

Sin llamadas reales al OS. Sin comportamiento errático. El test corre en microsegundos y produce el mismo resultado en cada máquina, para siempre.

No hay ningún framework aquí — ninguna librería de mocks, ningún contenedor de DI, nada. Solo un campo de función. Esto es inyección de dependencias en su forma más esencial.

**Conclusión:** Cada vez que tu código hable con el mundo exterior — OS, red, base de datos, reloj, sistema de archivos — inyéctalo como función o interfaz. Tus tests se vuelven instantáneos, deterministas y confiables. Estás testeando TU lógica, no el mundo alrededor de ella.

---

## 4. Escribir un test que falla primero

Mira este test del evaluador de alertas:

```go
func TestEvaluator_firesOnGT(t *testing.T) {
    points := []storage.QueryPoint{
        {Time: time.Now().UTC(), Value: 95.0},
        {Time: time.Now().UTC().Add(-time.Minute), Value: 95.0},
    }
    q := &fakeQuerier{
        queryFn: func(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
            return points, nil
        },
    }
    rule := alerting.Rule{
        Host:      "web-01",
        Name:      "cpu.usage_pct",
        Op:        alerting.OpGT,
        Threshold: 90.0,
        For:       5 * time.Minute,
    }
    e := alerting.NewEvaluator([]alerting.Rule{rule}, q, nil)
    e.EvaluateForTest(context.Background())

    alerts := e.ActiveAlerts()
    if len(alerts) != 1 {
        t.Fatalf("expected 1 alert, got %d", len(alerts))
    }
    if alerts[0].State != alerting.StateFiring {
        t.Fatalf("expected StateFiring, got %v", alerts[0].State)
    }
}
```

Lee este test de arriba a abajo. Nota que cuenta una historia completa antes de que exista una sola línea de implementación:

1. **Arrange**: configura una fuente de datos falsa que devuelve valores de CPU del 95% — por encima del umbral del 90%
2. **Act**: ejecuta un ciclo de evaluación
3. **Assert**: el evaluador debe reportar exactamente una alerta en estado `FIRING`

Cuando se escribió este test, el `Evaluator` todavía no existía. El test fallaba inmediatamente — **RED**. Ese fallo es la especificación. Dice exactamente qué debe hacer el código.

Luego se escribió la implementación para que pasara — **GREEN**.

Ahora mira el test complementario que verifica la dirección contraria:

```go
func TestEvaluator_resolvesGT(t *testing.T) {
    callCount := 0
    q := &fakeQuerier{
        queryFn: func(_ context.Context, _ storage.QueryParams) ([]storage.QueryPoint, error) {
            callCount++
            if callCount == 1 {
                return []storage.QueryPoint{{Value: 95.0}}, nil  // dispara
            }
            return []storage.QueryPoint{{Value: 85.0}}, nil  // resuelve
        },
    }
    // ...
    e.EvaluateForTest(context.Background())
    // verificar StateFiring...
    e.EvaluateForTest(context.Background())
    // verificar StateOK...
}
```

Este test especifica la transición de estado completa: `StateFiring` → `StateOK`. La implementación tuvo que manejar este caso o el test fallaría. El test es el documento de requisitos — y a diferencia de un documento de requisitos real, este corre.

**Conclusión:** Los tests no son para verificar si el código funciona. Son para especificar qué DEBE hacer el código antes de escribirlo. Un test escrito después de la implementación es una comprobación de cordura. Un test escrito antes es una herramienta de diseño.

---

## 5. Cómo funciona el JWT — sin tabla de usuarios, sin sesiones

La autenticación tradicional requiere mucha infraestructura: tabla de usuarios, almacenamiento de sesiones, gestión de cookies, rotación de refresh tokens. Para un agente de monitoreo que envía métricas a un servidor de confianza, todo eso es excesivo.

MiniObserv usa JWT con HS256 — un modelo mucho más simple basado en un **secreto compartido**.

Al arrancar el agente, se crea un token firmado con `AGENT_TOKEN`:

```go
// mintAgentToken creates a short-lived HS256 JWT signed with the given secret.
func mintAgentToken(secret string) (string, error) {
    claims := jwt.RegisteredClaims{
        Issuer:    "miniobserv-agent",
        ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
        IssuedAt:  jwt.NewNumericDate(time.Now()),
    }
    return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}
```

Ese token se adjunta a cada petición HTTP como header `Bearer`. En el servidor, el middleware lo verifica:

```go
// JWTMiddleware validates Bearer JWT tokens using HS256.
// It enforces jwt.WithValidMethods([]string{"HS256"}) to block alg=none attacks.
func JWTMiddleware(secret []byte) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // ...extraer token del header Authorization...
            _, err := jwt.ParseWithClaims(
                tokenStr,
                &jwt.RegisteredClaims{},
                func(t *jwt.Token) (any, error) { return secret, nil },
                jwt.WithValidMethods([]string{"HS256"}), // OBLIGATORIO
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

La línea clave es `jwt.WithValidMethods([]string{"HS256"})`. Esto no es decoración — bloquea el ataque `alg=none`, donde un token malicioso afirma que no se requiere firma. Sin esta protección, un atacante podría crear un token que se verifica contra cualquier clave. La librería te exige declarar explícitamente en qué algoritmos confías.

El flujo completo es: agente y servidor comparten el mismo secreto → el agente firma con él → el servidor verifica la firma → si coincide, la petición es auténtica. Sin consulta a base de datos. Sin estado de sesión. Solo matemática.

El tradeoff: si `AGENT_TOKEN` se filtra, cualquiera puede enviar métricas a tu servidor. Para un sistema de monitoreo interno en una red privada, este es el tradeoff correcto — gana la simplicidad.

**Conclusión:** Para autenticación servicio a servicio (no autenticación de usuarios), un secreto HMAC compartido es a menudo más simple y suficiente. Sin tabla de usuarios, sin sesiones, sin cookies. Solo dos lados que conocen el mismo secreto.

---

## 6. TimescaleDB — ¿por qué no PostgreSQL a secas?

Podrías almacenar métricas en una tabla PostgreSQL normal. Funcionaría. Pero consultar "dame el promedio de CPU en buckets de 5 minutos para la última hora" requeriría algo así:

```sql
SELECT
    date_trunc('minute', time) - (EXTRACT(MINUTE FROM time)::int % 5) * INTERVAL '1 minute' AS bucket,
    AVG(value)
FROM metrics
WHERE host = 'web-01' AND name = 'cpu.usage_pct'
  AND time >= NOW() - INTERVAL '1 hour'
GROUP BY bucket
ORDER BY bucket DESC;
```

Es verboso, y más importante, es lento en tablas grandes porque PostgreSQL no sabe que estos son datos de series temporales — no puede particionarlos ni indexarlos eficientemente por rangos de tiempo.

TimescaleDB agrega una cosa: `time_bucket()`. La migración que crea la tabla de métricas hace dos cosas críticas:

```sql
CREATE TABLE IF NOT EXISTS metrics (
    time   TIMESTAMPTZ      NOT NULL,
    host   TEXT             NOT NULL,
    name   TEXT             NOT NULL,
    value  DOUBLE PRECISION NOT NULL,
    labels JSONB
);

SELECT create_hypertable('metrics', 'time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);
```

`create_hypertable` convierte la tabla en una **hypertable** — internamente, TimescaleDB la divide en chunks (fragmentos), uno por día en este caso. Cada chunk es una tabla física separada. Las consultas con rango de tiempo solo tocan los chunks relevantes, no todo el dataset. Los chunks viejos pueden comprimirse o eliminarse automáticamente.

La consulta entonces se vuelve limpia y rápida:

```go
q := fmt.Sprintf(`
    SELECT time_bucket('%s', time) AS bucket,
           %s(value) AS value
    FROM metrics
    WHERE host = $1
      AND name = $2
      AND time >= $3
      AND time <= $4
    GROUP BY bucket
    ORDER BY bucket DESC`,
    bucketLiteral, aggFn,
)
```

Nota: `bucketLiteral` y `aggFn` nunca son entrada directa del usuario — se resuelven desde una lista de valores permitidos (los mapas `validBuckets` y `validAggs`) para prevenir inyección SQL. El bucket y la función de agregación se buscan en valores seguros antes de interpolarlos en la consulta.

La migración 003 agrega políticas de compresión y retención. Mira esta secuencia:

```sql
-- Step 2: Convert logs to a hypertable (logs was a plain BIGSERIAL table)
SELECT create_hypertable('logs', 'time', migrate_data => true, if_not_exists => true);

-- Step 6: Add retention policies (drop chunks older than threshold)
SELECT add_retention_policy('metrics', INTERVAL '30 days');
SELECT add_retention_policy('logs', INTERVAL '14 days');
```

La tabla `logs` tuvo que convertirse a hypertable PRIMERO, LUEGO se agregó la política de retención. No puedes agregar una política de retención a una tabla regular — solo funciona en hypertables. Ese orden en la migración no es arbitrario.

**Conclusión:** Cuando tus datos son series temporales — métricas, eventos, logs, auditorías — usa una base de datos o extensión de series temporales. Los patrones de consulta son completamente distintos a los datos relacionales, y la diferencia de rendimiento a escala es dramática.

---

## 7. La máquina de estados de alertas — por qué PENDING antes de FIRING

Mira los estados de alerta en el evaluador:

```go
const (
    StateFiring AlertState = "firing"
    StateOK     AlertState = "ok"
)
```

Solo hay dos estados persistidos. Pero el campo `for` en una regla crea un tercer estado implícito: **PENDING** — la condición es verdadera, pero aún no por suficiente tiempo.

¿Por qué importa esto? Considera una regla: "disparar si CPU > 90% durante 5 minutos". Sin la duración `for`, un pico único al 95% durante un segundo dispararía una notificación. Tu teléfono suena a las 3am porque un cron job se ejecutó. Eso es fatiga de alertas — el peor tipo de ruido en monitoreo de producción.

La duración `for` resuelve esto. El evaluador consulta el promedio sobre la ventana completa del `for`:

```go
points, err := e.repo.Query(ctx, storage.QueryParams{
    Host:   host,
    Name:   rule.Name,
    From:   now.Add(-rule.For),  // mirar atrás toda la ventana
    To:     now,
    Bucket: "1m",
    Agg:    "avg",
})
// ...
// Average of bucket averages.
sum := 0.0
for _, p := range points {
    sum += p.Value
}
mean := sum / float64(len(points))

firing := (rule.Op == OpGT && mean > rule.Threshold) ||
          (rule.Op == OpLT && mean < rule.Threshold)
```

La condición solo se vuelve `FIRING` cuando el promedio sobre toda la ventana `for` cruza el umbral. Un pico de un segundo no mueve suficientemente el promedio para disparar. Un problema sostenido sí.

Luego la transición de estado:

```go
e.mu.Lock()
prev, existed := e.state[key]
e.state[key] = Alert{...State: newState...}
e.mu.Unlock()

if !existed || prev.State != newState {
    if newState == StateFiring {
        e.notifyAll(ctx, "firing", ...)
    } else {
        // Only notify resolved if we transitioned FROM firing
        if existed && prev.State == StateFiring {
            e.notifyAll(ctx, "resolved", ...)
        }
    }
}
```

Las notificaciones solo se disparan en **transiciones**, no en cada tick. Y la notificación de "resolved" solo se dispara si el estado anterior era `FIRING` — si la alerta nunca se disparó, no hay nada que resolver. Esto previene el spam de "resolved" para alertas que nunca se activaron.

**Conclusión:** Las máquinas de estados están en todas partes en la programación de sistemas. Cuando tienes "si esta condición persiste durante X tiempo, hacer Y", necesitas rastrear estado. Sin eso obtienes falsos positivos (demasiado ruidoso) o alertas perdidas (demasiado silencioso).

---

## 8. Seguimiento de logs — ¿cómo sabe cuándo cambia un archivo?

El enfoque ingenuo para vigilar un archivo de log es un bucle de polling:

```go
for {
    content := readFile("app.log")
    // comparar con contenido previo
    time.Sleep(1 * time.Second)
}
```

Esto es lento, desperdicia CPU, pierde cambios que ocurren entre polls, y no escala a muchos archivos. Hay una forma mejor.

El `Tailer` usa [fsnotify](https://github.com/fsnotify/fsnotify), que envuelve las APIs de eventos del sistema de archivos a nivel del OS — `inotify` en Linux, `kqueue` en macOS, `ReadDirectoryChangesW` en Windows. El propio OS te avisa cuando cambia un archivo. Sin polling:

```go
// Run blocks until ctx is cancelled, processing fsnotify events.
func (t *Tailer) Run(ctx context.Context) {
    defer t.closeAll()
    for {
        select {
        case <-ctx.Done():
            return
        case event, ok := <-t.watcher.Events:
            if !ok {
                return
            }
            t.handleEvent(ctx, event)
        case err, ok := <-t.watcher.Errors:
            // ...
        }
    }
}
```

El tailer espera en un bucle `select` los eventos del OS. Cuando llega un evento `Write`, lee solo los bytes nuevos desde el último offset — nunca relee el archivo completo:

```go
func (t *Tailer) handleEvent(ctx context.Context, event fsnotify.Event) {
    path := event.Name
    switch {
    case event.Has(fsnotify.Write):
        entries := t.readNewLines(path)
        t.sendChunked(ctx, entries)
    case event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove):
        t.closePath(path)
        t.offsets[path] = 0
    case event.Has(fsnotify.Create):
        t.closePath(path)
        if err := t.openAndSeekEOF(path); err != nil {
            // ...
            return
        }
        t.offsets[path] = 0  // leer desde el inicio del nuevo archivo
        // ...
    }
}
```

Aquí está la parte difícil: **la rotación de logs**. Cuando un logger rota, renombra `app.log` a `app.log.1` y crea un nuevo `app.log` vacío. Tu file descriptor todavía apunta al archivo renombrado — estás leyendo de `app.log.1`, no del log activo.

El código maneja esto a través del stream de eventos. Un evento `Rename` o `Remove` cierra el handle viejo y resetea el offset a 0. Un evento `Create` (el nuevo `app.log` vacío apareciendo) reabre el archivo desde el inicio. El `offsets[path] = 0` en `Create` asegura que leas desde el inicio del nuevo archivo en lugar de intentar hacer seek a la posición EOF anterior.

El `openAndSeekEOF` inicial al arrancar hace lo contrario — registra el tamaño actual del archivo como offset de inicio, para que el contenido preexistente nunca se envíe. Solo se hace tail de las líneas nuevas escritas después de que el agente arrancó.

**Conclusión:** Para vigilar archivos, usa eventos del OS — no polling. Y siempre maneja la rotación para archivos de log. El archivo que abriste al inicio no siempre es el archivo que crees que estás leyendo.

---

## Qué llevarte de todo esto

Ninguno de estos patrones es exótico ni específico de MiniObserv. Aparecen en cualquier codebase serio en Go:

- **Cálculos de delta** para cualquier métrica de tasa
- **Funciones inyectables** para todo lo que toque el OS o la red
- **Tests escritos primero** como especificaciones ejecutables
- **JWT con secreto compartido** para autenticación servicio a servicio
- **Hypertables** para datos de series temporales
- **Máquinas de estados** para alertas basadas en condiciones
- **Eventos del OS** para seguimiento de logs

Si te llevas una sola cosa de esta página, que sea esta: la decisión más importante en cualquier sistema es qué NO construir. Cada sección de arriba es un caso donde se eligió deliberadamente un enfoque más simple — y los tradeoffs son explícitos. Ese juicio es lo que separa a un ingeniero senior de alguien que simplemente escribe código.
