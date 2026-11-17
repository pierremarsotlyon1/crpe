#!/usr/bin/env python3
"""Exercise the native APK on a dedicated emulator via Android's UI Automator and local Mailpit."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import time
import urllib.parse
import urllib.request
import uuid
import threading
import sys
sys.dont_write_bytecode=True
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from qa_relay import NativeRelay
import xml.etree.ElementTree as ET

parser=argparse.ArgumentParser()
parser.add_argument("--serial",required=True)
parser.add_argument("--mock-title",help="Validated mixed QA subject title with numeric answer 7 and one written exercise.")
parser.add_argument("--api-port",type=int,default=8080)
parser.add_argument("--mailpit-port",type=int,default=8025)
args=parser.parse_args()
if not args.serial.startswith("emulator-"):
    parser.error("Choose a dedicated test emulator.")
root=Path(__file__).resolve().parent
adb=Path(os.environ["ANDROID_HOME"])/"platform-tools"/"adb"
app="fr.prepamathscrpe.mobile"
email="native-android-"+uuid.uuid4().hex[:12]+"@example.test"
port="tcp:"+str(args.api_port)

def device(*cmd):
    return subprocess.check_output([str(adb),"-s",args.serial,*cmd],stderr=subprocess.DEVNULL).decode("utf-8",errors="replace")
def shell(*cmd):
    return device("shell",*cmd)
def nodes():
    for attempt in range(4):
        try:
            result=shell("uiautomator","dump","/sdcard/window-crpe.xml")
            if "ERROR:" in result: raise ValueError("Android UI is transitioning")
            return list(ET.fromstring(shell("cat","/sdcard/window-crpe.xml")).iter("node"))
        except (subprocess.CalledProcessError, ET.ParseError, ValueError):
            if attempt==3: raise
            time.sleep(.4)
def find(text=None,kind=None,timeout=20,starts=False):
    limit=time.monotonic()+timeout
    while time.monotonic()<limit:
        for node in nodes():
            a=node.attrib
            if kind and not a.get("class","").endswith(kind):
                continue
            values=[a.get("text",""),a.get("content-desc","")]
            if text is None or any(v.casefold().startswith(text.casefold()) if starts else v.casefold()==text.casefold() for v in values):
                return a
        time.sleep(0.25)
    raise AssertionError("Missing control: "+str(text or kind))
def tap(node):
    x1,y1,x2,y2=map(int,re.findall(r"\d+",node["bounds"]))
    shell("input","tap",str((x1+x2)//2),str((y1+y2)//2))
def tap_text(text,**kw):
    tap(find(text,**kw))
def mail_link():
    base="http://127.0.0.1:"+str(args.mailpit_port)
    for _ in range(40):
        search=urllib.parse.urlencode({"query":"to:"+email})
        with urllib.request.urlopen(base+"/api/v1/search?"+search,timeout=4) as r:
            messages=json.load(r)["messages"]
        if messages:
            with urllib.request.urlopen(base+"/api/v1/message/"+messages[0]["ID"],timeout=4) as r:
                body=json.load(r)["Text"]
            token=re.search(r"crpemaths://auth#token=[A-Za-z0-9_-]+",body)
            if token:return token.group(0)
        time.sleep(.4)
    raise AssertionError("Test login email missing")
def first_exercise():
    # The first exercise selection is the first non-navigation button in the copies section.
    for node in nodes():
        a=node.attrib
        if a["class"].endswith("Button") and a["text"] and a["text"] not in ("Journée","Parcours","Copies","Activités","Compte"):
            return a
    raise AssertionError("No published exercise")
def scroll_down():
    width,height=map(int,re.findall(r"(\d+)x(\d+)",shell("wm","size"))[-1])
    shell("input","swipe",str(width//2),str(int(height*.72)),str(width//2),str(int(height*.30)),"350")
def make_visible(text,limit=6):
    for _ in range(limit):
        for node in nodes():
            if text in (node.attrib.get("text"),node.attrib.get("content-desc")):
                return node.attrib
        scroll_down()
    raise AssertionError("Missing scroll target: "+text)

relay=NativeRelay("http://127.0.0.1:"+str(args.api_port))
threading.Thread(target=relay.serve_forever,daemon=True).start()
device("install","-r",str(root/"app/build/outputs/apk/debug/app-debug.apk"))
# Refuse to replace an actual signed-in account. A separate clean emulator is required.
device("reverse",port,"tcp:"+str(relay.server_port))
fixture="crpe-native-copy-"+uuid.uuid4().hex[:8]+".png"
photo=root.parent/"ios/Assets.xcassets/AppIcon.appiconset/Icon.png"
device("push",str(photo),"/sdcard/Download/"+fixture)
shell("am","broadcast","-a","android.intent.action.MEDIA_SCANNER_SCAN_FILE","-d","file:///sdcard/Download/"+fixture)
try:
    shell("am","force-stop",app)
    shell("am","start","-W","-f","0x10008000","-n",app+"/.MainActivity")
    time.sleep(1)
    if any(n.attrib.get("text")=="Supprimer définitivement mon compte ?" for n in nodes()):
        shell("input","keyevent","4")
    if any(n.attrib.get("text")=="Compte" for n in nodes()):
        tap_text("Compte")
        if not any(n.attrib.get("text","").startswith("native-android-") and n.attrib.get("text","").endswith("@example.test") for n in nodes()):
            raise RuntimeError("Dedicated emulator required: another account is signed in.")
        tap(make_visible("Supprimer mon compte"));tap_text("Supprimer")
    tap_text("Adresse e-mail")
    shell("input","text",email)
    shell("input","keyevent","4")
    tap_text("Recevoir mon lien")
    find("Consultez votre messagerie",starts=True)
    shell("am","start","-a","android.intent.action.VIEW","-d",mail_link(),"-p",app)
    find("Ma journée")
    tap_text("Parcours");find("Mon parcours")
    tap_text("Journée")
    if any(n.attrib.get("text")=="Télécharger le parcours" for n in nodes()):
        tap_text("Télécharger le parcours")
    tap_text("Commencer ma séance")
    if any(n.attrib.get("content-desc")=="Réponse numérique" for n in nodes()):
        tap_text("Réponse numérique");shell("input","text","42");shell("input","keyevent","4")
    else:
        choice=first_exercise();tap(choice)
    tap_text("Valider ma réponse")
    find("Réponse enregistrée.",starts=True)
    shell("am","force-stop",app)
    shell("am","start","-W","-f","0x10008000","-n",app+"/.MainActivity")
    find("Ma journée")
    tap_text("Copies")
    tap(first_exercise())
    tap(make_visible("Choisir une photo"))
    # ACTION_OPEN_DOCUMENT presents the newly added image under Recents or Downloads.
    if not any(fixture in (n.attrib.get("text","")+n.attrib.get("content-desc","")) for n in nodes()):
        tap_text("Show roots")
        tap_text("Downloads")
    tap(find(fixture.rsplit(".",1)[0],starts=True,timeout=15))
    find("Aperçu de la copie",timeout=15)
    tap(make_visible("Transmettre ma copie"))
    limit=time.monotonic()+30
    while time.monotonic()<limit:
        labels=[n.attrib.get("text","") for n in nodes()]
        if any("Dernière mise à jour" in t for t in labels):break
        scroll_down()
    else:raise AssertionError("Server correction state was not displayed")
    if args.mock_title:
        tap_text("Activités")
        tap(make_visible("Rendez-vous hebdomadaire · Suivi 🔒"))
        find("Cette activité est incluse dans la formule Suivi.")
        tap_text("Toutes les activités")
        def open_mock():
            tap(make_visible("Sujets blancs"))
            tap(make_visible("Ouvrir le sujet "+args.mock_title,limit=18))
            find("Temps restant")
        open_mock()
        tap(make_visible("Votre réponse"));shell("input","text","7");shell("input","keyevent","4")
        tap(make_visible("Votre raisonnement"));shell("input","text","Je%sregroupe%sles%sunites.");shell("input","keyevent","4")
        tap(make_visible("Enregistrer mes réponses"));time.sleep(2)
        shell("am","force-stop",app);shell("am","start","-W","-f","0x10008000","-n",app+"/.MainActivity")
        tap_text("Activités");open_mock()
        assert make_visible("Votre raisonnement")["text"]=="Je regroupe les unites."
        tap(make_visible("Rendre ma copie"));tap_text("Rendre ma copie")
        find("Résultat provisoire")
        if not (relay.saved_mock and relay.mixed_mock):raise AssertionError("Missing real saved draft or mixed grading")
    tap_text("Compte")
    tap(make_visible("Mon abonnement"))
    find("Restaurer mes achats Google Play")
    if any(n.attrib.get("text","").startswith("Choisir la formule") for n in nodes()):
        raise AssertionError("Closed-sales UI exposed a purchase")
    if not shell("pm","list","packages","com.android.vending").strip():
        tap_text("Restaurer mes achats Google Play")
        until=time.monotonic()+20
        notices=("Google Play n’est pas disponible sur cet appareil.","La restauration Google Play est indisponible. Vérifiez que le Play Store est installé et connecté.")
        while time.monotonic()<until:
            if any(n.attrib.get("text","") in notices for n in nodes()):break
        else:raise AssertionError("Missing Google Play availability explanation")
    tap_text("Revenir à mon compte")
    tap(make_visible("Choisir une heure"));tap_text("OK")
    for n in nodes():
        a=n.attrib
        if a.get("resource-id","").endswith(":id/permission_allow_button"):
            tap(a);break
    find("Rappel actif à 19 h, heure locale.")
    if "fr.prepamathscrpe.mobile.DAILY" not in shell("dumpsys","alarm"):
        raise AssertionError("Expected an alarm registered with Android AlarmManager")
    tap(make_visible("Désactiver le rappel"))
    find("Le rappel est désactivé.")
    tap(make_visible("Supprimer mon compte"))
    tap_text("Supprimer")
    find("Adresse e-mail")
    if len(relay.attempt_ids)<2 or len(set(relay.attempt_ids))!=1 or relay.uploads!=[True]:
        raise AssertionError("Native receipt or photo replay validation failed")
    if args.mock_title: print("Verified locked Suivi access, restored mock answers, 3 automatic points and a written answer awaiting teacher feedback.")
    print("Verified native Android login, catalogue, offline answer and restart, photo upload, correction state, deletion.")
finally:
    shell("rm","-f","/sdcard/Download/"+fixture,"/sdcard/window-crpe.xml")
    subprocess.run([str(adb),"-s",args.serial,"reverse","--remove",port],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
    relay.shutdown();relay.server_close()
