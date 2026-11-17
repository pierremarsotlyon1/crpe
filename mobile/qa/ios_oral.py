#!/usr/bin/env python3
"""Drive the isolated iOS UI journey with deliberate synthetic audio, Mailpit and teacher assessment."""
import argparse
import json
import re
import subprocess
import threading
import time
import urllib.parse
import urllib.request
import uuid
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from oral_journey import TestAPI

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--api-port", type=int, required=True)
parser.add_argument("--mail-port", type=int, required=True)
parser.add_argument("--device", required=True)
parser.add_argument("--teacher", default="oral-qa-teacher@example.test")
args = parser.parse_args()
api = TestAPI(args.api_port, args.mail_port)
run = uuid.uuid4().hex[:10]
email = "oral-ios-" + run + "@example.test"
bundle = "fr.prepamathscrpe.oral.qa" + run
student = api.member(email)
teacher = api.member(args.teacher)
owner = api.call("/v1/me", token=student)["id"]
finished = threading.Event()
mail_search = api.mail + "/api/v1/search?" + urllib.parse.urlencode({"query": "to:" + email})


def mails():
    with urllib.request.urlopen(mail_search, timeout=5) as stream:
        return [m["ID"] for m in json.load(stream)["messages"]]


before = set(mails())


def coordinate():
    deadline = time.monotonic() + 180
    opened = injected = graded = False
    while not finished.wait(.4) and time.monotonic() < deadline:
        if not opened:
            for message in mails():
                if message in before:
                    continue
                with urllib.request.urlopen(api.mail + "/api/v1/message/" + message) as s:
                    link = re.search(r"crpemaths://auth#token=[A-Za-z0-9_-]+", json.load(s)["Text"])
                if link:
                    subprocess.run(["xcrun", "simctl", "openurl", args.device, link[0]], check=True, capture_output=True)
                    opened = True
                    break
        if not opened:
            continue
        items = api.call("/v1/oral-sessions", token=student)["items"]
        if not items:
            continue
        session = items[0]
        if not injected:
            sandbox = Path(subprocess.check_output(["xcrun", "simctl", "get_app_container", args.device, bundle, "data"], text=True).strip())
            folder = sandbox / "tmp" / owner
            folder.mkdir(exist_ok=True)
            for slot in [-1, 0]:
                target = folder / f"oral-{str(uuid.UUID(session['id'])).upper()}-{slot}.m4a"
                subprocess.run(["ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=380:duration=0.6", "-c:a", "aac", "-ar", "24000", "-ac", "1", str(target)], check=True)
            injected = True
            print("Placed two synthetic AAC clips in the isolated test app container.", flush=True)
        if session["status"] == "pending_review" and not graded:
            assert session["transcript"].startswith("Transcription de test") and session["answers"][0].startswith("Transcription de test")
            api.review(teacher, session)
            graded = True
            print("Verified native iOS talk and answer transcripts, submitted teacher criteria.", flush=True)
        if graded:
            return True
    return False


root = Path(__file__).resolve().parents[1] / "ios"
try:
    with ThreadPoolExecutor() as pool:
        review = pool.submit(coordinate)
        result = subprocess.run(["xcodebuild", "test", "-project", str(root / "PrepaCRPE.xcodeproj"), "-scheme", "PrepaCRPEJourney",
            "-destination", "platform=iOS Simulator,id=" + args.device, "-parallel-testing-enabled", "NO",
            "-only-testing:NativeJourneyTests/OralJourney", "-derivedDataPath", "/private/tmp/crpe-oral-journey-derived",
            "CRPE_APP_ID=" + bundle, "CRPE_TEST_EMAIL=" + email, "CRPE_API_URL=http://127.0.0.1:" + str(args.api_port)])
        finished.set()
        if result.returncode or not review.result():
            raise SystemExit("iOS oral journey failed")
        print("Verified iOS oral UI, real AAC multipart, MP3 playback, editable responses and teacher criteria.")
finally:
    finished.set()
    api.call("/v1/me", "DELETE", student)
    api.call("/v1/auth/logout", "POST", teacher)
    subprocess.run(["xcrun", "simctl", "uninstall", args.device, bundle], capture_output=True)
