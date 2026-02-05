# Fyndmark (fyndmark-backend)

Fyndmark is a lightweight, self-hosted comment backend for Hugo sites that follow a Git-based workflow.

The central idea is that comments should behave like regular content, not like dynamic data that requires a database at page render time. Instead of injecting comments at runtime, Fyndmark collects and moderates them, then writes them back into the Hugo project as Markdown files. From Hugo’s perspective, they are just normal files inside each page bundle.

Because of this design, Git is not an add-on but a fundamental part of the system. Every approved comment becomes a committed file in the repository. The website itself remains fully static, can be deployed anywhere, and does not depend on a running backend or external service once built.

Fyndmark focuses purely on the backend responsibilities: receiving comments, storing them safely in SQLite, handling moderation, and updating the repository. Rendering and frontend integration live separately in the Hugo theme component:

https://github.com/geschke/hugo-fyndmark

Together, the backend and the theme form a small, fully self-hosted comment system that integrates naturally into existing Hugo and Git workflows.


## How it works

When a visitor submits a comment, the backend stores it in SQLite and marks it as pending. An administrator receives a moderation email with signed approve or reject links. Only approved comments are written to disk.

After approval, Fyndmark checks out the configured Git repository, generates Markdown files for the comments inside the corresponding page bundles, commits the changes, and pushes them back to the remote. This keeps the repository as the single source of truth for both content and comments.

Running Hugo itself is optional. Some setups prefer to build the site later during deployment, for example in a CI pipeline, and only commit Markdown sources. Others let Fyndmark run Hugo locally and commit the generated HTML output as well. Both approaches are supported, but the Git workflow is always part of the process.

CAPTCHA protection can be enabled if desired. Supported providers currently include Turnstile and hCaptcha, but the system also works without any CAPTCHA.


## Typical flow

1. A visitor submits a comment through the frontend.
2. The backend stores it as `pending` in SQLite.
3. An administrator reviews the comment via email.
4. Approval generates Markdown files and updates the Git repository (optionally running Hugo).
5. The next deployment already contains the new comment as static content.


## Installation

Fyndmark is primarily intended to run using the provided Docker image. The image already contains everything required at runtime, including Git and Hugo, so no additional tools need to be installed on the host system.

Alternatively, you can build Fyndmark from source and run the binary directly. In that case, you must provide the required tools yourself. At minimum, `git` must be available on the system, and if you enable the integrated Hugo build step, `hugo` must be installed as well.


### Quick start (Docker)

The easiest way to get started is Docker. The official image already contains everything Fyndmark needs at runtime, including Git and Hugo, so no additional tools have to be installed on the host system.

Mount your configuration file and two directories: one for the database and one for the working copy of your website repository. Then start the container:

```bash
docker run \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/config/config.yaml:ro \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/website:/app/website \
  ghcr.io/geschke/fyndmark:latest
```

After startup, the server listens on port 8080 and is ready to receive comments.

The mounted paths have the following purpose:

* `config.yaml` contains all application settings
* `data/` stores the SQLite database
* `website/` is used as the Git working directory where the repository is checked out and comment files are generated


### Build from source

If you prefer running Fyndmark directly without containers, you can build the binary yourself. This is especially convenient during development:

```bash
go build -o fyndmark .
./fyndmark serve --config ./config.yaml
```

The behavior is identical to Docker; only the packaging differs.

When running the compiled binary directly, `git` must be available in `PATH`. `hugo` is additionally required unless `hugo.disabled: true`.

### Docker Compose

For permanent or production-like deployments, Docker Compose is often the most convenient option. 
The container image already includes everything required (`git` and optionally `hugo`), so no additional tools are needed on the host.

Minimal example of docker-compose.yml file:

```
services:
  fyndmark:
    image: ghcr.io/geschke/fyndmark:latest
    container_name: fyndmark
    restart: unless-stopped

    ports:
      - "8080:8080"

    volumes:
      # Configuration file
      - ./config.yaml:/config/config.yaml:ro

      # SQLite database location
      - ./data:/app/data

      # Hugo website working copy (git checkout happens here)
      - ./website:/app/website

    command: serve --config /config/config.yaml
```

Start it:


```bash
docker compose -f docker-compose.yml up -d
```

This sets up persistent volumes and runs Fyndmark as a background service.

### Requirements

At minimum, Fyndmark needs a writable SQLite database and access to your Git repository. If you enable the integrated pipeline, the system also requires the `git` binary and, optionally, `hugo` to be available locally.

If you build your site elsewhere, for example in a CI pipeline, you can disable the Hugo step and only commit the generated Markdown files.


## Configuration

Fyndmark is configured via a single `config.yaml` file (or environment variables), and the configuration is intentionally “small but complete”. Because the workflow is based on email moderation, SQLite storage, and a Git working copy, most settings are required for a functional setup. The example configuration above is meant as a realistic starting point; you can copy it and adjust it to your site.

Configuration is grouped into three main areas: the HTTP server settings, the SQLite database path, and one or more comment sites under `comment_sites`. Each comment site is identified by its site ID (the map key) and provides all site-specific settings such as CORS, moderation recipients, Git access, optional CAPTCHA, and whether the integrated Hugo build step should be skipped.

### Where config values come from

Fyndmark loads configuration in this order:

1. `--config <file>`
2. Environment variables
3. Files named `config.*` in `.`, `./config`, or `/config` (YAML/JSON/TOML), with `.env` as a fallback


### Example `config.yaml`

Use this as a reference and replace values with your own:

```yaml
server:
  listen: ":8080"
  log_level: "debug"   # currently not used

smtp:
  host: "smtp.example.org"
  port: 587
  from: "noreply@example.org"
  tls_policy: "opportunistic"   # none | opportunistic | mandatory
  username: "smtp-user"
  password: "smtp-pass"

sqlite:
  path: "./data/fyndmark.db"

comment_sites:
  my_site:
    title: "Comments for my site"
    cors_allowed_origins:
      - "https://example.org"
      - "http://localhost:1313"
    admin_recipients:
      - "admin@example.org"
    token_secret: "CHANGE-ME-TO-A-LONG-RANDOM-STRING"
    timezone: "Europe/Berlin"
    captcha:
      enabled: false
      provider: "turnstile"
      #secret_key: "YOUR_TURNSTILE_SECRET"
    hugo:
      disabled: false
    git:
      repo_url: "https://github.com/you/your-hugo-site.git"
      branch: "main"
      access_token: ""   # empty if public
      depth: 1
      themes:
        - name: "fyndmark"
          repo_url: "https://github.com/geschke/hugo-fyndmark.git"
          branch: "main"
          target_path: "themes/hugo-fyndmark"
          access_token: ""
          depth: 1

```



### Options reference

### `server`

`server.listen` defines the address the HTTP server binds to, for example `:8080` or `0.0.0.0:8080`.

* `listen` (string, required)

### `sqlite`

`sqlite.path` points to the SQLite database file used to store comments and pipeline run status.

* `path` (string, required)

Ensure the directory for `sqlite.path` exists and is writable by the process.

### `smtp`

SMTP is used to send moderation emails (approve/reject links) to the configured administrators.

* `host` (string, required)
* `port` (int, optional): if omitted or `0`, the library default is used
* `from` (string, required)
* `username` (string, optional)
* `password` (string, optional)
* `tls_policy` (string, optional): controls TLS behavior for SMTP. Supported values are `none`, `opportunistic`, and `mandatory`.

### `comment_sites`

`comment_sites` is the core of the configuration. Each entry defines one Hugo site/blog. The key (for example `geschke_net`) is the site ID and is used in API routes like `/api/comments/:siteid`.

Common fields:

* `title` (string, optional): human-readable label used for logging and emails
* `cors_allowed_origins` (list of strings, required): allowed origins for browser requests (typically your site URL and local Hugo preview)
* `admin_recipients` (list of strings, required): moderation email recipients
* `token_secret` (string, required): A long random secret string used to sign moderation links. Generate a sufficiently long, unpredictable value.
* `timezone` (string, optional): IANA timezone string (for example `Europe/Berlin`). Default is `UTC`.

#### `comment_sites.<site>.captcha` (optional)

The `captcha` section is optional.
If omitted entirely, no captcha validation is performed.
If present, all required fields (`provider`, `secret_key`) must be set, even when `enabled: false`.
The `enabled` flag is intended only to temporarily disable an otherwise complete configuration.

* `enabled` (bool, optional)
* `provider` (string, required if enabled): currently supported values are `turnstile` and `hcaptcha`
* `secret_key` (string, required if enabled): provider secret used by the backend to verify tokens

#### `comment_sites.<site>.hugo` (optional)

The Hugo step is integrated but optional. By default it runs after comment generation. Set `disabled: true` to skip it (for example when your deployment pipeline runs Hugo elsewhere).

* `disabled` (bool, optional, default: false)

#### `comment_sites.<site>.git`

Git is required because the workflow writes generated Markdown comment files into a working copy and pushes changes back to the remote repository.

* `repo_url` (string, required): HTTPS URL to the Hugo site repository
* `branch` (string, optional): if unset, Git uses the default branch
* `access_token` (string, optional): used for HTTPS token auth; can be empty for public repos
* `clone_dir` (string, optional): target directory for the working copy. If unset, a default directory is used (for example `./website/<site_id>`).
* `depth` (int, optional): shallow clone depth; `0` means full clone
* `recurse_submodules` (bool, optional): if true, submodules are initialized/updated during clone (use this if your Hugo site uses submodules for themes/components)

##### `comment_sites.<site>.git.themes` (optional)

`git.themes` is an optional convenience feature that allows Fyndmark to clone additional theme/component repositories into the checked out website working copy (typically under `themes/`). This is useful if you do not use Git submodules or if you want Fyndmark to ensure specific theme directories exist.

Each entry supports:

* `name` (string, optional): label used for logging
* `repo_url` (string, required)
* `branch` (string, optional)
* `target_path` (string, required): path inside the checked out website repo (for example `themes/hugo-fyndmark`)
* `access_token` (string, optional)
* `depth` (int, optional)




## Access to private Git repositories

Fyndmark accesses your Hugo site repository via normal HTTPS Git commands (`clone`, `commit`, `push`).
If the repository is private, authentication is required.

This is done using a Personal Access Token which is configured as `git.access_token`.

If the repository is public, this setting can simply be left empty.

### GitHub (step-by-step)

On GitHub, create a **fine-grained personal access token**:

1. Open **Settings → Developer settings → Personal access tokens → Fine-grained tokens**
2. Click **Generate new token**
3. Select:

   * Repository access → **Only select repositories**
   * choose your Hugo repository
4. Permissions:

   * Repository permissions → **Contents → Read and write**
   * nothing else is required
5. Generate the token and copy it

Then add it to your `config.yaml`:

```yaml
git:
  repo_url: "https://github.com/you/your-hugo-site.git"
  access_token: "github_pat_xxxxxxxxxxxxxxxxx"
```

That’s all. Fyndmark will automatically use this token for clone and push.

### GitLab

GitLab provides the same mechanism via Personal Access Tokens.

Create a token with:

* `read_repository`
* `write_repository`

and use it exactly like with GitHub.

### Security notes

* Treat the token like a password
* Do not commit it to Git
* Prefer environment variables or Docker secrets in production
* Fyndmark never prints or logs the token










## API endpoints

### `POST /api/comments/:siteid`
Creates a new comment (JSON). Example payload:

```json
{
  "entry_id": "post-123",
  "post_path": "/posts/hello-world/",
  "parent_id": "",
  "author": "Jane",
  "email": "jane@example.org",
  "author_url": "https://example.org",
  "body": "Nice post!",
  "captcha_token": "..."
}
```

### `GET /api/comments/:siteid/decision?token=...`
Approve or reject via signed token (used by moderation emails).

### `POST /api/feedbackmail/:formid`
Sends a feedback mail based on `forms.<id>` config. Form fields are submitted as standard form values.

### `GET /health`
Basic health check.
