---
layout: home

hero:
  name: "MiniObserv"
  text: "Observabilidad self-hosted"
  tagline: Recopila métricas y logs de tus servidores, almacénalos en TimescaleDB, dispara alertas y monitorea todo desde un dashboard en tiempo real. Escrito en Go 1.23+.
  actions:
    - theme: brand
      text: Inicio rápido →
      link: /es/inicio-rapido
    - theme: alt
      text: GitHub
      link: https://github.com/KamerrEzz/theminidog

features:
  - icon: 📊
    title: Métricas del sistema
    details: CPU, memoria, disco y red recopilados cada 10s por un agente ligero en Go. Sin dependencias en el servidor monitoreado.

  - icon: 📄
    title: Tail de logs
    details: El agente sigue cualquier archivo de log con detección de rotación. Las líneas aparecen en el dashboard en tiempo real.

  - icon: 🔔
    title: Alertas + Webhooks
    details: Define reglas como cpu.usage_pct > 80 por 5m. Dispara un POST a Slack, Discord, Teams o cualquier endpoint HTTP al activarse y resolverse.

  - icon: 🖥️
    title: Dashboard en vivo
    details: Tema oscuro con gráficas SVG sparklines, indicadores de tendencia, badges animados de alerta y estado de salud por host. Se actualiza cada 5 segundos.

  - icon: 🩺
    title: Estado de hosts
    details: Rastrea el último heartbeat por host. Marca hosts como stale (>20s) o down (>50s) y dispara una alerta sintética host.down con notificación webhook.

  - icon: 🗃️
    title: Almacenamiento TimescaleDB
    details: Métricas en hypertable con API de consulta por time-bucket. Retención automática (30d métricas / 14d logs) y compresión (7d) via TimescaleDB.

  - icon: 🔑
    title: Autenticación JWT
    details: Secreto HS256 compartido. El agente genera tokens de 24h automáticamente. El SDK de TypeScript hace lo mismo para apps Node.js.

  - icon: 🐳
    title: Docker listo
    details: Dockerfiles multi-stage para agente y servidor. Un docker compose up --build y todo está corriendo.
---

## Así se ve

```
┌─ MiniObserv ──────────────────── ● live  🔴 1 firing ──────┐
├─ HOSTS ────┬────────────────────────────────────────────────┤
│            │  ⚠ Memory Used > 8  |  actual: 10.36%          │
│ ● web-01   │                                                │
│ ● web-02   │  ┌──────────────┐  ┌──────────────┐           │
│ ◐ staging  │  │ CPU Usage    │  │ Memory Used  │           │
│ ✕ worker   │  │   0.70%      │  │   10.19%     │           │
│            │  │ → estable    │  │ ↑ subiendo   │           │
│            │  │ ▁▂▁▁▂▁▁▁    │  │ ▃▄▅▅▄▅▅▄▅   │           │
│            │  └──────────────┘  └──────────────┘           │
│            │                                                │
│            │  Logs (20)                                     │
│            │  16:42:31  INFO   GET /tasks → 200 (0ms)       │
│            │  16:42:31  ERROR  DB connection timeout        │
└────────────┴────────────────────────────────────────────────┘
```

Indicadores de host: **●** ok · **◐** stale · **✕** down

## Setup en 5 minutos

```bash
git clone https://github.com/KamerrEzz/theminidog.git
cd theminidog/deployments
docker compose up --build
```

Abre **http://localhost:8080** — el dashboard está disponible en cuanto el agente empieza a recopilar datos.

## Notificaciones de alertas

```bash
# Notificar a Slack cuando se supere un umbral
ALERT_RULES='[{"host":"*","name":"cpu.usage_pct","op":">","threshold":80,"for":"5m"}]'
ALERT_NOTIFICATIONS='[{"type":"webhook","url":"https://hooks.slack.com/services/TU/WEBHOOK"}]'
```

Cualquier webhook HTTP funciona — Slack, Discord, Teams, PagerDuty o tu propio endpoint.
