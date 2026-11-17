# Applications natives CRPE

Le client iOS fonctionne à partir d’iOS 17 en SwiftUI, le client Android à partir d’Android 9 en Java. Le compte, les contenus validés, les échéances de révision et les corrections proviennent de la même API que le site.

## Développement

1. Démarrer le serveur de développement avec `make dev`. Pour les essais élèves, publier un exercice depuis le compte enseignant de la base de test. Une base dépourvue de contenu validé présente un parcours vide.
2. Sur Mac, ouvrir `ios/PrepaCRPE.xcodeproj` dans Xcode, sélectionner le scheme `PrepaCRPE` et un simulateur iPhone. La valeur Debug de `CRPE_API_URL` est `http://localhost:8080`. Sur appareil physique, utiliser une origine accessible depuis le téléphone et configurer l’équipe de signature.
3. Pour Android, utiliser le JDK 17, une installation Android SDK avec `platforms;android-37.0`, `build-tools;37.0.0` et `platform-tools`. Renseigner `JAVA_HOME` et `ANDROID_HOME`. Ouvrir `android/` dans Android Studio ou lancer `./gradlew :app:installDebug` depuis ce dossier. L’origine Debug par défaut de l’émulateur est `http://10.0.2.2:8080`. L’option `-PCRPE_DEBUG_API_URL=http://adresse-locale:8080` la remplace.
4. Demander le lien de connexion depuis l’application. En développement, ouvrir l’e-mail de Mailpit sur le téléphone ou le simulateur. Les deux applications enregistrent le schéma `crpemaths://auth`. Le jeton à usage unique reste dans le fragment du lien.

## Vérifications

Depuis la racine du dépôt, `make native-check` lance les tests iOS, les tests Java et l’assemblage de l’APK Debug. `ruby mobile/generate.rb --check` vérifie les modèles et chemins issus de l’OpenAPI.

Les parcours ci-dessous utilisent un environnement local isolé avec Mailpit, un exercice validé et un appareil virtuel de test. Ils créent puis suppriment leur propre compte.

```sh
python3 mobile/ios/test-journey.py --device <UDID-du-simulateur> --api http://127.0.0.1:8080 --mailpit http://127.0.0.1:8025

cd mobile/android
./gradlew -PCRPE_DEBUG_API_URL=http://127.0.0.1:8080 :app:assembleDebug
python3 test-journey.py --serial emulator-5554 --api-port 8080 --mailpit-port 8025
```

Le parcours iOS coupe la première requête de réponse, redémarre l’application et vérifie la réutilisation de l’identifiant UUID pour la synchronisation. Le parcours Android utilise un relais adb, la galerie système et une image de test. Sur les deux plateformes, les photos transmises sont réencodées en JPEG avant leur enregistrement en file locale. Les données locales sont isolées par compte et effacées à la déconnexion explicite. Les tests activent puis annulent le rappel quotidien après autorisation du système. Le système Android peut différer son affichage en mode économie d’énergie.

Les options `--mock-subject <UUID>` sur iOS et `--mock-title <titre>` sur Android étendent ce test au verrou Suivi et à la reprise d’un concours blanc mixte. Le sujet validé de cette base isolée doit contenir un exercice numérique de réponse 7 et une question de rédaction. Le test attend l’enregistrement serveur du brouillon, redémarre l’application, rend la copie et vérifie la note automatique ainsi que la relecture en attente. Sur iOS, `--photo-fixture` active la sélection de l’image synthétique sur un simulateur de test dédié nommé `CRPE Native Photo QA` dont la photothèque contient cette image.

## Accès et examens

Les droits du menu Activités proviennent de `GET /v1/me/access`. Le dernier état reçu est conservé dans la base locale du compte pour la consultation hors ligne. L’application demande au serveur les contenus suivant les droits accordés. Les sujets blancs gardent un brouillon local, envoient les modifications au serveur et reprennent l’échéance d’origine. L’API clôt le sujet à échéance avec les réponses qu’elle a reçues. Une rédaction reçoit une note après la relecture enseignante. L’oral permet la saisie ou l’enregistrement, la transcription, les questions et un retour par critère. Les liens du direct, du groupe et des corrections vidéo proviennent de l’espace Suivi authentifié.

## Achats dans les applications

Les activités verrouillées indiquent la formule requise. Quand les ventes sont ouvertes, cette page présente les produits du catalogue serveur disponibles dans l’App Store ou Google Play. Les noms, prix et périodes viennent de la boutique de l’appareil. La connexion au compte membre précède toute réservation. Les réglages du compte donnent accès à la restauration et à la gestion des abonnements de la boutique d’origine.

- **Apple.** Créer les produits Autonomie et Suivi dans le même groupe d’abonnements renouvelables, classés par niveau de service, puis configurer leurs identifiants sur le serveur. StoreKit 2 associe la transaction au membre avec `appAccountToken`. L’application écoute les transactions en cours, envoie leur identifiant au serveur et termine la transaction après vérification. Le compte App Store permet de gérer le renouvellement. [StoreKit](https://developer.apple.com/documentation/storekit/in-app-purchase)
- **Google.** Configurer les produits et leurs forfaits de base à renouvellement automatique. Un même produit peut proposer plusieurs périodes, distinguées par leur identifiant de forfait. Le serveur fournit le produit, le forfait et l’identifiant de compte obscurci. Le client Java utilise Google Play Billing 9.1.0 et les tarifs périodiques fournis par Play. Un changement de formule réutilise l’achat Google courant. Entre produits distincts, la montée de formule prend effet avec prorata et la descente à la prochaine échéance. Entre deux forfaits du même produit, la facturation du nouveau forfait commence à la prochaine échéance. Le serveur vérifie le jeton et accuse réception de l’achat. [Google Play Billing](https://developer.android.com/google/play/billing/integrate)
- **Accès.** Avant la feuille de paiement, l’API compare sous verrou le plan affiché au plan actif et réserve la commande avec son UUID. L’annulation explicite et la vérification définitive libèrent uniquement cette réservation. Une restauration sans correspondance laisse les autres commandes intactes. L’application transmet ensuite le justificatif à la vérification authentifiée et relit `/v1/me/access`. Une transaction annulée ou en attente conserve les droits déjà accordés. Les accès payants hors ligne respectent leur échéance reçue du serveur. Avec les ventes fermées, la restauration reste accessible dans le compte.
- **Essais boutique.** Le fichier `ios/Tests/Subscriptions.storekit` appartient uniquement aux tests Xcode. Pour qualifier la distribution, utiliser les produits App Store Connect et un compte Sandbox Apple, puis une piste de test interne Google Play, les mêmes package/product/base-plan IDs et des testeurs de licence. Vérifier achat, restauration sur un autre appareil, renouvellement, expiration et remboursement avec les notifications réelles. [Tests Xcode](https://developer.apple.com/documentation/xcode/setting-up-storekit-testing-in-xcode) et [tests Google Play](https://developer.android.com/google/play/billing/test).

## Distribution

L’identifiant proposé est `fr.prepamathscrpe.mobile`. Confirmer sa disponibilité sur les comptes de distribution et le nom affiché avant publication.

- **iOS.** Fournir une équipe Apple Developer, l’identifiant enregistré et les certificats ou profils associés. Configurer `CRPE_API_URL` sur l’origine HTTPS de production et, si nécessaire, `CRPE_APP_ID`. Dans Xcode, choisir un appareil générique iOS et utiliser **Product → Archive**, puis tester la distribution dans TestFlight. Le scheme Release contrôle la configuration de l’API.
- **Android.** Configurer l’origine HTTPS et ajuster `versionCode` dans `android/app/build.gradle` pour chaque publication. Depuis Android Studio, utiliser **Generate Signed App Bundle** avec la clé d’importation propriétaire. Pour produire le bundle avant signature, exécuter `./gradlew -PCRPE_API_URL=https://api.exemple.fr :app:bundleRelease`. Seul le bundle signé par la clé autorisée peut rejoindre une piste Play Console. Conserver la clé et les mots de passe hors du dépôt.
- Sur appareils physiques, vérifier les liens reçus dans les clients mail, la lisibilité des formules avec les réglages d’accessibilité, le changement d’heure, les permissions du rappel, du micro et de la caméra, une vraie prise de vue et la synchronisation après coupure réseau.
- Renseigner dans les stores la politique de confidentialité et les données effectivement traitées par l’hébergement, les services d’IA et les autres prestataires configurés. L’app permet d’exporter ses données et de supprimer son compte. L’adresse e-mail, l’identifiant du compte, les réponses, la progression, les photos, les extraits audio, les transcriptions envoyées et l’historique d’achat sont associés à l’utilisateur.
- Le manifeste iOS est livré avec le binaire. Comparer les pratiques de l’environnement de production aux déclarations App Privacy et Data safety, aux règles relatives aux comptes et au traitement des données par les services de correction. Vérifier les exigences applicables aux applications éducatives lors de la soumission.

Les références officielles couvrent la [confidentialité Apple](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration) et la [signature Android](https://developer.android.com/studio/publish/app-signing).
