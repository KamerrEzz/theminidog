# Workflows y CI/CD

## ¿Por qué toda esta infraestructura?

Este proyecto nació de una pregunta: _"¿Qué pasaría si construyera un mini Datadog desde cero?"_

> "Quería entender cómo funciona Datadog por dentro. La mejor forma de aprender algo es construirlo."

Pero hacer una buena pregunta es solo el comienzo. El aprendizaje real ocurre cuando te comprometes a construir la respuesta con la misma disciplina que aplicarías en un trabajo real — no porque alguien te lo pida, sino porque tomar atajos en un proyecto personal te enseña los hábitos equivocados.

Por eso MiniObserv tiene pipelines de CI/CD, imágenes en Docker Hub, un SDK publicado en npm y un sitio de documentación bilingüe. No para sobre-ingeniería un proyecto personal. Sino porque:

- **Los proyectos reales tienen CI** para detectar regresiones automáticamente, antes de que lleguen a producción — o a un revisor.
- **Docker Hub** significa que cualquier persona puede probar el sistema en 30 segundos con un `docker pull`, sin clonar el repositorio ni instalar Go.
- **npm** significa que el SDK se puede usar desde cualquier proyecto Node.js con `npm install @kamerrezz/miniobserv-sdk`, igual que cualquier paquete profesional.
- **El sitio de documentación** significa que las personas pueden entender lo que construiste — no solo leer un README escrito en 20 minutos.

Este enfoque se aplicó desde el primer día: Spec-Driven Development antes de escribir una sola línea de código, TDD estricto en todo momento, Architecture Decision Records para cada decisión significativa. La infraestructura es la capa final de esa misma mentalidad.

---

## GitHub Actions — Cómo funciona cada workflow

Los cuatro workflows viven en `.github/workflows/`. Cada uno tiene una responsabilidad única y bien definida.

### `ci.yml` — Integración continua

**Disparadores:** cualquier push a `main`, y cualquier pull request que apunte a `main`.

```yaml
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
```

Este es el workflow más importante. Se ejecuta en cada cambio de código y hace dos cosas:

**1. Ejecutar el conjunto completo de pruebas**

```yaml
- name: Test
  run: go test ./... -count=1 -timeout 60s
```

El flag `-count=1` es intencional y vale la pena entenderlo. Go cachea los resultados de las pruebas por defecto — si nada cambió en un paquete, `go test` devuelve el resultado en caché sin ejecutar las pruebas de verdad. Eso es rápido, pero significa que una prueba intermitente o un fallo dependiente del entorno puede pasar silenciosamente. `-count=1` desactiva el caché y fuerza que cada prueba se ejecute de verdad, siempre.

El `-timeout 60s` es una red de seguridad. Si una prueba se cuelga (deadlock, canal bloqueado, conexión rota), todo el conjunto falla rápido en lugar de consumir minutos de CI hasta que se active el timeout máximo del job.

**2. Compilar ambos binarios**

```yaml
- name: Build
  run: |
    go build ./cmd/agent
    go build ./cmd/server
```

Las pruebas detectan errores de lógica. Pero un archivo puede compilar durante las pruebas y aun así fallar al construir el binario si, por ejemplo, un paquete `main` tiene una importación incorrecta o una dependencia `init()` faltante. Compilar explícitamente tanto `agent` como `server` detecta esos errores de compilación antes del merge.

**Efecto práctico:** si un PR rompe una prueba o no compila, este workflow falla y la rama queda bloqueada. No se necesita revisión humana para detectar esa clase de error.

---

### `docker.yml` — Construir y publicar imágenes Docker

**Disparadores:** tags de versión con formato `v*.*.*`, y ejecución manual.

```yaml
on:
  push:
    tags: ['v*.*.*']
  workflow_dispatch:
```

El disparador `workflow_dispatch` permite ejecutar el workflow manualmente desde la interfaz de GitHub Actions — útil para publicar una imagen `latest` o reconstruir tras un cambio en el Dockerfile sin crear un nuevo tag de release.

**Estrategia de matrix — construir server y agent en paralelo**

```yaml
strategy:
  matrix:
    include:
      - binary: server
        dockerfile: Dockerfile.server
        image: kamerrezz/miniobserv-server
      - binary: agent
        dockerfile: Dockerfile.agent
        image: kamerrezz/miniobserv-agent
```

En lugar de dos jobs separados o pasos secuenciales, una matrix ejecuta ambas compilaciones de imagen en paralelo. GitHub Actions asigna runners separados para cada entrada de la matrix, así que ambas imágenes se construyen simultáneamente. El resultado son dos repositorios en Docker Hub: `kamerrezz/miniobserv-server` y `kamerrezz/miniobserv-agent`.

**Generación automática de tags con `docker/metadata-action`**

```yaml
- name: Docker meta
  id: meta
  uses: docker/metadata-action@v5
  with:
    images: ${{ matrix.image }}
    tags: |
      type=semver,pattern={{version}}
      type=semver,pattern={{major}}.{{minor}}
      type=raw,value=latest,enable={{is_default_branch}}
```

Cuando se publica el tag `v1.2.3`, esta acción genera tres tags automáticamente:
- `kamerrezz/miniobserv-server:1.2.3` — versión exacta
- `kamerrezz/miniobserv-server:1.2` — versión minor flotante
- `kamerrezz/miniobserv-server:latest` — solo cuando se publica desde la rama principal

Esto sigue las convenciones de Docker Hub sin mantenimiento manual.

**Builds multiplataforma**

```yaml
- name: Set up QEMU
  uses: docker/setup-qemu-action@v3

- name: Set up Docker Buildx
  uses: docker/setup-buildx-action@v3

- name: Build and push
  uses: docker/build-push-action@v6
  with:
    platforms: linux/amd64,linux/arm64
```

QEMU emula arquitecturas de CPU no nativas. Buildx es el frontend de compilación extendido de Docker que habilita los builds multiplataforma. Juntos, producen un único manifest de imagen que funciona en:
- `linux/amd64` — servidores x86-64 estándar, la mayoría de VMs en la nube
- `linux/arm64` — Macs con Apple Silicon (`docker run` funciona de forma nativa), AWS Graviton, Raspberry Pi

Los usuarios no necesitan preocuparse por la plataforma. Docker descarga la variante correcta automáticamente.

**Caché de builds**

```yaml
cache-from: type=gha
cache-to: type=gha,mode=max
```

Las capas de Docker se almacenan en caché en el sistema de caché integrado de GitHub Actions. En ejecuciones posteriores, las capas que no cambiaron (la descarga de dependencias de Go, la configuración de la imagen base) se restauran desde el caché en lugar de reconstruirse. Esto convierte un build de 3–4 minutos en uno de 30–60 segundos después de la primera ejecución.

**Cómo disparar un release**

```bash
git tag v1.1.0
git push origin v1.1.0
```

Eso es todo. El workflow se activa, construye ambas imágenes para ambas plataformas, les aplica los tags y las publica en Docker Hub.

---

### `npm.yml` — Publicar el SDK de TypeScript

**Disparadores:** tags con formato `sdk/v*.*.*`, y ejecución manual.

```yaml
on:
  push:
    tags: ['sdk/v*.*.*']
```

El prefijo `sdk/` en el nombre del tag mantiene los releases del SDK separados de los releases de las imágenes Docker. Un tag como `sdk/v0.3.0` solo activa este workflow; un tag como `v1.0.0` solo activa el workflow de Docker. Separación limpia.

**Configuración de Node.js con autenticación npm**

```yaml
- uses: actions/setup-node@v4
  with:
    node-version: 20
    registry-url: 'https://registry.npmjs.org'
```

El campo `registry-url` hace más que configurar el endpoint del registro. También configura el archivo `.npmrc` en el runner para usar `NODE_AUTH_TOKEN` como credencial de autenticación. Sin este campo, el paso `npm publish` no tiene forma de autenticarse aunque el secret esté presente.

**Paso de compilación**

```yaml
- name: Build
  run: npm run build
  working-directory: sdk/js
```

El código fuente TypeScript se compila a JavaScript y archivos de declaraciones de tipos `.d.ts` en `dist/`. Esto es lo que realmente se publica en npm — los consumidores instalan un paquete pre-compilado, no TypeScript en bruto.

**Publicación**

```yaml
- name: Publish
  run: npm publish --access public
  working-directory: sdk/js
  env:
    NODE_AUTH_TOKEN: ${{ secrets.MINIOBSERV_NPM_TOKEN }}
```

`--access public` es necesario para paquetes con scope (paquetes cuyo nombre empieza con `@`), ya que estos paquetes son privados por defecto en npm. Sin este flag, el comando de publicación fallaría a menos que el paquete esté explícitamente configurado como público en `package.json`.

**Importante: el tipo de token importa.** El secret `MINIOBSERV_NPM_TOKEN` debe ser un token **Classic Automation** — no un token granular, y no un token Classic de usuario estándar. Los tokens de tipo Automation evitan la autenticación de dos factores (2FA), lo cual es necesario para publicar en un entorno de CI donde no hay ninguna persona presente para completar la 2FA. Un token granular o un token de usuario estándar fallará con un error de 2FA.

**Cómo disparar un release del SDK**

```bash
git tag sdk/v0.3.0
git push origin sdk/v0.3.0
```

---

### `docs.yml` — Compilar y desplegar el sitio de documentación

**Disparadores:** cualquier push a `main` que modifique archivos bajo `docs/`, o el propio archivo del workflow.

```yaml
on:
  push:
    branches: [main]
    paths:
      - 'docs/**'
      - '.github/workflows/docs.yml'
```

El filtro `paths` evita que el workflow de documentación se ejecute cuando solo cambia código Go o Dockerfiles. La documentación se despliega únicamente cuando la documentación cambia de verdad.

**Permisos**

```yaml
permissions:
  contents: read
  pages: write
  id-token: write
```

`pages: write` permite al workflow desplegar en GitHub Pages. `id-token: write` habilita la autenticación OIDC — el workflow prueba su identidad ante GitHub Pages mediante un token de corta duración en lugar de un secret almacenado. No se necesita ningún secret `PAGES_TOKEN`; GitHub gestiona la autenticación automáticamente.

**Control de concurrencia**

```yaml
concurrency:
  group: pages
  cancel-in-progress: false
```

`cancel-in-progress: false` significa que si ya hay un despliegue en curso y se dispara uno nuevo, el nuevo espera en lugar de cancelar el actual. Esto evita un sitio parcialmente desplegado — siempre obtienes un despliegue completo y consistente.

**Job de compilación**

```yaml
- name: Build docs
  run: npm run docs:build
  working-directory: docs

- uses: actions/upload-pages-artifact@v3
  with:
    path: docs/.vitepress/dist
```

VitePress compila todos los archivos markdown en un sitio estático en `docs/.vitepress/dist/`. La acción `upload-pages-artifact` empaqueta ese directorio y lo deja disponible para el job de despliegue.

**Job de despliegue**

```yaml
deploy:
  needs: build
  steps:
    - uses: actions/deploy-pages@v4
```

El job de despliegue espera a que la compilación tenga éxito (`needs: build`), y luego publica el artefacto en GitHub Pages mediante OIDC. La URL desplegada se establece automáticamente como URL del entorno en la interfaz de GitHub Actions.

**¿Por qué VitePress?**

- Sin configuración para markdown — escribes un archivo `.md` y obtienes una página
- Configuración en TypeScript con seguridad de tipos completa
- Internacionalización (i18n) integrada — todo el sitio funciona en inglés y español desde la misma configuración
- Búsqueda local integrada — sin necesidad de cuenta en Algolia
- Solo ESM — de ahí que `docs/package.json` tenga `"type": "module"`

---

## Configuración del sitio de documentación

El directorio `docs/` es la raíz del sitio VitePress. Cada archivo `.md` se convierte en una página en la ruta URL correspondiente.

**Archivos y directorios clave:**

| Ruta | Propósito |
|---|---|
| `docs/.vitepress/config.ts` | Navegación, sidebar, locales, búsqueda, URL base |
| `docs/index.md` | Página de inicio — usa frontmatter `layout: home` para el hero y la grilla de características |
| `docs/es/` | Traducción completa al español — espeja la estructura en inglés |

**Internacionalización**

La configuración define dos locales:

```ts
locales: {
  root: { label: 'English', lang: 'en-US', ... },
  es:   { label: 'Español', lang: 'es',    link: '/es/', ... },
}
```

Esto produce un selector de idioma en el encabezado. Las páginas en inglés están en `/getting-started`, las páginas en español en `/es/inicio-rapido`.

**URL base**

```ts
base: '/theminidog/'
```

El sitio está alojado en `kamerrezz.github.io/theminidog/`, no en el dominio raíz. La opción `base` prefija todos los enlaces internos y rutas de assets para que la navegación funcione correctamente. Sin ella, cada enlace devolvería un 404.

**`ignoreDeadLinks: true`** evita fallos de compilación por enlaces entre documentos que existen en un idioma pero aún no en el otro. Útil durante el desarrollo activo.

**`editLink.pattern`** agrega un enlace "Edit this page on GitHub" a cada página, apuntando directamente al archivo fuente. El patrón usa `:path` como marcador que VitePress reemplaza con la ruta real del archivo.

**Cómo agregar una nueva página**

```bash
# 1. Crear el archivo markdown en inglés
echo "# My new page" > docs/my-page.md

# 2. Agregarlo al sidebar en docs/.vitepress/config.ts
# (bajo la sección correspondiente en enSidebar)

# 3. Crear la versión en español
echo "# Mi nueva página" > docs/es/mi-pagina.md

# 4. Push — la documentación se despliega automáticamente al hacer merge a main
git add docs/ && git commit -m "docs: add my-page" && git push
```

---

## Secrets que necesitas

| Secret | Dónde obtenerlo | Para qué sirve |
|---|---|---|
| `DOCKERHUB_USERNAME` | Tu nombre de usuario en Docker Hub | Autentica el paso `docker push` |
| `DOCKERHUB_TOKEN` | Docker Hub → Account Settings → Security → New Access Token | Contraseña para `docker push` (usa un token, no tu contraseña de cuenta) |
| `MINIOBSERV_NPM_TOKEN` | npmjs.com → Access Tokens → Generate New Token → **Classic** → **Automation** | Publica en npm — debe ser un token Automation para evitar 2FA en CI |

Agrega los secrets en: `github.com/<tu-org>/<repo>/settings/secrets/actions`

**GitHub Pages no necesita un secret.** El permiso `id-token: write` en `docs.yml` gestiona la autenticación mediante OIDC automáticamente.

---

## Lo que puedes llevarte de este proyecto

Si estás leyendo esto porque quieres construir algo similar, esto es lo que MiniObserv demuestra de principio a fin:

- **Go como lenguaje de sistemas** — concurrente, con tipado estático, sin dependencias externas para el núcleo
- **Spec-Driven Development** — la especificación existe antes de la primera línea de implementación
- **TDD estricto** — ROJO → VERDE → REFACTOR, sin excepciones, sin "agrego pruebas después"
- **Architecture Decision Records** — documentar el POR QUÉ de una decisión, no solo el QUÉ se construyó
- **Builds multi-etapa en Docker** — imágenes de producción pequeñas, sin cadena de compilación incluida
- **Imágenes Docker multiplataforma** — un único `docker pull` que funciona en x86 y ARM
- **GitHub Actions para CI, CD y documentación** — automatizado desde las pruebas hasta el despliegue
- **TimescaleDB para datos de series temporales** — el modelo de almacenamiento correcto para métricas
- **Autenticación JWT sin librerías externas** — entendiendo lo que una librería realmente hace
- **VitePress para documentación técnica** — centrado en markdown, bilingüe, sin costo de infraestructura

> "No necesitas permiso para construir algo real. Empieza con una pregunta, especifícala, constrúyela con pruebas primero y publícala."

La infraestructura descrita en esta página requirió tiempo real para configurar y depurar. Pero una vez que funciona, se ejecuta automáticamente en cada commit. Esa inversión de tiempo se multiplica — cada cambio futuro se valida, publica y documenta sin pasos manuales.

Ese es el punto.
