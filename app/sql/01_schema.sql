-- Schéma Gomme
-- Exécuté automatiquement par MariaDB au premier démarrage du conteneur

CREATE TABLE IF NOT EXISTS users (
  id                   BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  username             VARCHAR(191)    NOT NULL,
  password_hash        VARCHAR(191)    NOT NULL,
  is_admin             TINYINT(1)      NOT NULL DEFAULT 0,
  must_change_password TINYINT(1)      NOT NULL DEFAULT 0,
  created_at           DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY idx_users_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS inventories (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name         VARCHAR(191)    NOT NULL,
  source       VARCHAR(191)    NOT NULL,
  config       LONGTEXT        NULL,
  last_sync_at DATETIME(3)     NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS hosts (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  inventory_id BIGINT UNSIGNED NOT NULL,
  name         VARCHAR(191)    NOT NULL,
  ip           VARCHAR(191)    NULL,
  description  VARCHAR(191)    NULL,
  vars         LONGTEXT        NULL,
  PRIMARY KEY (id),
  KEY idx_hosts_inventory_id (inventory_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS `groups` (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  inventory_id BIGINT UNSIGNED NOT NULL,
  name         VARCHAR(191)    NOT NULL,
  description  VARCHAR(191)    NULL,
  PRIMARY KEY (id),
  KEY idx_groups_inventory_id (inventory_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS host_groups (
  host_id  BIGINT UNSIGNED NOT NULL,
  group_id BIGINT UNSIGNED NOT NULL,
  PRIMARY KEY (host_id, group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS repositories (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name         VARCHAR(191)    NOT NULL,
  url          VARCHAR(191)    NOT NULL,
  branch       VARCHAR(191)    NOT NULL DEFAULT 'main',
  local_path   VARCHAR(191)    NULL,
  auto_sync    TINYINT(1)      NOT NULL DEFAULT 0,
  last_sync_at DATETIME(3)     NULL,
  PRIMARY KEY (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS playbooks (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  repository_id BIGINT UNSIGNED NOT NULL,
  path          VARCHAR(191)    NOT NULL,
  name          VARCHAR(191)    NOT NULL,
  description   VARCHAR(191)    NULL,
  PRIMARY KEY (id),
  KEY idx_playbooks_repository_id (repository_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS playbook_runs (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  playbook_id  BIGINT UNSIGNED NOT NULL,
  user_id      BIGINT UNSIGNED NOT NULL,
  inventory_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
  status       VARCHAR(191)    NOT NULL DEFAULT 'pending',
  output       LONGTEXT        NULL,
  started_at   DATETIME(3)     NULL,
  finished_at  DATETIME(3)     NULL,
  PRIMARY KEY (id),
  KEY idx_playbook_runs_playbook_id (playbook_id),
  KEY idx_playbook_runs_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS playbook_vars (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  playbook_id BIGINT UNSIGNED NOT NULL,
  `key`       VARCHAR(191)    NOT NULL,
  value       LONGTEXT        NULL,
  encrypted   TINYINT(1)      NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY idx_playbook_vars_playbook_id (playbook_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS survey_fields (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  playbook_id BIGINT UNSIGNED NOT NULL,
  label       VARCHAR(191)    NOT NULL,
  var_name    VARCHAR(191)    NOT NULL,
  type        VARCHAR(191)    NOT NULL DEFAULT 'text',
  options     LONGTEXT        NULL,
  required    TINYINT(1)      NOT NULL DEFAULT 0,
  sort_order  BIGINT          NOT NULL DEFAULT 0,
  PRIMARY KEY (id),
  KEY idx_survey_fields_playbook_id (playbook_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS settings (
  id    BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `key` VARCHAR(191)    NOT NULL,
  value LONGTEXT        NULL,
  PRIMARY KEY (id),
  UNIQUE KEY idx_settings_key (`key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
