-- Utilisateur admin par défaut
-- Mot de passe : admin (bcrypt cost 10)
-- Le changement de mot de passe est obligatoire à la première connexion

INSERT INTO users (username, password_hash, is_admin, must_change_password, created_at)
SELECT 'admin', '$2a$10$QNHndIGvyjOVcZHs1PNeCOPk0e02nQar4rF2kHpJI2f9XHhBMUjae', 1, 1, NOW()
WHERE NOT EXISTS (SELECT 1 FROM users WHERE username = 'admin');
