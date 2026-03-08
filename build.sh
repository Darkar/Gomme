#!/bin/bash

param1="$1"

if [ "$param1" == "clean" ]; then
    docker compose down -v
fi

if [ ! -f ".env" ]; then
    echo "[info] .env absent — génération depuis .env.example"
    cp .env.example .env

    gen() { LC_ALL=C tr -dc 'A-Za-z0-9!@#%^&*' < /dev/urandom | head -c "$1"; echo; }

    SECRET_KEY=$(gen 32)
    DB_PASSWORD=$(gen 14)
    DB_ROOT_PASSWORD=$(gen 14)

    sed -i "s|^SECRET_KEY=.*|SECRET_KEY=${SECRET_KEY}|" .env
    sed -i "s|^GOMME_DB_PASSWORD=.*|GOMME_DB_PASSWORD=${DB_PASSWORD}|" .env
    sed -i "s|^GOMME_DB_ROOT_PASSWORD=.*|GOMME_DB_ROOT_PASSWORD=${DB_ROOT_PASSWORD}|" .env

    echo "[info] .env créé avec des secrets générés aléatoirement"
    echo "       SECRET_KEY         : ${SECRET_KEY}"
    echo "       GOMME_DB_PASSWORD  : ${DB_PASSWORD}"
    echo "       GOMME_DB_ROOT_PASSWORD : ${DB_ROOT_PASSWORD}"
fi


curl -fsSL "https://cdn.jsdelivr.net/npm/chart.js@4.4.4/dist/chart.umd.min.js" -o "app/static/js/chart.umd.min.js"

docker compose build --no-cache

docker image prune -f

docker compose up -d

