"use client";
import { useEffect, useRef, useState } from "react";
import { checked } from "../lib/model";
export function Recorder({
  sessionId,
  onText,
  maxMinutes,
  label,
}: {
  sessionId: string;
  onText: (text: string) => void;
  maxMinutes: number;
  label: string;
}) {
  const [state, setState] = useState<"idle" | "recording" | "processing">(
    "idle",
  );
  const [error, setError] = useState("");
  const recorder = useRef<MediaRecorder | null>(null);
  const stream = useRef<MediaStream | null>(null);
  const timeout = useRef<number | undefined>(undefined);
  const alive = useRef(true);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      if (recorder.current?.state === "recording") recorder.current.stop();
      stream.current?.getTracks().forEach((track) => {
        track.stop();
      });
      window.clearTimeout(timeout.current);
    };
  }, []);
  async function start() {
    setError("");
    if (
      !navigator.mediaDevices?.getUserMedia ||
      typeof MediaRecorder === "undefined"
    ) {
      setError(
        "Ce navigateur ne permet pas l’enregistrement. Vous pouvez saisir votre texte.",
      );
      return;
    }
    setState("processing");
    try {
      const input = await navigator.mediaDevices.getUserMedia({ audio: true });
      if (!alive.current) {
        input.getTracks().forEach((track) => {
          track.stop();
        });
        return;
      }
      stream.current = input;
      const type = [
        "audio/webm;codecs=opus",
        "audio/mp4",
        "audio/ogg;codecs=opus",
      ].find((value) => MediaRecorder.isTypeSupported(value));
      const recording = new MediaRecorder(input, {
        ...(type ? { mimeType: type } : {}),
        audioBitsPerSecond: 32000,
      });
      const chunks: BlobPart[] = [];
      recorder.current = recording;
      recording.ondataavailable = (event) => {
        if (event.data.size) chunks.push(event.data);
      };
      recording.onstop = async () => {
        input.getTracks().forEach((track) => {
          track.stop();
        });
        window.clearTimeout(timeout.current);
        if (!alive.current) return;
        setState("processing");
        try {
          const data = new FormData();
          const mime = recording.mimeType.split(";")[0] || "audio/webm";
          data.set(
            "audio",
            new Blob(chunks, { type: mime }),
            `oral.${mime === "audio/mp4" ? "mp4" : mime === "audio/ogg" ? "ogg" : "webm"}`,
          );
          const reply = await fetch(
            `/api/v1/oral-sessions/${sessionId}/transcribe`,
            { method: "POST", body: data },
          );
          checked(reply);
          const result = (await reply.json()) as { text?: string };
          if (!result.text) throw new Error("La transcription est vide.");
          if (alive.current) onText(result.text);
        } catch {
          if (alive.current)
            setError(
              "La transcription est indisponible. Réessayez ou saisissez votre intervention.",
            );
        } finally {
          if (alive.current) setState("idle");
        }
      };
      recording.start(1000);
      setState("recording");
      timeout.current = window.setTimeout(() => {
        if (recording.state === "recording") recording.stop();
      }, maxMinutes * 60000);
    } catch {
      stream.current?.getTracks().forEach((track) => {
        track.stop();
      });
      setState("idle");
      setError(
        "Accès au microphone impossible. Vérifiez son autorisation dans votre navigateur.",
      );
    }
  }
  return (
    <div className="audio-controls">
      <button
        className="button secondary"
        type="button"
        disabled={state === "processing"}
        onClick={() => {
          if (state === "recording") recorder.current?.stop();
          else void start();
        }}
      >
        {state === "recording"
          ? "Arrêter et transcrire"
          : state === "processing"
            ? "Traitement en cours…"
            : label}
      </button>
      {error && <p role="alert">{error}</p>}
    </div>
  );
}
