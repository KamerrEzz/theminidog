# Registros de Decisiones de Arquitectura — MiniObserv

---

## ADR-1: HTTP/JSON en lugar de gRPC

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

El agente envía batches de métricas al servidor en intervalos regulares (por defecto cada 10 segundos). Se evaluaron dos opciones de transporte: HTTP/JSON y gRPC con Protocol Buffers.

### Decisión

Se utiliza HTTP/JSON para toda la comunicación entre el agente y el servidor.

### Consecuencias

- **Habilita**: depuración directa con `curl` sin herramientas especiales; compatibilidad inmediata con cualquier proxy HTTP, load balancer o herramienta de observabilidad estándar; curva de aprendizaje mínima para nuevos colaboradores.
- **Restringe**: mayor sobrecarga de serialización respecto a Protocol Buffers (no relevante a la escala objetivo); no hay generación automática de clientes tipados.
- **Justificación**: el volumen de batches (una petición cada N segundos por host) no justifica la complejidad operativa de gRPC. HTTP es suficiente y reduce la fricción de desarrollo y operación.

---

## ADR-2: Monorepo plano con un único go.mod

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

El proyecto tiene dos binarios (agente y servidor) y paquetes internos compartidos (modelo, configuración). Se evaluó separar en múltiples módulos Go versus mantener un único `go.mod` en la raíz.

### Decisión

Se mantiene un único `go.mod` en la raíz con todos los paquetes bajo `internal/`.

### Consecuencias

- **Habilita**: compilación unificada con un solo comando `go build`; sin fricción de versiones entre módulos internos; gestión simplificada de dependencias externas.
- **Restringe**: no es posible publicar paquetes internos como módulos independientes sin refactorización; el directorio `internal/` garantiza que ningún código externo los importe directamente.
- **Justificación**: la escala del proyecto no requiere la complejidad de un workspace multi-módulo. La regla de visibilidad de `internal/` provee el aislamiento necesario.

---

## ADR-3: Modelo de datos estrecho con columna labels JSONB

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

Se necesitaba decidir cómo representar métricas de naturaleza heterogénea en PostgreSQL. Las alternativas eran: esquema ancho (una columna por dimensión), EAV (entity-attribute-value), o un modelo estrecho con metadatos variables en JSON.

### Decisión

La tabla `metrics` tiene cinco columnas fijas: `time`, `host`, `name`, `value`, `labels` (JSONB). Los metadatos específicos por tipo de métrica (núcleo de CPU, punto de montaje de disco, interfaz de red) se almacenan como pares clave-valor en `labels`.

### Consecuencias

- **Habilita**: esquema fijo que no requiere migraciones al añadir nuevas métricas con diferentes dimensiones; consultas simples por `host` y `name`; los índices JSONB de PostgreSQL permiten filtrar por labels si se necesita en el futuro.
- **Restringe**: no se pueden usar restricciones de base de datos para validar el contenido de `labels`; la validación ocurre en la capa de aplicación.
- **Justificación**: la variedad de dimensiones por tipo de métrica hace impráctica una columna por dimensión. JSONB en PostgreSQL es eficiente para lecturas y soporta indexación parcial si se requiere.

---

## ADR-4: pgx/v5 sin ORM — SQL directo para características de TimescaleDB

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

Se necesitaba elegir cómo acceder a PostgreSQL/TimescaleDB desde Go. Las alternativas principales eran: usar un ORM (GORM, ent), un query builder (sqlc, squirrel), o SQL directo con pgx.

### Decisión

Se usa `pgx/v5` directamente, con SQL escrito a mano en la capa de almacenamiento.

### Consecuencias

- **Habilita**: uso directo de `time_bucket()` y otras funciones de TimescaleDB sin capas de abstracción que interfieran; control total sobre el plan de consulta; acceso a `pgx.Batch` para inserciones eficientes.
- **Restringe**: más código boilerplate para mapear resultados; sin generación automática de consultas.
- **Justificación**: las consultas de TimescaleDB (`time_bucket`, hypertables) no tienen soporte natural en los ORMs populares. El conjunto de consultas es pequeño y estable, por lo que el boilerplate es manejable.

---

## ADR-5: pgx.Batch para inserción en un único round-trip

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

Cada batch del agente puede contener hasta 1000 métricas. Insertar cada una con una petición SQL independiente generaría hasta 1000 round-trips de red por tick.

### Decisión

Se usa `pgx.Batch` para agrupar todas las sentencias INSERT de un batch en un único envío al servidor de base de datos. El `BatchResults` siempre se cierra con `defer br.Close()`.

### Consecuencias

- **Habilita**: reducción drástica de la latencia de inserción; un único round-trip independientemente del tamaño del batch; menos carga en el pool de conexiones.
- **Restringe**: si un INSERT falla, el error se detecta al iterar los resultados, no durante el envío; es obligatorio iterar todos los resultados con `br.Exec()` antes de cerrar, o se producirá un deadlock.
- **Justificación**: la latencia de red dominaba el tiempo de inserción con INSERTs individuales. `defer br.Close()` es mandatorio para evitar que la conexión quede bloqueada en el pool.

---

## ADR-6: Interpolación de allowlist para time_bucket — evitar el problema de prepared statements

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

La función `time_bucket()` de TimescaleDB requiere un literal de intervalo como primer argumento (p. ej. `'1 minute'`). Al usar parámetros pgx (`$1::interval`), el driver almacena en caché el plan de consulta con el tipo del parámetro, lo que causa errores intermitentes con prepared statements en consultas subsecuentes con diferentes intervalos.

### Decisión

Los valores de `bucket` y de la función de agregación (`agg`) se interpolan directamente en el string SQL usando `fmt.Sprintf`, pero únicamente después de ser resueltos desde mapas de allowlist (`validBuckets`, `validAggs`). Ningún input del usuario llega a `fmt.Sprintf` sin pasar por esa resolución.

### Consecuencias

- **Habilita**: consultas correctas con `time_bucket()` sin errores de prepared statement; la consulta varía según el bucket pero siempre viene de un conjunto finito y controlado de valores.
- **Restringe**: los valores de `bucket` y `agg` no pueden ser arbitrarios; deben estar en la lista blanca definida en el código.
- **Justificación**: es la solución recomendada para este tipo de parámetros en TimescaleDB con pgx. La seguridad está garantizada por el allowlist, no por la parametrización.

---

## ADR-7: golang-migrate con esquema pgx5://

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

Se necesita un mecanismo para aplicar migraciones SQL al arrancar el servidor. `golang-migrate` soporta múltiples drivers de base de datos con diferentes esquemas de URL.

### Decisión

Se usa `golang-migrate` con el driver `pgx5`. La URL de migración usa el esquema `pgx5://` en lugar de `postgres://`.

### Consecuencias

- **Habilita**: uso de `pgx/v5` (la misma biblioteca que el resto del servidor) como driver de migración; consistencia en la gestión de conexiones.
- **Restringe**: la URL de `DATABASE_URL` (que usa `postgres://`) debe transformarse a `pgx5://` antes de pasarla a `golang-migrate`; esto es un detalle de inicialización que puede confundir a nuevos desarrolladores.
- **Justificación**: mezclar drivers (`database/sql` para migraciones y `pgx/v5` para el servidor) puede generar comportamientos inconsistentes en producción. Usar el mismo driver en toda la aplicación simplifica la gestión.

---

## ADR-8: TimescaleDB — extensión antes de create_hypertable

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

La migración inicial debe crear la extensión TimescaleDB, la tabla `metrics` y convertirla en hypertable. El orden de estas operaciones no es arbitrario.

### Decisión

La migración ejecuta `CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE` antes de crear la tabla y antes de llamar a `create_hypertable`.

### Consecuencias

- **Habilita**: migración idempotente que funciona en bases de datos nuevas y ya inicializadas; `IF NOT EXISTS` evita errores en re-ejecuciones.
- **Restringe**: TimescaleDB debe estar instalado en la instancia de PostgreSQL antes de ejecutar la migración (la imagen Docker `timescale/timescaledb:latest-pg16` lo incluye).
- **Justificación**: `create_hypertable` fallará si la extensión no está cargada. El orden es estricto: extensión → tabla → hypertable.

---

## ADR-9: Autenticación JWT HS256 con secreto compartido

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

Se necesita un mecanismo para que el servidor verifique que las métricas provienen del agente autorizado, y no de cualquier cliente arbitrario. Se evaluaron: sin autenticación, API key en header, JWT con clave asimétrica (RS256), y JWT con secreto compartido (HS256).

### Decisión

Se usa JWT HS256 con `AGENT_TOKEN` como secreto compartido. El agente genera tokens con `exp = now + 24h`. El servidor valida firma, algoritmo y expiración. El middleware fuerza explícitamente `jwt.WithValidMethods([]string{"HS256"})` para bloquear ataques de sustitución de algoritmo (`alg=none`, RS256 con clave pública como HMAC secret).

### Consecuencias

- **Habilita**: autenticación sin estado en el servidor; el mismo secreto en agente y servidor sin infraestructura de PKI; rotación de tokens automática cada 24 horas.
- **Restringe**: el secreto debe distribuirse a todos los agentes y al servidor; si el secreto se compromete, todos los agentes quedan afectados; no es adecuado para escenarios multi-tenant con diferentes credenciales por agente.
- **Justificación**: en el modelo de despliegue de MiniObserv (un servidor, N agentes con el mismo nivel de confianza), un secreto compartido es suficiente y operativamente simple. RS256 añadiría complejidad de gestión de claves sin beneficio real en este contexto.

---

## ADR-10: Inyección de statFn en los collectors — testabilidad sin syscalls reales

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

Los collectors (CPU, memoria, disco, red) llaman a funciones del sistema operativo a través de `gopsutil`. Probar estas llamadas en tests unitarios requeriría un sistema operativo real o mocks.

### Decisión

Cada collector expone un campo de función inyectable (`statFn`, `ioFn`, etc.) que por defecto apunta a la función real de `gopsutil`. En los tests, se reemplaza con una función que devuelve datos controlados.

### Consecuencias

- **Habilita**: tests unitarios completamente deterministas sin necesidad de Docker, contenedores ni mocks del sistema operativo; cobertura de casos borde (errores, valores extremos, contadores que se reinician).
- **Restringe**: el patrón añade un nivel de indirección; los campos de función no son exportados, por lo que solo son accesibles desde el mismo paquete.
- **Justificación**: los tests de integración que dependen del sistema operativo son frágiles y lentos. La inyección de dependencias de funciones es el patrón idiomático en Go para este tipo de mocking sin interfaces innecesarias.

---

## ADR-11: Semántica de deltas para métricas de red

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

Los contadores de red del sistema operativo son acumulativos (siempre crecen desde el arranque). Hay dos formas de exponer este dato: como valor acumulado o como delta por intervalo.

### Decisión

`net.bytes_in` y `net.bytes_out` representan el delta de bytes desde el tick anterior, no el valor acumulado. El primer tick siembra el estado inicial y no emite métricas de red.

### Consecuencias

- **Habilita**: valores directamente interpretables como "bytes transferidos en este intervalo"; sin necesidad de que el cliente calcule diferencias; los valores son consistentes independientemente del tiempo de arranque del agente.
- **Restringe**: el primer tick del agente no incluye métricas de red — esto puede sorprender a nuevos usuarios y es el caso de soporte más frecuente documentado; si el contador del SO retrocede (reinicio del kernel, overflow), el delta se clampea a cero.
- **Justificación**: los valores acumulados son difíciles de interpretar directamente en dashboards y sistemas de alerta. El delta por intervalo es la convención estándar en sistemas de monitorización de red (SNMP, Prometheus `rate()`).

---

## ADR-12: Canal con buffer para desacoplar recolección de envío

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

La recolección de métricas y el envío al servidor son operaciones con latencias muy diferentes. Si el servidor está lento o con backoff, la recolección no debe bloquearse.

### Decisión

El `Agent` usa un canal `batches chan model.MetricBatch` con buffer de tamaño 10 para desacoplar las goroutines `collectLoop` y `senderLoop`. Si el canal está lleno, el batch se descarta con un warning en el log.

### Consecuencias

- **Habilita**: la recolección continúa aunque el servidor esté temporalmente no disponible; el agente puede absorber hasta 10 ticks de backpressure antes de descartar datos.
- **Restringe**: en escenarios de indisponibilidad prolongada del servidor, se perderán batches; no hay persistencia local de batches no enviados.
- **Justificación**: la alternativa de bloquear la recolección cuando el canal está lleno podría causar que el agente acumule métricas en memoria indefinidamente. Descartar es preferible a un crecimiento ilimitado de memoria en un agente de producción.

---

## ADR-13: Backoff exponencial con jitter en el sender HTTP

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

El agente debe manejar fallos transitorios del servidor (reinicios, sobrecarga temporal, problemas de red) sin saturar al servidor con reintentos inmediatos.

### Decisión

`HTTPSender` implementa backoff exponencial comenzando en 1 s, con un techo de 60 s y un jitter del ±25 %. Los errores 4xx se consideran permanentes (no se reintenta). Los errores 5xx y los fallos de red se reintetan indefinidamente hasta que el contexto se cancela.

### Consecuencias

- **Habilita**: recuperación automática ante fallos transitorios sin intervención humana; el jitter evita el efecto "thundering herd" cuando múltiples agentes reinician simultáneamente.
- **Restringe**: un error 4xx permanente (p. ej. token inválido) hace que el batch se descarte silenciosamente después de logear el error; esto puede causar pérdida silenciosa de datos si hay un problema de configuración.
- **Justificación**: el jitter es una práctica estándar en sistemas distribuidos para evitar sincronización de reintentos. La distinción entre errores permanentes (4xx) y transitorios (5xx, red) evita reintentos infinitos de batches inválidos.

---

## ADR-14: Graceful shutdown con SIGINT y timeout configurable

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

El servidor HTTP necesita manejar señales de terminación (`SIGINT`, `SIGTERM`) de forma correcta, completando las peticiones en vuelo antes de cerrar.

### Decisión

El servidor captura `SIGINT` y `SIGTERM` mediante un canal de señales. Al recibir la señal, llama a `http.Server.Shutdown(ctx)` con un contexto que expira según `SHUTDOWN_TIMEOUT` (predeterminado 5 s, máximo 30 s). El agente también propaga la cancelación del contexto raíz cuando recibe la señal, lo que detiene el `collectLoop` y drena el canal `batches`.

### Consecuencias

- **Habilita**: despliegues sin pérdida de datos en peticiones en vuelo; tiempo de apagado predecible y acotado; comportamiento correcto con orquestadores de contenedores (Docker, Kubernetes) que envían SIGTERM antes de SIGKILL.
- **Restringe**: si hay peticiones muy largas en vuelo que superan `SHUTDOWN_TIMEOUT`, serán interrumpidas; el servidor no puede garantizar el procesamiento de todas las métricas en tránsito en el buffer del agente antes de cerrarse.
- **Justificación**: el graceful shutdown es un requisito mínimo para operar en entornos de contenedores. El timeout configurable permite ajustar el comportamiento según el tiempo de respuesta esperado del servidor de base de datos.

---

## ADR-15: Opción funcional WithNotifiers en el servidor

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

La semana 5 incorporó un sistema de notificaciones mediante webhooks. Era necesario inyectar los notificadores en el servidor sin romper los 213 tests existentes que instancian el servidor directamente. Añadir un parámetro posicional al constructor habría requerido actualizar cada test.

### Decisión

Se añadió la opción funcional `WithNotifiers(notifiers []Notifier) ServerOption` que se pasa al constructor del servidor solo cuando está configurado. Los tests existentes no necesitan cambios.

### Consecuencias

- **Habilita**: extensión del servidor sin modificar la firma del constructor; los tests heredados siguen compilando sin cambios; añadir nuevos tipos de notificador (email, PagerDuty) requiere solo implementar la interfaz.
- **Restringe**: el comportamiento de notificación es silencioso si `WithNotifiers` no se llama (correcto para tests unitarios, incorrecto si se olvida en producción).
- **Justificación**: el patrón de opciones funcionales es la forma idiomática en Go para extender constructores con parámetros opcionales sin romper la compatibilidad.

---

## ADR-16: HostTracker en memoria, sin persistencia en base de datos

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

El sistema necesita saber si un host está activo, inactivo o caído. Se evaluó persistir este estado en base de datos frente a mantenerlo solo en memoria.

### Decisión

`HostTracker` es una estructura en memoria con un mapa `map[string]time.Time` protegido por un mutex. El heartbeat del host se actualiza en cada ingesta de métricas. No se escribe nada en la base de datos.

### Consecuencias

- **Habilita**: latencia cero para actualizar el estado del host; sin carga adicional en TimescaleDB; implementación simple y directamente testeable.
- **Restringe**: el estado se pierde al reiniciar el servidor; después de un reinicio, todos los hosts aparecen como `unknown` hasta que envíen su primer batch. En producción, esto puede generar falsas alertas de `host.down` al arrancar.
- **Justificación**: la vivacidad de un host es información efímera por naturaleza. El coste operativo de persistirla no justifica el beneficio, especialmente en v1 con un único servidor.

---

## ADR-17: Heartbeat del host en el handler de ingesta, no en la capa de almacenamiento

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

Había que decidir en qué punto del flujo de ingesta se actualiza el timestamp de última actividad del host: en el handler HTTP o en el repositorio de almacenamiento.

### Decisión

`HandleIngest` llama a `hostTracker.Heartbeat(host)` inmediatamente después de validar el batch y antes de llamar a `storage.Insert`. El repositorio no tiene conocimiento de `HostTracker`.

### Consecuencias

- **Habilita**: separación de responsabilidades clara; `storage` permanece enfocado en persistencia; el heartbeat se registra incluso si la inserción en base de datos falla posteriormente.
- **Restringe**: si la validación falla antes del heartbeat, el host no se actualiza (correcto: un batch inválido no debe considerarse actividad válida).
- **Justificación**: el seguimiento de vivacidad del host es responsabilidad de la capa de API, no de la capa de almacenamiento.

---

## ADR-18: Intervalos de retención fijos en v1

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

TimescaleDB permite configurar políticas de retención de datos automáticas (`add_retention_policy`). Se evaluó exponer estos intervalos como variables de entorno versus fijarlos en el código para v1.

### Decisión

Los intervalos de retención están definidos como constantes en el código para v1. No se exponen como variables de entorno ni como parámetros configurables.

### Consecuencias

- **Habilita**: despliegue simple sin configuración adicional; comportamiento predecible.
- **Restringe**: cambiar los intervalos requiere recompilar; los operadores no pueden ajustar la retención sin modificar el código.
- **Justificación**: exponer la retención como configuración añade superficie de API y complejidad de validación. Para v1, los valores por defecto son suficientes; la configurabilidad puede añadirse en v2 cuando se conozcan los requisitos reales.

---

## ADR-19: La migración de conversión a hypertable de logs es irreversible en el down

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

La migración 003 convierte la tabla `logs` en un hypertable de TimescaleDB mediante `create_hypertable`. TimescaleDB no proporciona un mecanismo para deshacer esta conversión.

### Decisión

El archivo `.down.sql` de la migración 003 contiene únicamente un comentario explicando que la operación es irreversible. No intenta recrear la tabla como tabla regular.

### Consecuencias

- **Habilita**: honestidad operativa sobre las limitaciones de TimescaleDB; evita un down migration silenciosamente roto.
- **Restringe**: hacer rollback de la migración 003 requiere restaurar desde un backup o recrear la tabla manualmente.
- **Justificación**: un down migration que falla silenciosamente es peor que uno que documenta explícitamente su irreversibilidad. Los operadores deben saberlo antes de aplicar la migración en producción.

---

## ADR-20: host.down sintético mediante notificadores, no mediante ActiveAlerts()

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

Los eventos `host.down` necesitan disparar notificaciones webhook. Se evaluó reutilizar el sistema de `ActiveAlerts()` (que evalúa reglas de umbral) para modelar el estado `down` como una alerta especial.

### Decisión

`HostTracker` dispara directamente los notificadores cuando detecta que un host ha superado `HOST_DOWN_AFTER`. El evento `host.down` no pasa por el sistema de alertas basado en umbrales.

### Consecuencias

- **Habilita**: separación clara entre alertas de métricas (umbral) y alertas de vivacidad (silencio del host); cada sistema puede evolucionar independientemente.
- **Restringe**: los eventos `host.down` no aparecen en `GET /api/v1/alerts` si ese endpoint solo expone alertas activas de umbrales.
- **Justificación**: el estado `down` de un host es fundamentalmente diferente a cruzar un umbral de métrica: no tiene un valor numérico, no se "resuelve" automáticamente y su origen es la ausencia de datos, no su presencia.

---

## ADR-21: El panel lateral itera HostStatuses con fallback a Hosts

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

El panel de control (dashboard) necesita mostrar la lista de hosts con su estado. Los datos provienen de dos fuentes posibles: `HostTracker.HostStatuses()` (estado en tiempo real) y la lista de hosts únicos conocidos en la base de datos.

### Decisión

El panel lateral itera primero sobre `HostStatuses()` del `HostTracker`. Si el tracker no tiene entradas (por ejemplo, tras un reinicio del servidor antes de que los agentes reporten), hace fallback a la lista de hosts distintos de la base de datos.

### Consecuencias

- **Habilita**: panel funcional incluso tras un reinicio del servidor; coherencia con el estado en tiempo real cuando está disponible.
- **Restringe**: durante el período de fallback, todos los hosts aparecen sin estado (`unknown`) hasta que los agentes reporten su primer batch.
- **Justificación**: la experiencia de usuario es mejor con una lista de hosts (aunque sin estado) que con un panel vacío.

---

## ADR-22: Clave primaria compuesta (id, time) requerida antes de create_hypertable en logs

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

TimescaleDB requiere que la columna de particionamiento temporal (`time`) forme parte de la clave primaria o de todos los índices únicos de la tabla antes de convertirla en hypertable. La tabla `logs` tenía originalmente solo `id` como clave primaria.

### Decisión

La migración 003 altera la tabla `logs` para usar una clave primaria compuesta `(id, time)` antes de llamar a `create_hypertable('logs', 'time')`.

### Consecuencias

- **Habilita**: conversión correcta a hypertable sin errores de restricción de unicidad.
- **Restringe**: las referencias de clave foránea a `logs.id` deben incluir también `time` si se añaden en el futuro.
- **Justificación**: es un requisito técnico de TimescaleDB, no una decisión de diseño. No hacerlo causa un error en tiempo de migración.

---

## ADR-23: context.WithoutCancel para goroutines de notificación

**Estado**: Aceptado
**Fecha**: 2026-06-05

### Contexto

Las notificaciones webhook se envían en goroutines independientes. Si se usan directamente el contexto de la petición HTTP entrante, la goroutine de notificación se cancela en cuanto el handler de ingesta responde al cliente, antes de que el webhook pueda completar su petición HTTP de salida.

### Decisión

Cada goroutine de notificación recibe `context.WithoutCancel(ctx)` en lugar del contexto original. Esto desvincula el ciclo de vida de la notificación del de la petición HTTP que la originó.

### Consecuencias

- **Habilita**: las notificaciones se completan aunque la petición HTTP original ya haya terminado; el timeout de 5 segundos del notificador es el único límite.
- **Restringe**: si el servidor recibe SIGTERM durante el envío de una notificación, la goroutine no se cancelará por el contexto de shutdown; se cancelará solo cuando expire su propio timeout.
- **Justificación**: `context.WithoutCancel` es la solución correcta en Go para operaciones de "fire-and-forget" que deben completarse con independencia del contexto del llamante.
