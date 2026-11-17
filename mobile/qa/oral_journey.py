#!/usr/bin/env python3
"""Exercise a test APK against the loopback Go API, Mailpit and oral_voice.py."""
import argparse
import base64
import json
import os
import re
import subprocess
import time
import urllib.parse
import urllib.request
import uuid
from pathlib import Path


class TestAPI:
    def __init__(self, api_port, mail_port):
        self.url = f"http://127.0.0.1:{api_port}"
        self.mail = f"http://127.0.0.1:{mail_port}"

    def call(self, path, method="GET", token="", payload=None):
        body = json.dumps(payload).encode() if payload is not None else None
        request = urllib.request.Request(self.url + path, data=body, method=method)
        if token:
            request.add_header("Authorization", "Bearer " + token)
        if body is not None:
            request.add_header("Content-Type", "application/json")
        with urllib.request.urlopen(request, timeout=20) as response:
            raw = response.read()
        return json.loads(raw) if raw else None

    def link(self, email):
        with urllib.request.urlopen(self.mail + "/api/v1/search?" + urllib.parse.urlencode({"query": "to:" + email})) as r:
            previous = {m["ID"] for m in json.load(r).get("messages", [])}
        self.call("/v1/auth/request", "POST", payload={"email": email, "client": "mobile"})
        for _ in range(50):
            with urllib.request.urlopen(self.mail + "/api/v1/search?" + urllib.parse.urlencode({"query": "to:" + email})) as r:
                messages = json.load(r).get("messages", [])
            for message in messages:
                if message["ID"] in previous:
                    continue
                with urllib.request.urlopen(self.mail + "/api/v1/message/" + message["ID"]) as r:
                    match = re.search(r"crpemaths://auth#token=([A-Za-z0-9_-]+)", json.load(r)["Text"])
                if match:
                    return match.group(0)
            time.sleep(.2)
        raise AssertionError("Missing test email")

    def member(self, email):
        link = self.link(email)
        return self.call("/v1/auth/verify", "POST", payload={"token": link.split("=")[-1], "client": "mobile"})["token"]

    def review(self, teacher, item):
        return self.call("/v1/admin/oral-sessions/" + item["id"] + "/review", "POST", teacher,
                         {"feedback": [{"criterion": c, "status": "validated", "comment": "Critère vérifié sur la copie de test."} for c in item["criteria"]]})


class Android:
    def __init__(self, serial):
        if not re.fullmatch(r"emulator-\d+", serial):
            raise ValueError("Choose a dedicated emulator")
        self.adb = [str(Path(os.environ["ANDROID_HOME"]) / "platform-tools" / "adb"), "-s", serial]

    def command(self, *args):
        return subprocess.check_output([*self.adb, *args], stderr=subprocess.DEVNULL, text=True)

    def shell(self, *args):
        return self.command("shell", *args)

    def nodes(self):
        result = self.shell("am", "instrument", "-w", "-r", "fr.prepamathscrpe.qa/.Tree")
        tree = re.search(r"INSTRUMENTATION_RESULT: tree=([A-Za-z0-9+/=]+)", result)
        return json.loads(base64.b64decode(tree[1])) if tree else []

    def find(self, label, timeout=24, scroll=False, starts=False):
        deadline = time.monotonic() + timeout
        direction, steps = 1, 0
        while time.monotonic() < deadline:
            nodes = self.nodes()
            for n in nodes:
                values = [n.get("text", ""), n.get("content-desc", "")]
                if any(v.casefold().startswith(label.casefold()) if starts else v.casefold() == label.casefold() for v in values):
                    return n
            if scroll:
                self.swipe(direction)
                steps += 1
                if steps % 5 == 0:
                    direction *= -1
            else:
                time.sleep(.25)
        raise AssertionError("Missing " + label + " | " + repr([n.get("text") for n in nodes if n.get("text")]))

    def swipe(self, direction=1):
        y1, y2 = (1800, 650) if direction == 1 else (650, 1800)
        self.shell("input", "swipe", "500", str(y1), "500", str(y2), "350")

    def tap(self, label, **options):
        node = self.find(label, **options)
        left, top, right, bottom = map(int, re.findall(r"\d+", node["bounds"]))
        self.shell("input", "tap", str((left+right)//2), str((top+bottom)//2))

    def record(self, label):
        self.tap(label, scroll=True)
        self.find("Arrêter l’enregistrement", scroll=True)
        time.sleep(1)
        self.tap("Arrêter l’enregistrement", scroll=True)
        self.tap("Transcrire l’enregistrement", scroll=True)
        self.find("Transcription ajoutée.", starts=True, scroll=True)

    def run(self, args):
        api = TestAPI(args.api_port, args.mail_port)
        app = "fr.prepamathscrpe.mobile"
        email = "oral-android-" + uuid.uuid4().hex[:12] + "@example.test"
        token, teacher = api.member(email), api.member(args.teacher)
        port = "tcp:" + str(args.api_port)
        try:
            # This program never enables host audio input. Launch the emulator with -no-audio.
            self.command("install", "-t", str(args.tree_apk))
            if args.apk:
                self.command("install", "-r", str(args.apk))
            self.command("reverse", port, port)
            self.shell("pm", "grant", app, "android.permission.RECORD_AUDIO")
            self.shell("input", "keyevent", "82")
            self.shell("am", "start", "-W", "-a", "android.intent.action.VIEW", "-d", api.link(email), "-p", app)
            self.tap("Activités")
            self.tap("Entraînement à l’oral", scroll=True)
            self.tap("Tirer un sujet")
            self.find("Critères de l’oral", scroll=True)
            active = api.call("/v1/oral-sessions", token=token)["items"][0]
            self.record("Enregistrer mon exposé")
            print("Verified native Android AAC talk recording and server transcription.", flush=True)
            self.tap("Écouter la question", scroll=True)
            self.record("Enregistrer ma réponse")
            # Fill remaining questions one by one in their own visible control.
            for index in range(1, len(active["questions"])):
                self.find("Question " + str(index+1), scroll=True)
                # Select the empty response field; preceding response already contains its transcript.
                self.tap("Texte de ma réponse", scroll=True)
                self.shell("input", "text", "Argument%sde%stest.")
                self.shell("input", "keyevent", "4")
                if any(n.get("text") == "Mes séances" for n in self.nodes()):
                    self.tap(active["title"] + " · En cours", scroll=True)
            self.tap("Envoyer mon oral", scroll=True)
            if not any(n.get("text", "").casefold() == "transmettre" for n in self.nodes()):
                self.tap("Envoyer mon oral", scroll=True)
            self.tap("Transmettre")
            self.find("En attente d’évaluation", scroll=True)
            submitted = api.call("/v1/oral-sessions", token=token)["items"][0]
            assert submitted["id"] == active["id"] and submitted["status"] == "pending_review"
            assert "Transcription de test" in submitted["transcript"] and "Transcription de test" in submitted["answers"][0]
            api.review(teacher, submitted)
            self.tap("Actualiser la séance", scroll=True)
            self.find("Évaluation enseignante", scroll=True)
            self.find(submitted["criteria"][0]+" · Validé", starts=True, scroll=True)
            self.tap("Toutes mes séances", scroll=True)
            self.find("Progression par critère", scroll=True)
            print("Verified Android AAC answer, MP3 question, typed answer, server submission and teacher criteria progress.")
        finally:
            api.call("/v1/me", "DELETE", token)
            api.call("/v1/auth/logout", "POST", teacher)
            if args.apk:
                self.command("uninstall", app)
            else:
                self.shell("am", "force-stop", app)
            self.command("reverse", "--remove", port)
            self.command("uninstall", "fr.prepamathscrpe.qa")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--api-port", type=int, required=True)
    parser.add_argument("--mail-port", type=int, required=True)
    parser.add_argument("--teacher", default="oral-qa-teacher@example.test")
    parser.add_argument("--serial", required=True)
    parser.add_argument("--apk", type=Path)
    parser.add_argument("--tree-apk", type=Path, required=True)
    Android((args := parser.parse_args()).serial).run(args)
