# **Fyntral**

Fyntral is a small, self-hosted email relay for web forms.
It accepts form submissions, validates fields, optionally verifies Cloudflare Turnstile tokens, and sends the collected data via SMTP.
It was originally built to replace a simple AWS Lambda mail function — AWS was unnecessary overhead for this specific task.

Fyntral is intentionally minimal but can be extended if needed.

---

## **Features**

* Per-form configuration
* SMTP with optional TLS (`none`, `starttls`, `tls`)
* Cloudflare Turnstile (per form)
* Per-form CORS configuration
* Field-by-field validation
* Environment-variable overrides
* Runs as a single static binary or as a Docker container

---

## Building from source

You can build Fyntral locally using the standard Go toolchain.
Go 1.21+ is recommended.

### Clone and build

```bash
git clone https://github.com/geschke/fyntral.git
cd fyntral
go build -o fyntral .
```

This produces a local binary named `fyntral`.

### Install globally

If you prefer installing it into your `$GOPATH/bin` (or Go's module-aware bin directory):

```bash
go install github.com/geschke/fyntral@latest
```

The binary `fyntral` will be placed in:

```
$GOPATH/bin
```

or for module-managed installs:

```
~/go/bin
```

Make sure that directory is available in your `$PATH`.

---



## **Configuration**

Fyntral loads configuration from:

1. `--config <file>`
2. Environment variables
3. Files in `.`, `./config`, `/config` (`config.yaml`, `.env`, JSON, TOML)

### **Example `config.yaml`**

```yaml
server:
  listen: "0.0.0.0:8080"
  log_level: "info" # currently not used

smtp:
  host: "smtp.example.org"
  port: 587
  from: "noreply@example.org"
  tls_policy: "starttls"   # none | starttls | tls
  username: "smtp-user"
  password: "smtp-pass"

forms:
  example_form:
    title: "Example contact form"
    recipients:
      - "admin@example.org"
    subject_prefix: "[Contact]"
    cors_allowed_origins:
      - "https://example.org"
      - "http://localhost:1313"
    turnstile:
      enabled: true
      secret_key: "YOUR_TURNSTILE_SECRET"
    fields:
      - name: "name"
        label: "Name"
        type: "text"
        required: true
      - name: "email"
        label: "E-Mail"
        type: "email"
        required: true
      - name: "message"
        label: "Message"
        type: "text"
        required: true
```

---

## **API**

### **POST `/api/feedbackmail/:formid`**

Example:

```
POST /api/feedbackmail/example_form
```

POST body fields are defined in the config under `forms.<id>.fields`.

---

## **Environment Variables**

Viper maps config keys to environment variables using underscores:

```
SERVER_LISTEN=0.0.0.0:8080
SMTP_HOST=smtp.example.org
SMTP_TLS_POLICY=starttls
```

Per-form values also work:

```
FORMS_EXAMPLE_FORM_TURNSTILE_ENABLED=true
FORMS_EXAMPLE_FORM_TURNSTILE_SECRET_KEY=abc123
FORMS_EXAMPLE_FORM_RECIPIENTS="a@example.org,b@example.org"
FORMS_EXAMPLE_FORM_CORS_ALLOWED_ORIGINS="https://one,https://two"
```

---

## **Running with Docker**

### **Direct run**

```
docker run \
  -p 8080:8080 \
  -v $(pwd)/config:/config \
  ghcr.io/geschke/fyntral:latest \
  serve --config /config/config.yaml
```

---

## **Docker Compose**

```yaml
services:
  fyntral:
    image: ghcr.io/geschke/fyntral:latest
    container_name: fyntral
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - ./config:/config
    command: ["serve", "--config", "/config/config.yaml"]
```

---

## **Purpose**

Fyntral exists to provide:

* a simple, fully self-hosted alternative to cloud-based mail handling
* no external dependencies
* minimal overhead
* predictable behavior

It is built for personal use but can be used anywhere a lightweight form-to-mail bridge is needed.

---

## Running behind Traefik (reverse proxy)

Fyntral is a good fit for running behind a reverse proxy such as Traefik.
In this setup the container only listens on an internal port (e.g. `8080`), and Traefik terminates TLS and routes external requests (e.g. `https://func.example.org`) to the service.

A minimal example with Traefik labels might look like this:

```yaml
services:
  fyntral:
    image: ghcr.io/geschke/fyntral:latest
    container_name: fyntral
    restart: unless-stopped
    volumes:
      - ./config.yaml:/config/config.yaml:ro
    environment:
      TZ: "Europe/Berlin"
    networks:
      - traefik-public
    labels:
      - "traefik.enable=true"
      - "traefik.docker.network=traefik-public"

      # HTTP → redirect to HTTPS
      - "traefik.http.routers.fyntral.rule=Host(`func.example.org`)"
      - "traefik.http.routers.fyntral.entrypoints=http"
      - "traefik.http.middlewares.fyntral-https-redirect.redirectscheme.scheme=https"
      - "traefik.http.middlewares.fyntral-https-redirect.redirectscheme.permanent=true"
      - "traefik.http.routers.fyntral.middlewares=fyntral-https-redirect"

      # HTTPS router
      - "traefik.http.routers.fyntral-secured.rule=Host(`func.example.org`)"
      - "traefik.http.routers.fyntral-secured.entrypoints=https"
      - "traefik.http.routers.fyntral-secured.tls.certresolver=le-tls"

      # Forward to the internal Fyntral port
      - "traefik.http.services.fyntral-secured.loadbalancer.server.port=8080"

      # Optional: security / compression middlewares defined in Traefik file providers
      - "traefik.http.routers.fyntral-secured.middlewares=secHeaders@file,def-compress@file"

networks:
  traefik-public:
    external: true
```

In this configuration:

* Fyntral listens only on `8080` inside the Docker network.
* Traefik handles HTTP/HTTPS entrypoints and TLS certificates.
* The host `func.example.org` is routed to the Fyntral container without exposing any additional ports on the host.
