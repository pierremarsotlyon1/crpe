#!/usr/bin/env python3
"""Test-only loopback speech fixture. Validates real MP4 uploads and returns a synthetic MP3."""
import argparse
import json
import subprocess
from email import policy
from email.parser import BytesParser
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class SpeechFixture(BaseHTTPRequestHandler):
    def log_message(self, *_):
        pass

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        if not 0 < length < 21 * 1024 * 1024:
            self.send_error(413)
            return
        data = self.rfile.read(length)
        if self.path == "/transcribe":
            body = BytesParser(policy=policy.default).parsebytes(
                ("Content-Type: " + self.headers["Content-Type"] + "\r\n\r\n").encode() + data
            )
            files = [p.get_payload(decode=True) for p in body.iter_parts() if p.get_filename()]
            if len(files) != 1 or files[0][4:8] != b"ftyp":
                self.send_error(415)
                return
            # Validate the AAC stream, including frames, without retaining student audio.
            check = subprocess.run(
                ["ffmpeg", "-loglevel", "error", "-i", "pipe:0", "-map", "0:a:0", "-f", "null", "-"],
                input=files[0], capture_output=True, timeout=30
            )
            if check.returncode:
                self.send_error(422)
                return
            payload = json.dumps({"text": "Transcription de test de la séance."}).encode()
            content_type = "application/json"
            print("Validated AAC recording", len(files[0]), flush=True)
        elif self.path == "/speech" and json.loads(data).get("input"):
            payload = self.server.tone
            content_type = "audio/mpeg"
            print("Returned synthetic question audio", flush=True)
        else:
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--port", required=True, type=int)
    args = parser.parse_args()
    fixture = ThreadingHTTPServer(("127.0.0.1", args.port), SpeechFixture)
    fixture.tone = subprocess.check_output(
        ["ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.3",
         "-ac", "1", "-f", "mp3", "-"]
    )
    fixture.serve_forever()
