import {
  memo,
  startTransition,
  useDeferredValue,
  useEffect,
  useMemo,
  useState,
} from "react";
import { API_BASE_URL, fetchJSON } from "./api";

// ── Constants ───────────────────────────────────────────────────────────────

const SNAPSHOT_INTERVAL_MS = 15_000;
const MAX_PATTERNS = 100;
const MAX_ACTIVE_USERS = 15;

// ── Helpers ──────────────────────────────────────────────────────────────────

function formatRelativeTime(ts) {
  const s = Math.max(0, Math.floor((Date.now() - ts) / 1000));
  if (s < 5)   return "just now";
  if (s < 60)  return `${s}s ago`;
  const m = Math.floor(s / 60);
  if (m < 60)  return `${m}m ago`;
  return `${Math.floor(m / 60)}h ago`;
}

function formatPercent(v) {
  return `${(v * 100).toFixed(1)}%`;
}

function mergePatternFeed(current, next) {
  return [next, ...current.filter((p) => p.pattern_id !== next.pattern_id)].slice(0, MAX_PATTERNS);
}

function mergeActiveUserFeed(current, next) {
  return [next, ...current.filter((u) => u.user_id !== next.user_id)].slice(0, MAX_ACTIVE_USERS);
}

// ── Header ───────────────────────────────────────────────────────────────────

const Header = memo(function Header({ connectionStatus }) {
  return (
    <header className="header">
      <div className="header__brand">
        <div className="header__logo">BL</div>
        <span className="header__name">BehaviourLens</span>
        <span className="header__tag">v0.1</span>
      </div>

      <div className="header__right">
        <span className="header__url">{API_BASE_URL || "localhost:8080"}</span>
        <div className={`status status--${connectionStatus}`}>
          <span className="status__dot" />
          <span>{connectionStatus}</span>
        </div>
      </div>
    </header>
  );
});

// ── Metric card ──────────────────────────────────────────────────────────────

const MetricCard = memo(function MetricCard({ label, value, accent, sub }) {
  return (
    <div className={`metric metric--${accent}`}>
      <div className="metric__label">{label}</div>
      <div className="metric__value">{value}</div>
      <div className="metric__sub">{sub}</div>
    </div>
  );
});

// ── Spotlight (latest pattern) ────────────────────────────────────────────────

const Spotlight = memo(function Spotlight({ pattern }) {
  if (!pattern) return null;
  return (
    <div className={`spotlight spotlight--${pattern.type}`}>
      <div className="spotlight__bar" />
      <div className="spotlight__body">
        <div className="spotlight__label">Latest alert</div>
        <div className="spotlight__page">{pattern.page}</div>
        <div className="spotlight__expl">{pattern.explanation}</div>
        <div className="spotlight__meta">
          <span className="spotlight__user">{pattern.user_id}</span>
          <span className="spotlight__time">{formatRelativeTime(pattern.detected_at)}</span>
          <span className={`spotlight__sev spotlight__sev--${pattern.severity}`}>
            {pattern.severity}
          </span>
        </div>
      </div>
    </div>
  );
});

// ── Pattern row ───────────────────────────────────────────────────────────────

const PatternRow = memo(function PatternRow({ pattern, onResolve }) {
  return (
    <div className={`prow prow--${pattern.type}${pattern.resolved ? " prow--resolved" : ""}`}>
      <div className="prow__bar" />

      <div className="prow__body">
        <div className="prow__top">
          <span className="prow__type">{pattern.type}</span>
          <span className="prow__page">{pattern.page}</span>
        </div>
        <p className="prow__expl">{pattern.explanation}</p>
        <div className="prow__foot">
          <span className="prow__user">{pattern.user_id}</span>
          <span className="prow__time">{formatRelativeTime(pattern.detected_at)}</span>
        </div>
      </div>

      <div className="prow__side">
        <span className={`sev-dot sev-dot--${pattern.severity}`} title={pattern.severity} />
        {pattern.resolved ? (
          <span className="resolved-chip">resolved</span>
        ) : (
          <button
            type="button"
            className="resolve-btn"
            onClick={() => onResolve(pattern.pattern_id)}
          >
            resolve
          </button>
        )}
      </div>
    </div>
  );
});

// ── User item ─────────────────────────────────────────────────────────────────

const UserItem = memo(function UserItem({ user, isSelected, onSelect }) {
  return (
    <button
      type="button"
      className={`uitem${isSelected ? " uitem--selected" : ""}`}
      onClick={() => onSelect(user.user_id)}
    >
      <div className="uitem__left">
        <span className="uitem__id">{user.user_id}</span>
        <span className="uitem__page">{user.current_page}</span>
      </div>
      <span className="uitem__time">{formatRelativeTime(user.last_seen)}</span>
    </button>
  );
});

// ── Event row ─────────────────────────────────────────────────────────────────

const EventRow = memo(function EventRow({ event }) {
  const durStr =
    event.metadata?.duration_ms
      ? ` · ${Math.round(Number(event.metadata.duration_ms) / 1000)}s`
      : "";
  return (
    <div className="erow">
      <span className="erow__action">{event.action}</span>
      <span className="erow__page">{event.page}{durStr}</span>
      <span className="erow__time">{formatRelativeTime(event.timestamp)}</span>
    </div>
  );
});

// ── App ───────────────────────────────────────────────────────────────────────

export default function App() {
  const [stats, setStats]                   = useState(null);
  const [patterns, setPatterns]             = useState([]);
  const [activeUsers, setActiveUsers]       = useState([]);
  const [connectionStatus, setConnectionStatus] = useState("connecting");
  const [error, setError]                   = useState("");
  const [filterType, setFilterType]         = useState("all");
  const [filterSeverity, setFilterSeverity] = useState("all");
  const [searchTerm, setSearchTerm]         = useState("");
  const [selectedUserId, setSelectedUserId] = useState("");
  const [selectedUserEvents, setSelectedUserEvents] = useState([]);
  const [userEventsStatus, setUserEventsStatus]     = useState("idle");

  const deferredPatterns    = useDeferredValue(patterns);
  const deferredActiveUsers = useDeferredValue(activeUsers);
  const deferredSearch      = useDeferredValue(searchTerm);

  // ── Snapshot polling ────────────────────────────────────────────────────────
  useEffect(() => {
    let active = true;
    let timer;

    async function load() {
      if (document.visibilityState === "hidden") return;
      try {
        const [s, p, u] = await Promise.all([
          fetchJSON("/api/stats"),
          fetchJSON(`/api/patterns?limit=${MAX_PATTERNS}`),
          fetchJSON("/api/users/active?within=60"),
        ]);
        if (!active) return;
        startTransition(() => {
          setStats(s);
          setPatterns(p);
          setActiveUsers(u.slice(0, MAX_ACTIVE_USERS));
          setError("");
        });
      } catch {
        if (active) setError("Could not reach the backend.");
      }
    }

    load();
    timer = window.setInterval(load, SNAPSHOT_INTERVAL_MS);

    const onVisibility = () => { if (document.visibilityState === "visible") load(); };
    document.addEventListener("visibilitychange", onVisibility);

    return () => {
      active = false;
      window.clearInterval(timer);
      document.removeEventListener("visibilitychange", onVisibility);
    };
  }, []);

  // ── SSE stream ──────────────────────────────────────────────────────────────
  useEffect(() => {
    const src = new EventSource(`${API_BASE_URL}/api/stream`);

    src.onopen = () => {
      setConnectionStatus("live");
      setError("");
    };

    src.addEventListener("pattern", (e) => {
      const p = JSON.parse(e.data);
      startTransition(() => {
        setPatterns((cur) => mergePatternFeed(cur, p));
        setActiveUsers((cur) =>
          mergeActiveUserFeed(cur, {
            user_id:      p.user_id,
            current_page: p.page,
            last_seen:    p.detected_at,
          }),
        );
      });
    });

    src.addEventListener("stats", (e) => {
      startTransition(() => setStats(JSON.parse(e.data)));
    });

    src.onerror = () => {
      setConnectionStatus("reconnecting");
      setError("Live stream disconnected — retrying automatically.");
    };

    return () => src.close();
  }, []);

  // ── User drill-down ──────────────────────────────────────────────────────────
  useEffect(() => {
    if (!selectedUserId) {
      setSelectedUserEvents([]);
      setUserEventsStatus("idle");
      return;
    }
    let active = true;
    setUserEventsStatus("loading");

    fetchJSON(`/api/users/${encodeURIComponent(selectedUserId)}/events`)
      .then((evs) => {
        if (!active) return;
        setSelectedUserEvents(evs.slice().reverse());
        setUserEventsStatus("ready");
      })
      .catch(() => {
        if (!active) return;
        setSelectedUserEvents([]);
        setUserEventsStatus("error");
      });

    return () => { active = false; };
  }, [selectedUserId]);

  // Auto-select first active user
  useEffect(() => {
    if (!selectedUserId && activeUsers[0]?.user_id) {
      setSelectedUserId(activeUsers[0].user_id);
    }
  }, [activeUsers, selectedUserId]);

  // ── Resolve pattern ──────────────────────────────────────────────────────────
  async function handleResolve(patternId) {
    try {
      const resolved = await fetchJSON(
        `/api/patterns/${encodeURIComponent(patternId)}/resolve`,
        { method: "PATCH" },
      );
      startTransition(() => {
        setPatterns((cur) => cur.map((p) => (p.pattern_id === patternId ? resolved : p)));
      });
    } catch { /* best-effort */ }
  }

  // ── Filtered patterns ────────────────────────────────────────────────────────
  const visiblePatterns = useMemo(() => {
    const q = deferredSearch.trim().toLowerCase();
    return deferredPatterns.filter((p) => {
      if (filterType !== "all" && p.type !== filterType) return false;
      if (filterSeverity !== "all" && p.severity !== filterSeverity) return false;
      if (!q) return true;
      return (
        p.user_id.toLowerCase().includes(q) ||
        p.page.toLowerCase().includes(q) ||
        p.explanation.toLowerCase().includes(q)
      );
    });
  }, [deferredPatterns, deferredSearch, filterType, filterSeverity]);

  const latestPattern  = deferredPatterns[0] ?? null;
  const patternCount   = visiblePatterns.length;

  // ── Render ───────────────────────────────────────────────────────────────────
  return (
    <div className="app">
      <Header connectionStatus={connectionStatus} />

      {error && <div className="error-banner">{error}</div>}

      <main className="content">
        {/* ── Metrics ── */}
        <section className="metrics">
          <MetricCard
            label="Total Events"
            value={stats ? stats.total_events.toLocaleString() : "—"}
            accent="green"
            sub="Events processed since startup"
          />
          <MetricCard
            label="Active Users"
            value={stats ? stats.active_users : "—"}
            accent="amber"
            sub="Seen in last 60 seconds"
          />
          <MetricCard
            label="Patterns Detected"
            value={stats ? stats.patterns_detected.toLocaleString() : "—"}
            accent="red"
            sub="Friction signals emitted"
          />
          <MetricCard
            label="Abandonment Rate"
            value={stats ? formatPercent(stats.abandonment_rate) : "—"}
            accent="blue"
            sub="Abandonments / total users"
          />
        </section>

        {/* ── Workspace ── */}
        <div className="workspace">

          {/* ── Feed panel ── */}
          <div className="panel feed">
            <div className="panel__head">
              <span className="panel__title">Live pattern feed</span>
              <span className="panel__count">{patternCount}</span>
            </div>

            {/* Spotlight: always shows latest regardless of filters */}
            <Spotlight pattern={latestPattern} />

            {/* Filters */}
            <div className="feed__controls">
              <select
                className="ctrl-select"
                value={filterType}
                onChange={(e) => setFilterType(e.target.value)}
              >
                <option value="all">All types</option>
                <option value="hesitation">Hesitation</option>
                <option value="navigation-loop">Navigation loop</option>
                <option value="abandonment">Abandonment</option>
              </select>

              <select
                className="ctrl-select"
                value={filterSeverity}
                onChange={(e) => setFilterSeverity(e.target.value)}
              >
                <option value="all">All severities</option>
                <option value="high">High</option>
                <option value="medium">Medium</option>
                <option value="low">Low</option>
              </select>

              <input
                type="search"
                className="ctrl-input"
                placeholder="Search user, page, explanation…"
                value={searchTerm}
                onChange={(e) => setSearchTerm(e.target.value)}
              />
            </div>

            {/* Pattern list */}
            <div className="feed__list">
              {visiblePatterns.length > 0 ? (
                visiblePatterns.map((p) => (
                  <PatternRow key={p.pattern_id} pattern={p} onResolve={handleResolve} />
                ))
              ) : (
                <div className="empty">
                  <div className="empty__icon">◎</div>
                  <div className="empty__title">No patterns yet</div>
                  <div className="empty__sub">
                    Run the simulator or adjust filters to see live behavior signals.
                  </div>
                </div>
              )}
            </div>
          </div>

          {/* ── Sidebar ── */}
          <div className="sidebar">

            {/* Active users */}
            <div className="panel ulist">
              <div className="panel__head">
                <span className="panel__title">Active sessions</span>
                <span className="panel__count">{deferredActiveUsers.length}</span>
              </div>

              <div className="ulist__scroll">
                {deferredActiveUsers.length > 0 ? (
                  deferredActiveUsers.map((u) => (
                    <UserItem
                      key={u.user_id}
                      user={u}
                      isSelected={selectedUserId === u.user_id}
                      onSelect={setSelectedUserId}
                    />
                  ))
                ) : (
                  <div className="empty" style={{ padding: "28px 16px" }}>
                    <div className="empty__title">No active users</div>
                    <div className="empty__sub">Users appear here when traffic flows.</div>
                  </div>
                )}
              </div>
            </div>

            {/* Event window */}
            <div className="panel ewindow">
              <div className="panel__head">
                <span className="panel__title">Event window</span>
                {selectedUserId && (
                  <span className="panel__count">
                    {userEventsStatus === "loading" ? "…" : selectedUserEvents.length}
                  </span>
                )}
              </div>

              {selectedUserId && (
                <div className="ewindow__user">
                  <span className="ewindow__user-id">{selectedUserId}</span>
                  {userEventsStatus === "ready" && (
                    <span className="ewindow__count">
                      {selectedUserEvents.length} events in window
                    </span>
                  )}
                </div>
              )}

              <div className="ewindow__scroll">
                {!selectedUserId ? (
                  <div className="empty" style={{ padding: "28px 16px" }}>
                    <div className="empty__title">No user selected</div>
                    <div className="empty__sub">Click a session above to inspect events.</div>
                  </div>
                ) : userEventsStatus === "error" ? (
                  <div className="empty" style={{ padding: "28px 16px" }}>
                    <div className="empty__title">Failed to load</div>
                    <div className="empty__sub">Could not retrieve this user's events.</div>
                  </div>
                ) : selectedUserEvents.length > 0 ? (
                  selectedUserEvents.map((ev, i) => (
                    <EventRow key={`${ev.timestamp}-${ev.action}-${i}`} event={ev} />
                  ))
                ) : (
                  <div className="empty" style={{ padding: "28px 16px" }}>
                    <div className="empty__sub">Loading events…</div>
                  </div>
                )}
              </div>
            </div>

          </div>
        </div>
      </main>
    </div>
  );
}
