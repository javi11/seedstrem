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
    <div className="flex min-h-screen items-center justify-center bg-base-200">
      <div className="card w-96 bg-base-100 shadow-xl">
        <form className="card-body" onSubmit={submit}>
          <h1 className="card-title justify-center text-2xl">🌱 seedstrem</h1>
          <p className="text-center text-sm opacity-70">
            Enter the admin password (printed to the server log on first run).
          </p>
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
