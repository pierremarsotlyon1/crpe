export default function HomePage() {
  return (
    <div className="page">
      <header className="brand">
        <span className="brand-mark" aria-hidden="true">
          m.
        </span>
        <span>Prépa Maths CRPE</span>
      </header>

      <main id="main-content">
        <p className="eyebrow">
          <span aria-hidden="true" /> En préparation
        </p>
        <h1>
          Les maths du CRPE,
          <br />
          <em>pas à pas.</em>
        </h1>
        <p className="introduction">
          Un nouvel espace dédié à la préparation des mathématiques du CRPE est
          en cours de création.
        </p>
        <div className="notice">
          <p>La prépa prend forme.</p>
          <p>Les informations d’ouverture seront publiées ici.</p>
        </div>
      </main>

      <footer>
        Mathématiques · Concours de recrutement de professeurs des écoles
      </footer>
    </div>
  );
}
