# Parcours oral sur simulateurs

Ces essais utilisent une instance locale de l’API Go, une base PostgreSQL de test migrée, Mailpit et un sujet oral publié par un compte enseignant de test. Les candidats portent une adresse unique en `@example.test` et leur compte est supprimé à la fin. Utiliser uniquement des émulateurs dédiés. L’API de développement ordinaire peut continuer à fonctionner sur un autre port.

1. Lancer le service vocal de test avec `python3 mobile/qa/oral_voice.py --port 18318`. Il requiert FFmpeg. Il décode les trames AAC reçues, retourne un texte de test et produit un court MP3 synthétique pour les questions.
2. Lancer une instance de l’API Go sur `HTTP_ADDR=127.0.0.1:18317` avec les adresses PostgreSQL et SMTP de la pile isolée. Configurer `ADMIN_EMAILS=oral-qa-teacher@example.test`, `MOBILE_AUTH_URL=crpemaths://auth`, `STT_ENDPOINT=http://127.0.0.1:18318/transcribe`, `TTS_ENDPOINT=http://127.0.0.1:18318/speech` et `STT_MODEL=fixture`, `TTS_MODEL=fixture`, `TTS_VOICE=fixture`. L’essai utilise les liens de connexion reçus dans Mailpit. La valeur de `--mail-port` désigne son interface HTTP.
3. Pour iOS, démarrer le simulateur puis lancer la commande suivante. Xcode installe une application de test isolée. Le pilote dépose deux fichiers AAC synthétiques dans le répertoire privé de cette application pour tester la transcription sans utiliser le microphone du Mac.

```sh
PYTHONDONTWRITEBYTECODE=1 python3 mobile/qa/ios_oral.py \
  --device <UDID> --api-port 18317 --mail-port 18125
```

4. Pour Android, démarrer un AVD dédié avec `-no-audio` et le SDK configuré. Compiler l’APK avec `-PCRPE_DEBUG_API_URL=http://127.0.0.1:18317`. Le pilote crée une redirection ADB vers le port de test. L’APK de lecture de l’arbre d’accessibilité s’instrumente lui-même et est désinstallé à la fin.

```sh
sh mobile/qa/tree/build.sh /tmp/crpe-oral-tree.apk
PYTHONDONTWRITEBYTECODE=1 python3 mobile/qa/oral_journey.py \
  --serial emulator-5586 --api-port 18317 --mail-port 18125 \
  --tree-apk /tmp/crpe-oral-tree.apk \
  --apk mobile/android/app/build/outputs/apk/debug/app-debug.apk
```

Sur Android, le parcours appuie sur les contrôles du microphone pour capter l’exposé puis une réponse avec MediaRecorder. Les deux plateformes envoient l’audio par l’API, lisent une question de synthèse, soumettent une réponse saisie et affichent l’évaluation enseignante puis la progression par critère. Une recette avec la voix d’un volontaire et le prestataire vocal de production reste nécessaire sur appareils physiques.
