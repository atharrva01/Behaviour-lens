# BehaviourLens

BehaviourLens is a real-time user behavior observability platform built for product and systems interviews. It detects user hesitation, navigation loops, and abandonment while sessions are still active, then explains those patterns in plain English through a live dashboard.

## Stack

- Backend: Go, `net/http`, goroutines, channels, in-memory state
- Frontend: React + Vite
- Real-time transport: Server-Sent Events (SSE)
- Deployment: Docker Compose for local full-stack runs

## What It Does

- Ingests live behavioral events
- Maintains per-user sliding-window state
- Detects rule-based friction patterns
- Generates deterministic explanations
- Streams live updates to a dashboard
- Lets you inspect active users and their recent event windows

## Run Locally

### Backend

```powershell
cd "Behaviour lens/Backend"
go run ./cmd/server
```

### Simulator

```powershell
cd "Behaviour lens/Backend"
go run ./cmd/simulator -users 10 -rate 400 -url http://localhost:8080
```

### Frontend

```powershell
cd "Behaviour lens/Frontend"
npm install
npm run dev
```

Open:

- Frontend: `http://localhost:5173`
- Backend: `http://localhost:8080`

## Run Full Stack With Docker

```powershell
cd "Behaviour lens"
docker compose up --build
```

Open:

- App: `http://localhost:3000`
- Backend health: `http://localhost:8080/health`

## Project Documents

- [Backend README](/C:/Users/athar/OneDrive/Desktop/Behaviour%20lens/Backend/README.md)
- [Frontend README](/C:/Users/athar/OneDrive/Desktop/Behaviour%20lens/Frontend/README.md)
- [Architecture and diagrams](/C:/Users/athar/OneDrive/Desktop/Behaviour%20lens/docs/ARCHITECTURE.md)
- [Schemas and API notes](/C:/Users/athar/OneDrive/Desktop/Behaviour%20lens/docs/SCHEMAS.md)
- [Deployment guide](/C:/Users/athar/OneDrive/Desktop/Behaviour%20lens/docs/DEPLOYMENT.md)

## Interview Framing

BehaviourLens demonstrates:

- event-driven backend design
- bounded in-memory stream processing
- rule-based explainable detection
- full-stack real-time UI integration
- deployment awareness and product thinking
