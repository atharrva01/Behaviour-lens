# Deployment Guide

## Local full stack

Use Docker Compose from the repo root:

```powershell
docker compose up --build
```

Services:

- frontend on `http://localhost:3000`
- backend on `http://localhost:8080`

## Frontend deployment

Recommended options:

- Vercel for static hosting
- Netlify for static hosting
- Docker + nginx when you want same-origin proxying

If deploying the frontend separately, set:

```env
VITE_API_BASE_URL=https://your-backend-domain.example
```

## Backend deployment

Recommended options:

- Render
- Fly.io
- Railway
- Any small VM or container platform

The backend is stateless except for in-memory runtime state, so it works best as an MVP demo service.

## Important MVP note

Because the backend stores state in memory only:

- restarts clear users, events, and patterns
- horizontal scaling would require sticky routing or shared state later

## Demo checklist

- Start backend
- Start simulator
- Open frontend
- Show live pattern cards appearing
- Click an active user to inspect session events
- Explain the detection pipeline using the architecture docs
