# Construido con curiosidad

Este proyecto empezó con una pregunta simple: *"He escuchado de Datadog. ¿Cómo funciona realmente?"*

No "cómo se usa" — sino cómo *funciona*. ¿Qué pasa cuando una métrica sale de un servidor y termina en un dashboard en algún lugar? ¿Cómo recopila un sistema el uso de CPU de miles de máquinas y lo convierte en algo coherente en tiempo real? Quería entenderlo, no solo usarlo.

Datadog tenía plan de pago. No investigué más. En cambio pensé: *¿y si simplemente construyo mi propia versión?*

Ahí empezó todo.

---

## Quién construyó esto

Me llamo KamerrEzz. Soy un desarrollador full-stack de México, trabajando principalmente en el espacio SaaS — plataformas web, suscripciones, dashboards, ese tipo de cosas. JavaScript y TypeScript son mi casa. También escribo Lua cuando se presenta.

Go era nuevo para mí cuando empezó este proyecto. Estuve considerando .NET un tiempo, pero algo de Go encajó — se sentía como un lenguaje que podía darme lo que JavaScript no podía sin el costo de aprender un ecosistema completamente diferente. Ya conocía JavaScript, ya conocía Lua. Go se sentía como el paso correcto para el tipo de trabajo de sistemas que quería explorar.

Spoiler: Go me gustó mucho.

---

## Lo que la IA realmente hizo aquí

Quiero ser honesto sobre esto porque creo que hay una versión de esta historia que hace parecer que simplemente le dije a una IA que construyera un proyecto y apareció. Eso no fue lo que pasó.

Esto fue una colaboración 50/50.

La IA me ayudó a entender cosas que no sabía. Cuando necesitaba recopilar métricas de CPU de un sistema Linux, no sabía ni dónde vivían esos datos. La IA me explicó `/proc/stat`, me mostró el cálculo de deltas, me ayudó a entender por qué las métricas de red necesitan diferenciarse entre muestras. Cuando estaba aprendiendo patrones de concurrencia en Go — canales, goroutines, cancelación de contexto — la IA me ayudó a entender el *por qué* detrás de los patrones, no solo la sintaxis.

Yo era quien tomaba las decisiones. Elegí la arquitectura. Decidí qué construir cada semana. Revisé el código. Encontré bugs. Cuestioné cuando algo se sentía mal. Cuando la implementación de alertas tenía un detalle — disparar "resuelto" en la primera evaluación de una regla nueva — lo noté y lo corregimos.

Lo que me hubiera tomado cuatro a seis meses construir solo — aprender Go, entender sistemas de observabilidad, escribir 213 tests — ocurrió en cinco semanas. No porque la IA lo escribió por mí. Sino porque la IA comprimió dramáticamente la curva de aprendizaje. Usé mi tiempo en decisiones y comprensión, no en descifrar cómo parsear un string de duración.

Hay una diferencia entre usar la IA como atajo y usarla como maestro. Esto fue lo segundo.

---

## El momento en que decidí tomarlo en serio

Alrededor de la Semana 2, me di cuenta de que esto no era solo un experimento desechable. El código estaba limpio. Los tests pasaban. La arquitectura tenía sentido. Empecé a pensar: *¿y si alguien más pudiera usar esto?*

Ahí apareció el SDK. Mencioné querer tener un cliente TypeScript — solo para mí. Al final de la conversación, había un paquete npm publicado. No lo había planeado. El proceso de construir bien llevó naturalmente a publicar bien.

Las imágenes de Docker Hub vinieron del mismo lugar. El sitio de documentación. Los docs bilingües. Los workflows de GitHub Actions. Nada de esto estaba en el plan original. Sucedieron porque una vez que empiezas a construir algo real, el siguiente paso correcto se vuelve obvio.

---

## Qué tiene que ver Zeew Space con esto

Dirijo [Zeew Space](https://zeew.space) — una plataforma donde aprendes a programar construyendo cosas reales desde el primer día. No es teoría en videos infinitos. El modelo es: entiendes un concepto, lo aplicas inmediatamente en código, recibes feedback de un humano, y terminas con un proyecto que puedes mostrar en tu portafolio.

Funciona en rutas — caminos progresivos donde cada curso es la base del siguiente. Aprendes JavaScript, después React, después Next.js, y al final de cada etapa tienes algo funcional que es tuyo. La IA está integrada como herramienta que acelera lo que ya sabes — no como muleta.

La diferencia: otras plataformas te enseñan a programar. Zeew te enseña a pensar como programador, después te enseña la sintaxis.

Si quieres aprender a programar — o profundizar en sistemas, desarrollo asistido por IA, o construir productos reales:

**[zeew.space/discord](https://zeew.space/discord)** — únete a la comunidad, recibe feedback sobre lo que construyes, y aprende junto a personas que están haciendo lo mismo.

Este proyecto es un ejemplo de cómo se ve aprender construyendo en la práctica.

---

## Lo que le diría a alguien empezando desde cero

Si quieres construir algo como esto — un sistema real, no un proyecto de tutorial — aquí está la versión honesta:

**No necesitas saber todo primero.** No sabía Go cuando empecé. No sabía cómo se recopilaban las métricas de CPU. No sabía cómo funcionaban las consultas time-bucket de TimescaleDB. Aprendí todo eso durante el proyecto, no antes.

**Elige algo que realmente quieras entender.** No una app de tareas. No un clon de algo que ya construiste. Elige un sistema que usas pero no entiendes. Esa curiosidad es el combustible. Sin ella, te detendrás cuando se ponga difícil.

**Usa la IA como maestro, no como ghostwriter.** Pídele que explique cosas. Pregunta por qué. Cuando sugiera algo, entiéndelo antes de usarlo. En el momento en que dejas de entender tu propio código, perdiste el aprendizaje.

**Publica algo.** El acto de hacerlo público — aunque sea una función, aunque sea imperfecto — cambia cómo construyes. Empiezas a preocuparte por los archivos README y los mensajes de error y qué pasa cuando alguien más ejecuta tu código por primera vez. Ahí es donde está el aprendizaje real.

---

## Pensamiento final

No sé para qué usaré Go en el próximo proyecto. No sé si necesitaré un sistema de monitoreo para algo real. Pero ahora entiendo cómo funciona uno — desde el agente recopilando métricas cada diez segundos hasta la hypertable almacenándolas y el sparkline SVG renderizándose en el navegador.

Ese entendimiento no caduca.

---

*KamerrEzz — Zeew Space · [GitHub](https://github.com/KamerrEzz) · [Discord](https://zeew.space/discord)*
