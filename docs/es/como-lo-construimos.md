# Cómo lo construimos — Un viaje de 5 semanas

## La pregunta que lo inició todo

"¿Y si construyo un mini Datadog?"

No para reemplazarlo. No para competir con él. Sino para entenderlo: ¿cómo funciona realmente un pipeline de métricas de principio a fin? ¿Qué hace falta para recolectar CPU y memoria de una máquina, enviarlo por la red, persistirlo en una base de datos de series temporales y mostrarlo en un dashboard en tiempo real?

La respuesta fue construirlo desde cero. Sin código existente del cual copiar, sin atajos, sin generadores de scaffolding. El objetivo era aprender, no entregar rápido. Cada semana, una capa del sistema pasó de "no sé cómo funciona esto" a "lo construí, sé exactamente cómo funciona".

Ese proyecto es MiniObserv.

---

## Spec-Driven Development (SDD)

Antes de escribir una sola línea de código, cada semana comenzaba con una fase de planificación. La secuencia siempre era la misma:

```
explore → propose → spec ──┐
                            ├──► tasks → apply → verify → archive
                       design ──┘
```

**Explore** significa leer el espacio del problema sin comprometerse: ¿cuáles son las restricciones? ¿qué han hecho otros? ¿cuáles son las compensaciones?

**Propose** significa elegir una dirección y escribirla como una propuesta breve: esto es lo que vamos a construir, esto es el por qué, estas son las alternativas que rechazamos.

**Spec** convierte la propuesta en un contrato: ¿qué entradas acepta el sistema, qué salidas produce, cuáles son las invariantes?

**Design** mapea la especificación en estructuras concretas: ¿qué paquetes, qué interfaces, qué tipos, qué esquema SQL?

**Tasks** es el desglose: una lista numerada de unidades de trabajo pequeñas y verificables, en orden de dependencias.

**Apply** es la implementación: una tarea a la vez, primero los tests.

**Verify** comprueba la implementación contra la especificación: ¿existe y funciona correctamente todo lo que se prometió?

**Archive** cierra el cambio: la especificación, el diseño, el registro de aplicación y el informe de verificación quedan en el registro.

¿Por qué importa esto? Cuando se escribe la especificación primero, los errores de diseño se detectan antes de convertirse en errores de código. Las especificaciones son baratas de cambiar. Una oración en una especificación no cuesta nada eliminarla. Una firma de función que se ha propagado por diez archivos cuesta horas renombrarla. La disciplina de especificar primero obliga a pensar en el sistema antes de comprometerse con una forma.

---

## TDD estricto — Rojo, Verde, Refactorizar

Cada tarea que tocó código Go siguió el mismo ritmo:

1. **ROJO** — Escribir un test que falla y que describe exactamente lo que el código debe hacer. Ejecutarlo. Verlo fallar. El mensaje de falla debe ser específico.
2. **VERDE** — Escribir el código mínimo que hace pasar el test. No el código más limpio. No el más general. Solo el suficiente para poner el test en verde.
3. **Refactorizar** — Limpiar. Eliminar duplicación. Mejorar nombres. El conjunto de tests mantiene la seguridad: si algo se rompe durante la limpieza, un test falla de inmediato.

La regla era estricta: nunca escribir código de implementación sin un test que falle primero. Si se escribe una función y luego se escriben tests para ella, ya se perdió el beneficio. El test no es una formalidad — es la especificación en forma ejecutable.

El resultado: 213 tests en toda la base de código, cero tests inestables, y la capacidad de refactorizar la capa de almacenamiento, el evaluador o el pipeline del collector con la confianza de que el conjunto de tests detectará cualquier regresión.

---

## Semana a semana

### Semana 1 — Agent

El agente recolecta métricas del sistema (CPU, memoria, disco, red) usando gopsutil, las acumula en batches y las envía al servidor vía HTTP/JSON cada 10 segundos. Las decisiones clave aquí fueron la semántica de deltas para contadores de red, la inyección de funciones para testabilidad sin syscalls del SO, y un canal con buffer para desacoplar la recolección del envío.

### Semana 2 — Server

El servidor recibe métricas por HTTP, autentica agentes con JWT HS256 y persiste datos en TimescaleDB. La API de consulta permite a los clientes solicitar agregaciones por ventanas de tiempo sobre cualquier rango. Las decisiones críticas aquí fueron el modelo de tabla estrecha, `pgx.Batch` para inserciones en masa, y la interpolación por lista blanca (allowlist) para parámetros de `time_bucket`, para evitar colisiones en la caché de planes de pgx.

### Semana 3 — Logs

El agente incorporó capacidad de seguimiento de logs: lee archivos de log línea por línea usando `bufio.Scanner`, envía las entradas al servidor vía la API de ingesta, y el servidor las almacena en un hypertable de TimescaleDB. La API de búsqueda soporta filtrado por texto completo con paginación. La migración 003 introdujo el requisito de clave primaria compuesta que TimescaleDB impone sobre las hypertables.

### Semana 4 — Dashboard y Alertas

Se añadió un dashboard web en tiempo real usando `html/template` y `embed` — sin framework JavaScript, sin paso de compilación frontend. Las reglas de alerta por umbral permiten a los operadores definir condiciones como "cpu.percent > 90 durante 5 evaluaciones consecutivas". El evaluador corre en un ticker, consulta métricas recientes y dispara alertas cuando se cruzan los umbrales. Cero nuevas dependencias externas.

### Semana 5 — Notificaciones, Salud de Hosts y Retención

La semana final añadió notificaciones webhook para alertas de umbral y eventos de host caído, un `HostTracker` para monitorización de vivacidad en tiempo real, y políticas de retención de TimescaleDB para limitar el crecimiento del almacenamiento. `context.WithoutCancel` garantizó que las goroutines de notificación sobrevivan al ciclo de vida del handler de ingesta. La opción funcional `WithNotifiers` mantuvo los 213 tests existentes intactos.

---

## Architecture Decision Records

Cada decisión no obvia tiene un ADR. No "qué hicimos" — el código ya muestra eso. Los ADRs documentan el *por qué*, y específicamente cuáles fueron las alternativas y por qué se rechazaron.

23 ADRs en 5 semanas.

Algunos ejemplos de por qué esto importa:

- **ADR-6** explica por qué los parámetros de `time_bucket` se interpolan como strings desde una lista blanca en lugar de parametrizarse con `$1::interval`. Sin el ADR, un desarrollador futuro vería el `fmt.Sprintf` y concluiría razonablemente que es una vulnerabilidad de seguridad.
- **ADR-22** explica por qué la tabla `logs` tiene una clave primaria compuesta `(id, time)` en lugar de solo `id`. Sin el ADR, parece una complicación innecesaria.
- **ADR-5** documenta el requisito de `defer br.Close()` para `pgx.Batch`. Sin él, una refactorización que elimine el defer agotaría silenciosamente el pool de conexiones bajo carga.

Cuando se vuelva a este código en seis meses, o cuando alguien nuevo se una al proyecto, los ADRs responden las preguntas del "por qué" que el código no puede responder.

---

## Cero dependencias externas para nuevas funcionalidades

Las semanas 4 y 5 añadieron alertas, un dashboard en tiempo real, notificaciones webhook y seguimiento de salud de hosts usando únicamente la biblioteca estándar de Go:

- `html/template` para renderizado del lado del servidor
- `embed` para incrustar templates y assets estáticos en el binario
- `sync` para el mutex en `HostTracker`
- `context` para gestión de cancelación y timeouts
- `time` para tickers, timestamps y umbrales de vivacidad
- `encoding/json` para payloads de webhooks

Sin nuevos imports. Esta es una elección deliberada.

Cuando se recurre a una biblioteca, se deja de aprender cómo se resuelve realmente el problema. Una biblioteca es una respuesta correcta — pero oculta la pregunta. Si se quiere entender cómo funciona un motor de templates, construir uno. Si se quiere entender cómo funciona una primitiva de concurrencia, implementarla primero uno mismo. Luego usar la biblioteca en producción, con plena conciencia de lo que hace.

---

## Si quieres construir algo como esto

Construir un sistema desde cero para entenderlo es una de las estrategias de aprendizaje más efectivas disponibles. Algunos consejos prácticos:

1. **Elige un sistema que uses pero no entiendas.** No todo el stack — un componente. Un pipeline de métricas. Una capa de autenticación. Una cola. Una caché. Un rate limiter.

2. **Pregunta: ¿cómo sería la versión más simple?** No la más escalable. No la más lista para producción. La versión más simple que demuestre la idea central. MiniObserv no es Datadog. No necesita serlo.

3. **Escribe la especificación antes del código.** ¿Qué datos entran? ¿Qué datos salen? ¿Cuáles son las invariantes? Escríbelo. Si no puedes explicarlo en oraciones simples, no lo entiendes suficientemente bien para implementarlo.

4. **Añade tests antes de añadir funcionalidades.** Todo comportamiento nuevo tiene un test. El conjunto de tests es la versión ejecutable de la especificación.

5. **Publica algo, aunque esté incompleto.** El acto de desplegar, ver cómo corre y verlo fallar de maneras inesperadas enseña cosas que la especificación nunca enseñará. El agente de la semana 1 estaba incompleto. El servidor de la semana 2 no tenía dashboard. La semana 5 añadió cosas que la semana 1 no anticipó. Eso no es un problema — así se construyen los sistemas reales.

---

Este proyecto es de código abierto y fue construido para leerse, no solo para usarse. Si algo no está claro, los ADRs explican el por qué. Si quieres extenderlo, la guía de internos muestra dónde.
