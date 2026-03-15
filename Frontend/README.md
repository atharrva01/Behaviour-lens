# BehaviourLens Frontend

React dashboard for the BehaviourLens backend.

## Features

- Live stats from `GET /api/stats`
- Live pattern feed from `GET /api/patterns`
- Active-user panel from `GET /api/users/active`
- Real-time updates from `GET /api/stream`

## Run locally

```powershell
cd "Behaviour lens/Frontend"
npm install
npm run dev
```

The app runs on `http://localhost:5173` by default.
In local development, Vite proxies `/api`, `/events`, and `/health` to `http://localhost:8080`.

## Backend URL

By default, the app uses same-origin requests.
That means:

- local dev works through the Vite proxy
- deployed setups can serve frontend and backend behind one domain

To change it, create a `.env` file in this folder:

```env
VITE_API_BASE_URL=http://localhost:8080
```

## Build

```powershell
npm run build
```
