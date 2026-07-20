-- Fitxer de migració inicial per a PostgreSQL

-- Taula per a la memòria cau de respostes de l'orquestrador (estalvi de tokens)
CREATE TABLE IF NOT EXISTS cache_respostes (
    clau       TEXT PRIMARY KEY,
    resposta   TEXT NOT NULL,
    expira_el  TIMESTAMPTZ NOT NULL
);

-- Taula per a la pauta nutricional de la família
CREATE TABLE IF NOT EXISTS pauta_nutricional (
    id          SERIAL PRIMARY KEY,
    dia_setmana TEXT NOT NULL,  -- dilluns, dimarts, dimecres, dijous, divendres, dissabte, diumenge
    apat        TEXT NOT NULL,  -- esmorzar, dinar, berenar, sopar
    menu        TEXT NOT NULL   -- descripció del plat o menú
);

-- Índexs per a rendiment de consultes
CREATE INDEX IF NOT EXISTS idx_cache_expira ON cache_respostes (expira_el);
CREATE INDEX IF NOT EXISTS idx_pauta_dia ON pauta_nutricional (LOWER(dia_setmana));
