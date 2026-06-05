# Workflows & CI/CD

## Why all this infrastructure?

This project started as a question: _"What if I built a mini Datadog from scratch?"_

> "I wanted to learn how Datadog works under the hood. The best way to learn something is to build it."

But asking a good question is only the beginning. The real learning happens when you commit to building the answer with the same discipline you'd apply at a real job — not because someone told you to, but because cutting corners on a side project teaches you the wrong habits.

That's why MiniObserv has CI/CD pipelines, Docker Hub images, an npm-published SDK, and a bilingual documentation site. Not to over-engineer a side project. But because:

- **Real projects have CI** so you catch regressions automatically, before they reach production — or a reviewer.
- **Docker Hub** means anyone can try the system in 30 seconds with a single `docker pull`, without cloning a repo or installing Go.
- **npm** means the SDK is usable from any Node.js project with `npm install @kamerrezz/miniobserv-sdk`, just like any other professional package.
- **The docs site** means people can actually understand what you built — not just read a README that was written in 20 minutes.

This approach was applied from day one: Spec-Driven Development before writing a single line of code, strict TDD throughout, Architecture Decision Records for every significant choice. The infrastructure is the final layer of that same mindset.

---

## GitHub Actions — How each workflow works

All four workflows live in `.github/workflows/`. Each has a single, well-defined responsibility.

### `ci.yml` — Continuous Integration

**Triggers:** any push to `main`, and any pull request targeting `main`.

```yaml
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
```

This is the most important workflow. It runs on every code change and does two things:

**1. Run the full test suite**

```yaml
- name: Test
  run: go test ./... -count=1 -timeout 60s
```

The `-count=1` flag is intentional and worth understanding. Go caches test results by default — if nothing in a package changed, `go test` will return the cached result without actually running the tests. That's fast, but it means a flaky test or an environment-dependent failure can silently pass. `-count=1` disables the cache and forces every test to run for real, every time.

The `-timeout 60s` is a safety net. If a test hangs (deadlock, blocking channel, broken connection), the whole suite fails fast instead of consuming CI minutes until the job's maximum timeout kicks in.

**2. Build both binaries**

```yaml
- name: Build
  run: |
    go build ./cmd/agent
    go build ./cmd/server
```

Tests catch logic errors. But a file can compile during tests and still fail to build as a binary if, for example, a `main` package has a bad import or a missing `init()` dependency. Building both `agent` and `server` explicitly catches those compilation errors before merge.

**Practical effect:** if a PR breaks a test or fails to compile, this workflow fails and the branch is blocked from merging. No human review needed to catch that class of error.

---

### `docker.yml` — Build and push Docker images

**Triggers:** version tags matching `v*.*.*`, and manual dispatch.

```yaml
on:
  push:
    tags: ['v*.*.*']
  workflow_dispatch:
```

The `workflow_dispatch` trigger lets you run the workflow manually from the GitHub Actions UI — useful for pushing a `latest` image or rebuilding after a Dockerfile change without creating a new release tag.

**Matrix strategy — building server and agent in parallel**

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

Instead of two separate jobs or sequential steps, a matrix runs both image builds in parallel. GitHub Actions allocates separate runners for each matrix entry, so both images build simultaneously. The result is two Docker Hub repositories: `kamerrezz/miniobserv-server` and `kamerrezz/miniobserv-agent`.

**Automatic tag generation with `docker/metadata-action`**

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

When you push tag `v1.2.3`, this action generates three tags automatically:
- `kamerrezz/miniobserv-server:1.2.3` — exact version
- `kamerrezz/miniobserv-server:1.2` — minor version float
- `kamerrezz/miniobserv-server:latest` — only when pushed from the default branch

This follows Docker Hub conventions without manual bookkeeping.

**Multi-platform builds**

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

QEMU emulates non-native CPU architectures. Buildx is Docker's extended build frontend that enables multi-platform builds. Together, they produce a single image manifest that works on:
- `linux/amd64` — standard x86-64 servers, most cloud VMs
- `linux/arm64` — Apple Silicon Macs (`docker run` works natively), AWS Graviton, Raspberry Pi

Users don't need to care about platform. Docker pulls the right variant automatically.

**Build cache**

```yaml
cache-from: type=gha
cache-to: type=gha,mode=max
```

Docker layers are cached in GitHub Actions' built-in cache. On subsequent runs, layers that haven't changed (the Go dependency download, the base image setup) are restored from cache instead of rebuilt. This turns a 3–4 minute build into a 30–60 second one after the first run.

**How to trigger a release**

```bash
git tag v1.1.0
git push origin v1.1.0
```

That's it. The workflow fires, builds both images for both platforms, tags them, and pushes to Docker Hub.

---

### `npm.yml` — Publish TypeScript SDK

**Triggers:** tags matching `sdk/v*.*.*`, and manual dispatch.

```yaml
on:
  push:
    tags: ['sdk/v*.*.*']
```

The `sdk/` prefix in the tag name keeps SDK releases separate from Docker image releases. A tag like `sdk/v0.3.0` triggers only this workflow; a tag like `v1.0.0` triggers only the Docker workflow. Clean separation.

**Node.js setup with npm authentication**

```yaml
- uses: actions/setup-node@v4
  with:
    node-version: 20
    registry-url: 'https://registry.npmjs.org'
```

The `registry-url` field does more than configure the registry endpoint. It also sets up the `.npmrc` file in the runner to use `NODE_AUTH_TOKEN` as the authentication credential. Without this field, the `npm publish` step has no way to authenticate even if the secret is present.

**Build step**

```yaml
- name: Build
  run: npm run build
  working-directory: sdk/js
```

The TypeScript source is compiled to JavaScript and `.d.ts` type declaration files in `dist/`. This is what actually gets published to npm — consumers install a pre-compiled package, not raw TypeScript.

**Publish**

```yaml
- name: Publish
  run: npm publish --access public
  working-directory: sdk/js
  env:
    NODE_AUTH_TOKEN: ${{ secrets.MINIOBSERV_NPM_TOKEN }}
```

`--access public` is required for scoped packages (packages whose name starts with `@`) because scoped packages default to private on npm. Without this flag, the publish command would fail unless the package were explicitly set to public in `package.json`.

**Important: the token type matters.** The `MINIOBSERV_NPM_TOKEN` secret must be a **Classic Automation token** — not a granular token, and not a Classic User token. Automation tokens bypass two-factor authentication, which is required for publishing in a CI environment where there's no human present to complete 2FA. A granular token or a standard user token will fail with a 2FA error.

**How to trigger an SDK release**

```bash
git tag sdk/v0.3.0
git push origin sdk/v0.3.0
```

---

### `docs.yml` — Build and deploy documentation site

**Triggers:** any push to `main` that touches files under `docs/`, or the workflow file itself.

```yaml
on:
  push:
    branches: [main]
    paths:
      - 'docs/**'
      - '.github/workflows/docs.yml'
```

The `paths` filter prevents the docs workflow from running when only Go code or Dockerfiles change. Documentation deploys only when documentation actually changes.

**Permissions**

```yaml
permissions:
  contents: read
  pages: write
  id-token: write
```

`pages: write` allows the workflow to deploy to GitHub Pages. `id-token: write` enables OIDC authentication — the workflow proves its identity to GitHub Pages via a short-lived token instead of a stored secret. No `PAGES_TOKEN` secret is needed; GitHub handles authentication automatically.

**Concurrency control**

```yaml
concurrency:
  group: pages
  cancel-in-progress: false
```

`cancel-in-progress: false` means if a deploy is already running and a new one is triggered, the new one waits instead of canceling the current one. This prevents a partially-deployed site — you always get a complete, consistent deployment.

**Build job**

```yaml
- name: Build docs
  run: npm run docs:build
  working-directory: docs

- uses: actions/upload-pages-artifact@v3
  with:
    path: docs/.vitepress/dist
```

VitePress compiles all markdown files into a static site in `docs/.vitepress/dist/`. The `upload-pages-artifact` action packages that directory and makes it available to the deploy job.

**Deploy job**

```yaml
deploy:
  needs: build
  steps:
    - uses: actions/deploy-pages@v4
```

The deploy job waits for the build to succeed (`needs: build`), then publishes the artifact to GitHub Pages via OIDC. The deployed URL is automatically set as the environment URL in the GitHub Actions UI.

**Why VitePress?**

- Zero configuration for markdown — write a `.md` file, get a page
- TypeScript configuration with full type safety
- Built-in internationalization (i18n) — the entire site runs in English and Spanish from the same config
- Local search out of the box — no Algolia account needed
- ESM-only — hence `"type": "module"` in `docs/package.json`

---

## Documentation site setup

The `docs/` directory is the VitePress site root. Every `.md` file becomes a page at the corresponding URL path.

**Key files and directories:**

| Path | Purpose |
|---|---|
| `docs/.vitepress/config.ts` | Navigation, sidebar, locales, search, base URL |
| `docs/index.md` | Landing page — uses `layout: home` frontmatter for the hero and features grid |
| `docs/es/` | Full Spanish translation — mirrors the English structure |

**Internationalization**

The config defines two locales:

```ts
locales: {
  root: { label: 'English', lang: 'en-US', ... },
  es:   { label: 'Español', lang: 'es',    link: '/es/', ... },
}
```

This produces a language switcher in the header. English pages live at `/getting-started`, Spanish pages at `/es/inicio-rapido`.

**Base URL**

```ts
base: '/theminidog/'
```

The site is hosted at `kamerrezz.github.io/theminidog/`, not at the root domain. The `base` setting prefixes all internal links and asset paths so navigation works correctly. Without it, every link would 404.

**`ignoreDeadLinks: true`** prevents build failures from cross-doc links that exist in one language but not yet in the other. Useful during active development.

**`editLink.pattern`** adds an "Edit this page on GitHub" link to every page, pointing directly to the source file. This pattern uses `:path` as a placeholder that VitePress replaces with the actual file path.

**How to add a new page**

```bash
# 1. Create the English markdown file
echo "# My new page" > docs/my-page.md

# 2. Add it to the sidebar in docs/.vitepress/config.ts
# (under the relevant section in enSidebar)

# 3. Create the Spanish version
echo "# Mi nueva página" > docs/es/mi-pagina.md

# 4. Push — docs deploy automatically on merge to main
git add docs/ && git commit -m "docs: add my-page" && git push
```

---

## Secrets you need

| Secret | Where to get it | What it does |
|---|---|---|
| `DOCKERHUB_USERNAME` | Your Docker Hub username | Authenticates the `docker push` step |
| `DOCKERHUB_TOKEN` | Docker Hub → Account Settings → Security → New Access Token | Password for `docker push` (use a token, not your account password) |
| `MINIOBSERV_NPM_TOKEN` | npmjs.com → Access Tokens → Generate New Token → **Classic** → **Automation** | Publishes to npm — must be an Automation token to bypass 2FA in CI |

Add secrets at: `github.com/<your-org>/<repo>/settings/secrets/actions`

**GitHub Pages does not need a secret.** The `id-token: write` permission in `docs.yml` handles authentication via OIDC automatically.

---

## What you can take from this project

If you're reading this because you want to build something similar, here is what MiniObserv demonstrates end to end:

- **Go as a systems language** — concurrent, strongly typed, zero external dependencies for the core
- **Spec-Driven Development** — the spec exists before the first line of implementation
- **Strict TDD** — RED → GREEN → REFACTOR, no exceptions, no "I'll add tests later"
- **Architecture Decision Records** — documenting WHY a decision was made, not just WHAT was built
- **Docker multi-stage builds** — small production images with no build toolchain included
- **Multi-platform Docker images** — a single `docker pull` that works on x86 and ARM
- **GitHub Actions for CI, CD, and documentation** — automated from test to deployment
- **TimescaleDB for time-series data** — the right storage model for metrics
- **JWT authentication without external libraries** — understanding what a library actually does
- **VitePress for technical documentation** — markdown-first, bilingual, zero infrastructure cost

> "You don't need permission to build something real. Start with a question, spec it out, build it test-first, and ship it."

The infrastructure described on this page took real time to set up and debug. But once it's working, it runs automatically on every commit. That time investment compounds — every future change is validated, published, and documented without any manual steps.

That's the point.
