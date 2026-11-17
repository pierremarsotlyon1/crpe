#!/usr/bin/env python3
"""Run the native iOS journey against an isolated API + Mailpit with a validated exercise."""
import argparse
import concurrent.futures
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import threading
import time
import urllib.parse
import urllib.request
import uuid
import sys
sys.dont_write_bytecode=True
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from qa_relay import NativeRelay

parser = argparse.ArgumentParser()
parser.add_argument("--api", default="http://127.0.0.1:8080")
parser.add_argument("--mailpit", default="http://127.0.0.1:8025")
parser.add_argument("--device", required=True, help="Booted iOS simulator UDID")
parser.add_argument("--photo-fixture", action="store_true", help="Use only a dedicated empty simulator named CRPE Native Photo QA with the one synthetic fixture imported.")
parser.add_argument("--mock-subject", type=uuid.UUID, help="Validated test subject with numeric answer 7 and one written exercise.")
args = parser.parse_args()
for endpoint in (args.api, args.mailpit):
    if urllib.parse.urlparse(endpoint).hostname not in ("localhost", "127.0.0.1", "::1"):
        parser.error("Use isolated local test services.")
if args.photo_fixture:
    devices=json.loads(subprocess.check_output(["xcrun","simctl","list","devices","-j"]))["devices"]
    if not any(d["udid"]==args.device and d["name"]=="CRPE Native Photo QA" for group in devices.values() for d in group):
        parser.error("For photo testing, create a dedicated simulator named CRPE Native Photo QA and import only the fixture.")
run_id = uuid.uuid4().hex
address = f"native-{run_id}@example.test"
app_id = "fr.prepamathscrpe.test.t" + run_id[:10]
finished = threading.Event()

def deliver():
    until = time.monotonic() + 180
    while time.monotonic() < until and not finished.is_set():
        time.sleep(0.6)
        query = urllib.parse.urlencode({"query": "to:" + address, "limit": 5})
        with urllib.request.urlopen(args.mailpit + "/api/v1/search?" + query, timeout=5) as result:
            messages = json.load(result)["messages"]
        for item in messages:
            if not any(entry["Address"] == address for entry in item["To"]):
                continue
            with urllib.request.urlopen(args.mailpit + "/api/v1/message/" + item["ID"], timeout=5) as response:
                body = json.load(response)["Text"]
            match = re.search(r"crpemaths://auth#token=[a-zA-Z0-9_-]+", body)
            if match:
                # Keep the one-time token outside XCTest logs and screenshots.
                subprocess.run(["xcrun","simctl","openurl",args.device,match.group(0)], check=True, stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
                return
    if not finished.is_set():
        raise RuntimeError("Timed out waiting for the dedicated test email")

with NativeRelay(args.api) as relay:
    threading.Thread(target=relay.serve_forever, daemon=True).start()
    with concurrent.futures.ThreadPoolExecutor() as pool:
        mail = pool.submit(deliver)
        try:
            run = subprocess.run(["xcodebuild", "test", "-project", "PrepaCRPE.xcodeproj", "-scheme","PrepaCRPEJourney","-only-testing:NativeJourneyTests/NativeJourney","-parallel-testing-enabled","NO","-destination","id="+args.device,
                "-derivedDataPath",os.path.join(tempfile.gettempdir(),"crpe-journey-derived"),"CRPE_APP_ID="+app_id,"CRPE_TEST_EMAIL="+address,"CRPE_TEST_MOCK="+(str(args.mock_subject) if args.mock_subject else ""),"CRPE_TEST_PHOTO="+("yes" if args.photo_fixture else "no"),"CRPE_API_URL=http://127.0.0.1:"+str(relay.server_port)],
                cwd=Path(__file__).parent)
            if run.returncode != 0:
                raise SystemExit(run.returncode)
            mail.result(timeout=10)
            if args.mock_subject and not (relay.saved_mock and relay.mixed_mock):
                raise RuntimeError("Missing saved draft, partial score, or teacher review on real API")
            if len(relay.attempt_ids)<2 or len(set(relay.attempt_ids)) != 1:
                raise RuntimeError("Missing stable attempt replay across process restart")
            if args.photo_fixture and relay.uploads != [True]:
                raise RuntimeError("Expected one photo submission with server correction state")
            print("Verified native email sign-in, course, offline replay with one UUID, and account deletion.")
        finally:
            finished.set()
            subprocess.run(["xcrun","simctl","uninstall",args.device,app_id],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            relay.shutdown()
