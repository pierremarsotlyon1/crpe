"use client";
import { useRouter } from "next/navigation";
import { type FormEvent, type ReactNode, useState } from "react";

export function ActionForm({
  children,
  submit,
  label,
  pendingLabel = "En cours…",
  secondary,
  refresh,
  reset,
  once,
}: {
  children?: ReactNode;
  submit: (form: FormData) => Promise<string | undefined> | Promise<void>;
  label: string;
  pendingLabel?: string;
  secondary?: boolean;
  refresh?: boolean;
  reset?: boolean;
  once?: boolean;
}) {
  const router = useRouter();
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const [done, setDone] = useState(false);
  const [failed, setFailed] = useState(false);
  async function handle(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending || (once && done)) return;
    const form = event.currentTarget;
    const data = new FormData(form);
    setPending(true);
    setMessage("");
    setFailed(false);
    try {
      const response = await submit(data);
      setMessage(response ?? "");
      setDone(true);
      if (reset) form.reset();
      if (refresh) router.refresh();
    } catch (error) {
      setFailed(true);
      setMessage(
        error instanceof Error && !(error instanceof TypeError)
          ? error.message
          : "Connexion au serveur impossible. Vérifiez votre connexion et réessayez.",
      );
    } finally {
      setPending(false);
    }
  }
  return (
    <form className="form" onSubmit={handle}>
      {children}
      <div>
        <button
          className={secondary ? "button secondary" : "button"}
          disabled={pending || (once && done)}
          type="submit"
        >
          {pending ? pendingLabel : label}
        </button>
      </div>
      <div
        aria-live="polite"
        className={
          message ? (failed ? "feedback error" : "feedback") : undefined
        }
      >
        {message}
      </div>
    </form>
  );
}

export function field(form: FormData, key: string) {
  return String(form.get(key) ?? "").trim();
}
