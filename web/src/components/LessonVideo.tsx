"use client";
import { useState } from "react";

function embedUrl(link: URL): string | undefined {
  if (
    [
      "youtube.com",
      "www.youtube.com",
      "m.youtube.com",
      "youtu.be",
      "www.youtube-nocookie.com",
    ].includes(link.hostname)
  ) {
    const id =
      link.hostname === "youtu.be"
        ? link.pathname.slice(1)
        : link.searchParams.get("v") ||
          link.pathname.match(/^\/(?:embed|shorts)\/([\w-]+)/)?.[1];
    return id && /^[\w-]{11}$/.test(id)
      ? `https://www.youtube-nocookie.com/embed/${id}`
      : undefined;
  }
  if (
    ["vimeo.com", "www.vimeo.com", "player.vimeo.com"].includes(link.hostname)
  ) {
    const match = link.pathname.match(
      /^\/(?:video\/)?(\d+)(?:\/([a-zA-Z0-9]+))?\/?$/,
    );
    if (!match) return;
    const player = new URL(`https://player.vimeo.com/video/${match[1]}`);
    player.searchParams.set("dnt", "1");
    const privateHash = match[2] || link.searchParams.get("h");
    if (privateHash) player.searchParams.set("h", privateHash);
    return player.href;
  }
}
export function LessonVideo({
  source,
  title,
}: {
  source: string;
  title: string;
}) {
  const [opened, setOpened] = useState(false);
  if (!URL.canParse(source)) return null;
  const link = new URL(source);
  if (link.protocol !== "https:") return null;
  const embed = embedUrl(link);
  return (
    <section className="lesson-video">
      <h3>Vidéo du cours</h3>
      {embed &&
        (opened ? (
          <iframe
            src={embed}
            title={`Vidéo · ${title}`}
            allow="fullscreen; encrypted-media; picture-in-picture"
            referrerPolicy="strict-origin-when-cross-origin"
            allowFullScreen
          />
        ) : (
          <button
            className="button"
            onClick={() => setOpened(true)}
            type="button"
          >
            Lire la vidéo sur{" "}
            {link.hostname.includes("vimeo") ? "Vimeo" : "YouTube"}
          </button>
        ))}
      <a className="text-link" href={source} target="_blank" rel="noreferrer">
        Ouvrir la vidéo dans un nouvel onglet ↗
      </a>
    </section>
  );
}
