import { useEffect, useState } from "react";

// A slim, persistent banner shown whenever the browser reports it is offline.
// Polling hooks handle their own retry; this just tells the user why the screen
// has stopped updating.
export function OfflineBanner() {
  const [offline, setOffline] = useState(
    typeof navigator !== "undefined" ? !navigator.onLine : false,
  );

  useEffect(() => {
    const on = () => setOffline(false);
    const off = () => setOffline(true);
    window.addEventListener("online", on);
    window.addEventListener("offline", off);
    return () => {
      window.removeEventListener("online", on);
      window.removeEventListener("offline", off);
    };
  }, []);

  if (!offline) return null;

  return (
    <div className="alert alert-error rounded-none border-x-0 border-t-0 py-2">
      <span className="inline-block h-2 w-2 rounded-full bg-current" aria-hidden />
      <span className="text-sm">You&rsquo;re offline — reconnecting automatically…</span>
    </div>
  );
}
