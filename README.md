# Gomme

Interface web d'automatisation Ansible — gestion d'inventaires, de playbooks et d'exécutions via Docker.

## Fonctionnalités

- **Inventaires** — création manuelle ou synchronisation automatique depuis Proxmox (API REST ou SSH), avec groupes, hôtes, variables et visualisation graphique
- **Repositories Git** — clone et synchronisation de dépôts (HTTPS avec identifiants ou SSH avec clé)
- **Playbooks Ansible** — configuration, variables, formulaire de saisie (survey), identifiants injectés, exécution dans un conteneur Docker isolé
- **Tâches planifiées** — exécution récurrente de playbooks via un scheduler intégré
- **Identifiants** — stockage chiffré (AES-GCM) de comptes login/mot de passe, associés aux playbooks
- **Organisations** — espaces multi-utilisateurs avec permissions granulaires (créer/modifier/supprimer inventaires et playbooks)
- **Administration** — gestion des utilisateurs, des images Docker Ansible disponibles et des paramètres globaux

## Stack technique

| Couche | Technologie |
|--------|-------------|
| Backend | Go 1.25, Echo v4, GORM |
| Base de données | MariaDB 11 |
| Exécution Ansible | Docker (via socket-proxy) |
| Frontend | HTML/CSS vanilla, Cytoscape.js (graphe) |
| Git intégré | Gitea (optionnel) |

## Prérequis

- Docker & Docker Compose
- Une image Docker contenant Ansible (configurable dans l'interface admin)

## Installation

```bash
cp .env.example .env
# Éditer .env avec vos valeurs
docker compose up -d --build
```

L'application est accessible sur `http://localhost:3000`.

**Compte par défaut :** `admin` / `admin` — un changement de mot de passe est demandé à la première connexion.

## Configuration (.env)

```env
# Application
SECRET_KEY=changeme_secret_key_32_chars_min_   # Clé AES + sessions (min. 32 chars)
SERVER_PORT=3000

# Base de données Gomme
GOMME_DB_HOST=gomme-db
GOMME_DB_USER=gomme
GOMME_DB_PASSWORD=changeme_gomme_password
GOMME_DB_NAME=gomme
GOMME_DB_ROOT_PASSWORD=changeme_gomme_root

# Gitea (optionnel)
GITEA_PORT=8080
GITEA_DB_HOST=gitea-db
GITEA_DB_USER=gitea
GITEA_DB_PASSWORD=changeme_gitea_password
GITEA_DB_NAME=gitea
GITEA_DB_ROOT_PASSWORD=changeme_gitea_root

# Images
gitea_image=docker.gitea.com/gitea:1.25.4
mysql_image=mariadb:11
```

## Architecture

```
src/                  Code Go
├── config/           Variables d'environnement
├── crypto/           Chiffrement AES-GCM
├── db/               Init GORM + migrations
├── docker/           Client HTTP socket-proxy
├── handlers/         Contrôleurs Echo
├── inventory/        Sources Proxmox (API + SSH)
├── middleware/       RequireAuth, RequireAdmin
├── models/           Modèles GORM
└── scheduler/        Tâches planifiées

app/
├── templates/        Templates HTML Go
└── static/           CSS, JS (Cytoscape.js, Chart.js)
```

## Rebuild après modification

Le binaire Go est compilé dans l'image Docker — tout changement de code source nécessite un rebuild :

```bash
docker compose up -d --build gomme-app
```

## Inventaire Proxmox

Deux modes de connexion disponibles :

- **API REST** — authentification par compte (`user@realm` + mot de passe) ou par token API (`user@realm!tokenid` + secret). Supporte la récupération des tags VM comme groupes Ansible.
- **SSH** — connexion directe au nœud pour lister les VMs et conteneurs LXC via `qm list` / `pct list`.

## Sécurité

- Mots de passe et clés SSH stockés chiffrés en base (AES-GCM, clé dérivée de `SECRET_KEY`)
- Accès Docker via `tecnativa/docker-socket-proxy` — seules les opérations nécessaires sont autorisées
- Sessions cookie signées (Gorilla Sessions)
- Authentification requise sur toutes les routes sauf `/login`
