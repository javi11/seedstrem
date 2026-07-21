import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../api";

export function Login() {
  const navigate = useNavigate();
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api.login(password);
      navigate("/");
    } catch {
      setError("Wrong password");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-base-200 p-4">
      <div className="surface w-full max-w-sm p-8">
        <div className="flex flex-col items-center gap-3">
          <span className="grid h-14 w-14 place-items-center rounded-box bg-gradient-to-br from-primary to-accent text-3xl shadow-lg">
            🌱
          </span>
          <h1 className="text-2xl font-bold tracking-brand">seedstrem</h1>
          <p className="text-center text-sm opacity-60">
            Enter the admin password (printed to the server log on first run).
          </p>
        </div>
        <form className="mt-6 flex flex-col gap-3" onSubmit={submit}>
          <input
            type="password"
            className="input input-bordered w-full"
            placeholder="Admin password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoFocus
          />
          {error && <div className="alert alert-error py-2 text-sm">{error}</div>}
          <button className="btn btn-primary" disabled={busy || !password}>
            {busy ? <span className="loading loading-spinner loading-sm" /> : "Log in"}
          </button>
        </form>
      </div>
    </div>
  );
}
