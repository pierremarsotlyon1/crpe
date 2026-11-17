"""Loopback-only HTTP relay used to exercise a real native transport failure."""
import http.client
import http.server
import json
import threading
import urllib.parse

class Relay(http.server.BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def forward(self):
        if self.headers.get("Transfer-Encoding","").lower()=="chunked":
            chunks=[]
            while True:
                size=int(self.rfile.readline(96).split(b";")[0].strip(),16)
                if size==0:
                    while self.rfile.readline(1024).strip(): pass
                    break
                chunks.append(self.rfile.read(size));self.rfile.read(2)
            body=b"".join(chunks)
        else:
            body=self.rfile.read(int(self.headers.get("Content-Length",0)))
        if self.path == "/v1/attempts" and self.command == "POST":
            with self.server.lock:
                self.server.attempt_ids.append(json.loads(body)["clientId"])
                if len(self.server.attempt_ids) == 1:
                    self.send_error(503)
                    return
        upstream = http.client.HTTPConnection(self.server.upstream.hostname, self.server.upstream.port, timeout=20)
        try:
            headers = {k: v for k, v in self.headers.items() if k.lower() not in ("host","connection","transfer-encoding","content-length")}
            upstream.request(self.command, self.path, body=body or None, headers=headers)
            response = upstream.getresponse()
            payload = response.read()
            if self.command=="POST" and self.path=="/v1/submissions" and 200<=response.status<300:
                record=json.loads(payload)
                self.server.uploads.append(bool(record.get("hasImage") and record.get("correctionProgress",{}).get("stage")))
            if self.path.startswith("/v1/mock-sessions/") and 200<=response.status<300:
                if self.path.endswith("/answers") and self.command=="PUT":
                    self.server.saved_mock=bool(json.loads(payload).get("answers"))
                if self.path.endswith("/finish") and self.command=="POST":
                    result=json.loads(payload)
                    self.server.mixed_mock=(result.get("gradingStatus")=="pending_review" and
                        any(r.get("points") is None for r in result["results"]) and
                        bool(result.get("answers")) and result.get("automaticPoints",0)>0)
            self.send_response(response.status)
            for key in ("Content-Type", "Content-Disposition"):
                if response.getheader(key):
                    self.send_header(key, response.getheader(key))
            self.send_header("Content-Length", str(len(payload)))
            self.end_headers()
            self.wfile.write(payload)
        finally:
            upstream.close()
    do_GET = do_PUT = do_POST = do_DELETE = forward

class NativeRelay(http.server.ThreadingHTTPServer):
    def __init__(self, origin):
        self.upstream=urllib.parse.urlparse(origin)
        if self.upstream.hostname not in ("127.0.0.1","localhost","::1"):
            raise ValueError("Use an isolated local test API")
        self.attempt_ids=[]
        self.uploads=[]
        self.saved_mock=False
        self.mixed_mock=False
        self.lock=threading.Lock()
        super().__init__(("127.0.0.1",0),Relay)
