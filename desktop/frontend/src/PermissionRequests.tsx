import { useCallback, useEffect, useState } from "react";
import {
  api,
  type PermissionGrant,
  type PermissionRequest,
  type PermissionRequestsPayload,
} from "./api";

// Durations the operator can approve. 0 means "until I revoke it".
const DURATIONS: { minutes: number; label: string }[] = [
  { minutes: 5, label: "5 minutes" },
  { minutes: 15, label: "15 minutes" },
  { minutes: 60, label: "1 hour" },
  { minutes: 240, label: "4 hours" },
  { minutes: 0, label: "Until I revoke it" },
];

const POLL_MS = 4000;

function formatTime(iso?: string): string {
  if (!iso) return "";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 2000) return "";
  return date.toLocaleTimeString();
}

function grantExpiry(grant: PermissionGrant): string {
  if (!grant.expires_at) return "until revoked";
  const until = new Date(grant.expires_at);
  if (Number.isNaN(until.getTime())) return "until revoked";
  return `until ${until.toLocaleTimeString()}`;
}

/**
 * The operator's side of request_permission: what Neuro asked for, approvals
 * that are still in force, and a way to take them back. Approving a request
 * never widens the policy file; it adds a grant that expires or is revoked.
 */
export default function PermissionRequests({ onChanged }: { onChanged?: () => void }) {
  const [data, setData] = useState<PermissionRequestsPayload | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [durations, setDurations] = useState<Record<string, number>>({});

  const refresh = useCallback(async () => {
    try {
      setData(await api.permissionRequests());
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, []);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), POLL_MS);
    return () => window.clearInterval(timer);
  }, [refresh]);

  async function decide(request: PermissionRequest, approve: boolean) {
    setBusy(request.id);
    try {
      const minutes = durations[request.id] ?? (request.minutes > 0 ? request.minutes : 15);
      const result = await api.decidePermissionRequest(request.id, approve, minutes);
      setError(result.ok === false ? result.message ?? "The request could not be decided." : "");
      await refresh();
      onChanged?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy("");
    }
  }

  async function revoke(grant: PermissionGrant) {
    setBusy(`grant:${grant.scope}`);
    try {
      await api.revokePermissionGrant(grant.scope);
      await refresh();
      onChanged?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy("");
    }
  }

  const pending = data?.pending ?? [];
  const grants = data?.grants ?? [];
  const recent = (data?.recent ?? []).slice(0, 5);

  return (
    <section className="permission-requests" aria-label="Permission requests">
      <div className="panel-header">
        <div>
          <h2>Neuro's requests</h2>
          <p>
            Neuro can ask for a permission you have made requestable. An approval lasts for the time
            you choose, or until you revoke it. It never changes the policy file.
          </p>
        </div>
      </div>

      {error && <p className="scope-warning">{error}</p>}

      <h3>Waiting for you</h3>
      {pending.length === 0 && <p className="hint">Nothing is waiting.</p>}
      <ul className="request-list">
        {pending.map((request) => (
          <li key={request.id} className="request-item">
            <div>
              <strong>{request.scope}</strong> <small>requested {formatTime(request.requested_at)}</small>
              <p>{request.reason || "(no reason given)"}</p>
              {request.minutes > 0 && <small>Neuro suggests {request.minutes} minutes.</small>}
            </div>
            <div className="request-actions">
              <label>
                <span className="sr-only">Duration</span>
                <select
                  value={durations[request.id] ?? (request.minutes > 0 ? request.minutes : 15)}
                  onChange={(event) =>
                    setDurations((prev) => ({ ...prev, [request.id]: Number(event.target.value) }))
                  }
                >
                  {DURATIONS.map((option) => (
                    <option key={option.minutes} value={option.minutes}>
                      {option.label}
                    </option>
                  ))}
                </select>
              </label>
              <button
                className="primary"
                disabled={busy === request.id}
                onClick={() => void decide(request, true)}
              >
                Approve
              </button>
              <button
                className="secondary"
                disabled={busy === request.id}
                onClick={() => void decide(request, false)}
              >
                Deny
              </button>
            </div>
          </li>
        ))}
      </ul>

      <h3>Approved right now</h3>
      {grants.length === 0 && <p className="hint">No approvals are active.</p>}
      <ul className="request-list">
        {grants.map((grant) => (
          <li key={grant.scope} className="request-item">
            <div>
              <strong>{grant.scope}</strong> <small>{grantExpiry(grant)}</small>
              {grant.reason && <p>{grant.reason}</p>}
            </div>
            <button
              className="secondary"
              disabled={busy === `grant:${grant.scope}`}
              onClick={() => void revoke(grant)}
            >
              Revoke
            </button>
          </li>
        ))}
      </ul>

      {recent.length > 0 && (
        <>
          <h3>Recent decisions</h3>
          <ul className="request-recent">
            {recent.map((request) => (
              <li key={request.id}>
                <span className={request.status === "approved" ? "allowed" : "denied"}>{request.status}</span>{" "}
                {request.scope} <small>{formatTime(request.decided_at)}</small>
              </li>
            ))}
          </ul>
        </>
      )}

      <p className="hint">
        Neuro may ask for: {data && data.requestable.length > 0 ? data.requestable.join(", ") : "nothing yet. Turn on “Neuro may request” for a scope below."}
      </p>
    </section>
  );
}
