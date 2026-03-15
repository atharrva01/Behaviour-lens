import {
  memo,
  startTransition,
  useDeferredValue,
  useEffect,
  useMemo,
  useState,
} from "react";
import { API_BASE_URL, fetchJSON } from "./api";

const patternTone = {
  hesitation: "tone-amber",
  "navigation-loop": "tone-blue",
  abandonment: "tone-red",
};

const SNAPSHOT_INTERVAL_MS = 15000;
const MAX_PATTERNS = 12;
const MAX_ACTIVE_USERS = 10;

function formatRelativeTime(timestamp) {
  const deltaSeconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000));

  if (deltaSeconds < 5) {
    return "just now";
  }
  if (deltaSeconds < 60) {
    return `${deltaSeconds}s ago`;
  }

  const minutes = Math.floor(deltaSeconds / 60);
  if (minutes < 60) {
    return `${minutes}m ago`;
  }

  const hours = Math.floor(minutes / 60);
  return `${hours}h ago`;
}

function formatPercent(value) {
  return `${(value * 100).toFixed(1)}%`;
}

function mergePatternFeed(current, nextPattern) {
  return [nextPattern, ...current.filter((pattern) => pattern.pattern_id !== nextPattern.pattern_id)].slice(
    0,
    MAX_PATTERNS,
  );
}

function mergeActiveUserFeed(current, nextUser) {
  return [nextUser, ...current.filter((user) => user.user_id !== nextUser.user_id)].slice(
    0,
    MAX_ACTIVE_USERS,
  );
}

function formatEventLabel(event) {
  if (!event) {
    return "";
  }

  const parts = [event.action, event.page];
  if (event.metadata?.duration_ms) {
    parts.push(`${Math.round(Number(event.metadata.duration_ms) / 1000)}s`);
  }
  return parts.join(" - ");
}

const MetricCard = memo(function MetricCard({ label, value, accent, helper }) {
  return (
    <article className={`metric-card ${accent}`}>
      <span className="eyebrow">{label}</span>
      <strong>{value}</strong>
      <p>{helper}</p>
    </article>
  );
});

const PatternCard = memo(function PatternCard({ pattern }) {
  return (
    <article className={`pattern-card ${patternTone[pattern.type] || "tone-slate"}`}>
      <div className="pattern-card__header">
        <span className="pill">{pattern.type}</span>
        <span className="pattern-card__severity">{pattern.severity}</span>
      </div>
      <h3>{pattern.page}</h3>
      <p>{pattern.explanation}</p>
      <div className="pattern-card__meta">
        <span>{pattern.user_id}</span>
        <span>{formatRelativeTime(pattern.detected_at)}</span>
      </div>
    </article>
  );
});

const UserRow = memo(function UserRow({ user, isSelected, onSelect }) {
  return (
    <button
      type="button"
      className={`user-row ${isSelected ? "user-row--selected" : ""}`}
      onClick={() => onSelect(user.user_id)}
    >
      <div>
        <span className="user-row__label">{user.user_id}</span>
        <strong>{user.current_page}</strong>
      </div>
      <span className="user-row__time">{formatRelativeTime(user.last_seen)}</span>
    </button>
  );
});

export default function App() {
  const [stats, setStats] = useState(null);
  const [patterns, setPatterns] = useState([]);
  const [activeUsers, setActiveUsers] = useState([]);
  const [connectionStatus, setConnectionStatus] = useState("connecting");
  const [error, setError] = useState("");
  const [filterType, setFilterType] = useState("all");
  const [searchTerm, setSearchTerm] = useState("");
  const [selectedUserId, setSelectedUserId] = useState("");
  const [selectedUserEvents, setSelectedUserEvents] = useState([]);
  const [userEventsStatus, setUserEventsStatus] = useState("idle");
  const deferredPatterns = useDeferredValue(patterns);
  const deferredActiveUsers = useDeferredValue(activeUsers);
  const deferredSearchTerm = useDeferredValue(searchTerm);

  useEffect(() => {
    let isActive = true;
    let intervalId;

    async function loadSnapshot() {
      if (document.visibilityState === "hidden") {
        return;
      }

      try {
        const [statsData, patternsData, usersData] = await Promise.all([
          fetchJSON("/api/stats"),
          fetchJSON(`/api/patterns?limit=${MAX_PATTERNS}`),
          fetchJSON("/api/users/active?within=60"),
        ]);

        if (!isActive) {
          return;
        }

        startTransition(() => {
          setStats(statsData);
          setPatterns(patternsData);
          setActiveUsers(usersData.slice(0, MAX_ACTIVE_USERS));
          setError("");
        });
      } catch (snapshotError) {
        if (isActive) {
          setError("Dashboard snapshot could not be loaded.");
        }
      }
    }

    loadSnapshot();
    intervalId = window.setInterval(loadSnapshot, SNAPSHOT_INTERVAL_MS);

    function handleVisibilityChange() {
      if (document.visibilityState === "visible") {
        loadSnapshot();
      }
    }

    document.addEventListener("visibilitychange", handleVisibilityChange);

    return () => {
      isActive = false;
      window.clearInterval(intervalId);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, []);

  useEffect(() => {
    const source = new EventSource(`${API_BASE_URL}/api/stream`);

    source.onopen = () => {
      setConnectionStatus("live");
      setError("");
    };

    source.addEventListener("pattern", (event) => {
      const nextPattern = JSON.parse(event.data);
      startTransition(() => {
        setPatterns((current) => mergePatternFeed(current, nextPattern));
        setActiveUsers((current) =>
          mergeActiveUserFeed(current, {
            user_id: nextPattern.user_id,
            current_page: nextPattern.page,
            last_seen: nextPattern.detected_at,
          }),
        );
      });
    });

    source.addEventListener("stats", (event) => {
      startTransition(() => {
        setStats(JSON.parse(event.data));
      });
    });

    source.onerror = () => {
      setConnectionStatus("reconnecting");
      setError("Live stream disconnected. Retrying automatically.");
    };

    return () => {
      source.close();
    };
  }, []);

  useEffect(() => {
    if (!selectedUserId) {
      setSelectedUserEvents([]);
      setUserEventsStatus("idle");
      return;
    }

    let isActive = true;
    setUserEventsStatus("loading");

    fetchJSON(`/api/users/${encodeURIComponent(selectedUserId)}/events`)
      .then((events) => {
        if (!isActive) {
          return;
        }
        setSelectedUserEvents(events.slice().reverse());
        setUserEventsStatus("ready");
      })
      .catch(() => {
        if (!isActive) {
          return;
        }
        setSelectedUserEvents([]);
        setUserEventsStatus("error");
      });

    return () => {
      isActive = false;
    };
  }, [selectedUserId]);

  useEffect(() => {
    if (!selectedUserId && activeUsers[0]?.user_id) {
      setSelectedUserId(activeUsers[0].user_id);
    }
  }, [activeUsers, selectedUserId]);

  const latestPattern = deferredPatterns[0];
  const visiblePatterns = useMemo(() => {
    const query = deferredSearchTerm.trim().toLowerCase();

    return deferredPatterns.filter((pattern) => {
      const matchesType = filterType === "all" || pattern.type === filterType;
      if (!matchesType) {
        return false;
      }
      if (!query) {
        return true;
      }

      return (
        pattern.user_id.toLowerCase().includes(query) ||
        pattern.page.toLowerCase().includes(query) ||
        pattern.explanation.toLowerCase().includes(query)
      );
    });
  }, [deferredPatterns, deferredSearchTerm, filterType]);

  return (
    <main className="page-shell">
      <section className="hero">
        <div className="hero__copy">
          <span className="eyebrow">Behavior observability</span>
          <h1>See user friction as it forms, not after it causes churn.</h1>
          <p>
            BehaviourLens watches live behavioral signals like hesitation, loops,
            and abandonment so product teams can react while sessions are still active.
          </p>
        </div>

        <div className="hero__status">
          <div className={`status-badge status-badge--${connectionStatus}`}>
            <span className="status-dot" />
            <span>{connectionStatus}</span>
          </div>
          <p>
            Backend stream:
            <code>{API_BASE_URL}</code>
          </p>
          {error ? <p className="hero__error">{error}</p> : null}
        </div>
      </section>

      <section className="metrics-grid">
        <MetricCard
          label="Total Events"
          value={stats ? stats.total_events.toLocaleString() : "--"}
          accent="accent-mint"
          helper="Cumulative events processed since startup."
        />
        <MetricCard
          label="Active Users"
          value={stats ? stats.active_users : "--"}
          accent="accent-sun"
          helper="Users seen in the last 60 seconds."
        />
        <MetricCard
          label="Patterns Detected"
          value={stats ? stats.patterns_detected.toLocaleString() : "--"}
          accent="accent-coral"
          helper="Rule-based friction signals emitted by the engine."
        />
        <MetricCard
          label="Abandonment Rate"
          value={stats ? formatPercent(stats.abandonment_rate) : "--"}
          accent="accent-sky"
          helper="Abandonment patterns divided by total users observed."
        />
      </section>

      <section className="panel panel--controls">
        <div className="controls">
          <label className="control">
            <span className="eyebrow">Pattern type</span>
            <select value={filterType} onChange={(event) => setFilterType(event.target.value)}>
              <option value="all">All patterns</option>
              <option value="hesitation">Hesitation</option>
              <option value="navigation-loop">Navigation loop</option>
              <option value="abandonment">Abandonment</option>
            </select>
          </label>

          <label className="control control--search">
            <span className="eyebrow">Search</span>
            <input
              type="search"
              placeholder="Filter by user, page, or explanation"
              value={searchTerm}
              onChange={(event) => setSearchTerm(event.target.value)}
            />
          </label>
        </div>
      </section>

      <section className="content-grid">
        <div className="panel panel--spotlight">
          <div className="panel__header">
            <span className="eyebrow">Latest alert</span>
            <h2>Most recent pattern</h2>
          </div>
          {latestPattern ? (
            <PatternCard pattern={latestPattern} />
          ) : (
            <div className="empty-state">
              <h3>No patterns yet</h3>
              <p>Start the simulator to watch live behavior signals appear here.</p>
            </div>
          )}
        </div>

        <div className="panel">
          <div className="panel__header">
            <span className="eyebrow">Session radar</span>
            <h2>Active users</h2>
          </div>
          <div className="user-list">
            {deferredActiveUsers.length > 0 ? (
              deferredActiveUsers.map((user) => (
                <UserRow
                  key={user.user_id}
                  user={user}
                  isSelected={selectedUserId === user.user_id}
                  onSelect={setSelectedUserId}
                />
              ))
            ) : (
              <p className="empty-copy">No active users in the last minute.</p>
            )}
          </div>
        </div>
      </section>

      <section className="panel panel--detail">
        <div className="panel__header">
          <span className="eyebrow">Drill-down</span>
          <h2>User event window</h2>
        </div>

        {selectedUserId ? (
          <div className="detail-panel">
            <div className="detail-panel__summary">
              <strong>{selectedUserId}</strong>
              <span>
                {userEventsStatus === "loading"
                  ? "Loading session events..."
                  : `${selectedUserEvents.length} events in current window`}
              </span>
            </div>

            <div className="event-list">
              {userEventsStatus === "error" ? (
                <p className="empty-copy">Could not load this user&apos;s event history.</p>
              ) : selectedUserEvents.length > 0 ? (
                selectedUserEvents.map((event, index) => (
                  <div
                    key={`${event.timestamp}-${event.action}-${index}`}
                    className="event-row"
                  >
                    <div>
                      <strong>{formatEventLabel(event)}</strong>
                      <span>{event.user_id}</span>
                    </div>
                    <time>{formatRelativeTime(event.timestamp)}</time>
                  </div>
                ))
              ) : (
                <p className="empty-copy">Select an active user to inspect their live session.</p>
              )}
            </div>
          </div>
        ) : (
          <p className="empty-copy">Active users will appear here once traffic is flowing.</p>
        )}
      </section>

      <section className="panel panel--feed">
        <div className="panel__header">
          <span className="eyebrow">Live feed</span>
          <h2>Detected patterns</h2>
        </div>

        <div className="pattern-grid">
          {visiblePatterns.length > 0 ? (
            visiblePatterns.map((pattern) => (
              <PatternCard key={pattern.pattern_id} pattern={pattern} />
            ))
          ) : (
            <div className="empty-state">
              <h3>No matching patterns</h3>
              <p>Adjust the filter or search input to widen the live feed.</p>
            </div>
          )}
        </div>
      </section>
    </main>
  );
}
