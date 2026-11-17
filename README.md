# Prépa Maths CRPE

Une plateforme de préparation aux mathématiques du CRPE. L’enseignante prépare ses cours dans un atelier web et choisit les contenus à publier. Les élèves disposent d’un parcours web et de clients mobiles natifs. La liste d’attente fonctionne indépendamment des ventes.

## Démarrer en local

Lancez cette commande depuis la racine du dépôt avec Docker et Docker Compose.

```sh
make dev
```

1. Ouvrez le [site local](http://localhost:3000). L’accueil permet de s’inscrire à la liste d’attente. Le lien de confirmation arrive dans [Mailpit](http://localhost:8025).
2. Pour ouvrir l’atelier, cliquez sur **Mon espace**, saisissez `enseignante@example.test`, ouvrez son email dans Mailpit et confirmez la connexion. Cette adresse d’administration est configurée par `ADMIN_EMAILS`.
3. Dans **Contenus**, créez un chapitre, ajoutez sa source et un PDF, puis rattachez-lui des leçons et des exercices. Le cours, les formules, le corrigé et les critères se relisent avant validation. **Importer** accepte les fiches Markdown et les exercices CSV.
4. Dans **Épreuves et suivi**, attribuez les points aux réponses rédigées et transmettez votre commentaire. Pour les membres Suivi, vous pouvez ajouter un lien de correction vidéo, le rendez-vous hebdomadaire et l’invitation WhatsApp.
5. Pour suivre le parcours élève en parallèle, ouvrez une fenêtre privée et connectez-vous avec une autre adresse. Les chapitres apparaissent à mesure que l’enseignante les valide.

Le serveur SMTP local conserve les emails dans Mailpit. Les enseignants fournissent le matériel pédagogique. `make down` arrête les services et conserve les volumes de développement. L’état précis des vérifications et des opérations nécessaires avant publication est dans [STATUS.md](STATUS.md).

## Organisation

| Répertoire             | Responsabilité                                                  |
| ---------------------- | --------------------------------------------------------------- |
| `backend/`             | API Go, workers indépendants, migrations SQL, contrat OpenAPI   |
| `web/`                 | Site public Next.js, espace élève et atelier de l’enseignante   |
| `mobile/ios/`          | Application native Swift, projet Xcode et tests                 |
| `mobile/android/`      | Application native Java et projet Gradle                        |
| `packages/api-client/` | Client HTTP TypeScript typé à partir du contrat                 |
| `infra/`               | Pile locale PostgreSQL, S3, SMTP, proxy et services applicatifs |
| `scripts/`             | Vérifications reproductibles dans des environnements isolés     |

Le contrat OpenAPI définit les routes communes au web et aux applications mobiles. PostgreSQL conserve les sessions, la progression et les travaux différés. Le stockage compatible S3 conserve les pièces jointes. Les ventes sont désactivées par défaut avec `SALES_ENABLED=false`.

## Développer et vérifier

Installez Go selon `backend/go.mod`, Node 24 et npm 11+. Les applications natives utilisent Xcode ou Android Studio avec le SDK Android adapté. Les prérequis et les commandes détaillées figurent dans le [guide de développement](docs/development.md).

```sh
make install
make check
make integration
make scale-smoke
make browser-e2e
```

Consultez le [plan et les critères de livraison](PLANS.md), la [carte du code](docs/context-map.md) et les [choix d’architecture](docs/architecture.md). Les règles de contribution commencent dans [AGENTS.md](AGENTS.md).
