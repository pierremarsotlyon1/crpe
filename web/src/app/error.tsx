"use client";
export default function ErrorPage({ reset }: { reset: () => void }) {
  return (
    <main className="auth-panel" id="main-content">
      <h1>Service indisponible</h1>
      <p>
        Impossible de charger les données. Vérifiez votre connexion puis
        réessayez.
      </p>
      <button type="button" className="button" onClick={reset}>
        Réessayer
      </button>
    </main>
  );
}
