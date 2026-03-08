FROM python:3.11-slim

LABEL maintainer="Ansible Runner"
LABEL description="Ansible Runner with separated collection and pip requirements"

# Variables d'environnement
ENV PYTHONUNBUFFERED=1
ENV ANSIBLE_HOST_KEY_CHECKING=False
ENV ANSIBLE_RETRY_FILES_ENABLED=False

# Installation des dépendances système
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    git \
    openssh-client \
    sshpass \
    rsync \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Création du répertoire de travail
WORKDIR /ansible

# Copie des fichiers de requirements
COPY requirements.txt /ansible/requirements.txt
COPY requirements-collections.yml /ansible/requirements-collections.yml

# Installation des packages Python
RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir -r requirements.txt

# Installation des collections Ansible
RUN ansible-galaxy collection install -r requirements-collections.yml

# Création des répertoires Ansible standards
RUN mkdir -p /ansible/playbooks /ansible/inventory /ansible/roles

# Configuration SSH
RUN mkdir -p /root/.ssh && \
    chmod 700 /root/.ssh

# Point d'entrée
WORKDIR /ansible/playbooks

CMD ["ansible-playbook", "--version"]