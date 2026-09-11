# DocQuery AI

A production-ready document question-answering assistant built with retrieval-augmented generation (RAG). Upload a PDF, retrieve the most relevant passages, and ask grounded questions with source citations.

**Live demo:** [assistant-ui-sand.vercel.app](https://assistant-ui-sand.vercel.app/)

## Features

- PDF upload, parsing, and chunking
- Cohere embeddings with pgvector similarity search
- Source-grounded answers streamed through Server-Sent Events
- Per-session document isolation and usage limits
- Session leases with heartbeat and automatic cleanup
- Searchable document chunks with page and section metadata
- Responsive Next.js interface with source expansion and status feedback
- Protected debug endpoints that are disabled by default

## Architecture

```text
Next.js frontend (Vercel)
        |
        | HTTPS + credentialed CORS
        v
Go API (Render)
        |
        +--> Neon PostgreSQL + pgvector
        +--> Cohere embeddings and chat
```

| Layer | Technology | Deployment |
| --- | --- | --- |
| Frontend | Next.js, React, TypeScript | Vercel |
| API | Go, Gin | Render Docker service |
| Database | Neon PostgreSQL, pgvector | Neon |
| AI | Cohere embeddings and generation | Cohere API |

## Repository Layout

```text
cmd/server/                 Go API entrypoint
internal/                   API handlers, services, sessions, and database code
migrations/schema.sql       PostgreSQL and pgvector schema
Frontend/assistant-ui/       Next.js frontend
Dockerfile                  Production backend image
docker-compose.yml          Local PostgreSQL and backend stack
render.yaml                 Render deployment blueprint
```

## Local Development

### Backend and PostgreSQL

Requirements: Docker, Go 1.25+, and a Cohere API key.

1. Copy the environment template:

   ```bash
   cp .env.example .env
   ```

2. Set `COHERE_API_KEY` in `.env`.
3. Start PostgreSQL and the API:

   ```bash
   docker compose up --build
   ```

The API is available at `http://localhost:8080` and reports health at `/health`.

### Frontend

```bash
cd Frontend/assistant-ui
cp .env.example .env.local
npm install
npm run dev
```

Open `http://localhost:3000`.

For local development, set `NEXT_PUBLIC_API_URL=http://localhost:8080` in `.env.local` if the backend is running on the default Compose port.

## Deployment

This repository uses separate services:

- **Vercel:** import the repository and set the root directory to `Frontend/assistant-ui`.
- **Render:** deploy the backend from the repository root using `Dockerfile`.
- **Neon:** create a PostgreSQL database with the `vector` extension and apply `migrations/schema.sql` once.

### Render environment variables

```env
COHERE_API_KEY=your-cohere-api-key
DATABASE_URL=postgresql://...?...sslmode=require
ENV=production
ALLOWED_ORIGINS=https://your-vercel-domain.vercel.app
DEBUG_TOKEN=
```

### Vercel environment variables

```env
NEXT_PUBLIC_API_URL=https://your-render-service.onrender.com
BACKEND_URL=https://your-render-service.onrender.com
```

`ALLOWED_ORIGINS` must exactly match the deployed frontend origin. Never commit `.env` files or API keys.

## Validation

```bash
go test ./cmd/... ./internal/...
cd Frontend/assistant-ui && npm run build
```

## Notes

- Uploaded files are stored on the backend filesystem. Ephemeral Render storage means uploads can disappear after a restart or redeploy; the database remains persistent in Neon.
- The public demo has per-session and shared AI-provider limits.
- For production use, rotate any credential that has been exposed and store secrets only in Render/Vercel environment settings.

## License

Released under the [MIT License](LICENSE).
