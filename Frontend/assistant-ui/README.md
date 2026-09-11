 # DocQuery AI Frontend

 This directory contains the Next.js frontend for [DocQuery AI](../../README.md), a session-based document RAG assistant.

 ## Development

 ```bash
 npm install
 npm run dev
 ```

 Open [http://localhost:3000](http://localhost:3000) in a browser.

 Set the backend URL in `.env.local`:

 ```env
 NEXT_PUBLIC_API_URL=http://localhost:8080
 ```

 ## Production Build

 ```bash
 npm run build
 npm run start
 ```

 ## Deployment

 Deploy this directory on Vercel with `Frontend/assistant-ui` as the project root. Configure `NEXT_PUBLIC_API_URL` with the deployed Render API URL.

 See the [main project README](../../README.md) for the full architecture, backend setup, and deployment instructions.
